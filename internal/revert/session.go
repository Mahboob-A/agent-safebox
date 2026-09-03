package revert

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

var (
	// ErrNoSessionFound is returned when no active session matches the requested directory.
	ErrNoSessionFound = errors.New("safebox: no active session found for directory")
)

// ErrSessionAlreadyActive is returned when a new session is requested in a
// directory where an existing session's <baseDir>/active lockfile is held
// by a live process.
type ErrSessionAlreadyActive struct {
	PID     int
	BaseDir string
}

func (e *ErrSessionAlreadyActive) Error() string {
	return fmt.Sprintf("safebox session is already active in %s (PID %d)", e.BaseDir, e.PID)
}

func (e *ErrSessionAlreadyActive) Hint() string {
	return "wait for active session to complete, or use 'safebox diff' / 'safebox apply' to inspect or capture changes; pass --force-discard to override"
}

// Session encapsulates the metadata and directory paths for an OverlayFS execution session.
type Session struct {
	ID        string    `json:"id"`
	BaseDir   string    `json:"base_dir"`
	LowerDir  string    `json:"lower_dir"`
	UpperDir  string    `json:"upper_dir"`
	WorkDir   string    `json:"work_dir"`
	MergedDir string    `json:"merged_dir"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionRoot returns the root directory where safebox sessions are stored.
// Defaults to /tmp/safebox/sessions or respects SAFEBOX_SESSION_ROOT environment variable.
func SessionRoot() string {
	if root := os.Getenv("SAFEBOX_SESSION_ROOT"); root != "" {
		return root
	}
	return filepath.Join(os.TempDir(), "safebox", "sessions")
}

// PruneSessions scans the session root directory and purges sessions older than maxAge.
// Returns the count of pruned sessions.
func PruneSessions(maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, nil
	}

	root := SessionRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("safebox: failed to read session root directory: %w", err)
	}

	now := time.Now()
	prunedCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessDir := filepath.Join(root, entry.Name())
		sess, err := LoadSession(sessDir)
		if err != nil {
			if fi, sErr := entry.Info(); sErr == nil && now.Sub(fi.ModTime()) > maxAge {
				_ = os.RemoveAll(sessDir)
				prunedCount++
			}
			continue
		}

		// Skip active sessions regardless of age
		if active, _, _ := sess.IsActive(); active {
			continue
		}

		if now.Sub(sess.CreatedAt) > maxAge {
			if err := DiscardSession(sess); err == nil {
				prunedCount++
			}
		}
	}

	return prunedCount, nil
}

// CreateSession initializes a new session directory structure for lowerDir,
// creates upper, work, and merged directories, and persists session metadata.
func CreateSession(lowerDir string) (*Session, error) {
	if lowerDir == "" {
		return nil, errors.New("safebox: lower directory cannot be empty")
	}
	absLower, err := filepath.Abs(lowerDir)
	if err != nil {
		return nil, fmt.Errorf("safebox: cannot resolve absolute path for lower directory: %w", err)
	}

	// Check if an active session already exists in this directory
	if existing, xErr := MostRecentSession(absLower, true); xErr == nil && existing != nil {
		if active, activePID, _ := existing.IsActive(); active {
			return nil, &ErrSessionAlreadyActive{PID: activePID, BaseDir: existing.BaseDir}
		}
	} else if xErr != nil && !errors.Is(xErr, ErrNoSessionFound) {
		return nil, xErr
	}

	// Automatic best-effort pruning of stale sessions older than 24 hours
	_, _ = PruneSessions(24 * time.Hour)

	sessionID := fmt.Sprintf("sess-%d-%d", time.Now().UnixNano(), os.Getpid())
	baseDir := filepath.Join(SessionRoot(), sessionID)
	upperDir := filepath.Join(baseDir, "upper")
	workDir := filepath.Join(baseDir, "work")
	mergedDir := filepath.Join(baseDir, "merged")

	if err := os.MkdirAll(upperDir, 0700); err != nil {
		return nil, fmt.Errorf("safebox: failed to create overlay upper directory: %w", err)
	}
	if err := os.MkdirAll(workDir, 0700); err != nil {
		return nil, fmt.Errorf("safebox: failed to create overlay work directory: %w", err)
	}
	if err := os.MkdirAll(mergedDir, 0700); err != nil {
		return nil, fmt.Errorf("safebox: failed to create overlay merged directory: %w", err)
	}

	session := &Session{
		ID:        sessionID,
		BaseDir:   baseDir,
		LowerDir:  absLower,
		UpperDir:  upperDir,
		WorkDir:   workDir,
		MergedDir: mergedDir,
		CreatedAt: time.Now().UTC(),
	}

	metaPath := filepath.Join(baseDir, "session.json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("safebox: failed to serialize session metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, data, 0600); err != nil {
		return nil, fmt.Errorf("safebox: failed to write session metadata: %w", err)
	}

	// Write active lockfile containing parent PID
	lockPath := filepath.Join(baseDir, "active")
	pidStr := fmt.Sprintf("%d\n", os.Getpid())
	if err := os.WriteFile(lockPath, []byte(pidStr), 0600); err != nil {
		return nil, fmt.Errorf("safebox: failed to write active lockfile: %w", err)
	}

	return session, nil
}

// LoadSession reads and deserializes session metadata from a given session base directory.
func LoadSession(baseDir string) (*Session, error) {
	metaPath := filepath.Join(baseDir, "session.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("safebox: failed to read session metadata: %w", err)
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("safebox: failed to parse session metadata: %w", err)
	}
	return &session, nil
}

// MostRecentSession finds the latest session associated with targetDir or any of its ancestors.
// When strict is true, only exact LowerDir matches are returned (use for destructive ops like revert and apply).
// When strict is false, prefix matches are also accepted (use for diff discovery from subdirs).
func MostRecentSession(targetDir string, strict bool) (*Session, error) {
	if targetDir == "" {
		return nil, errors.New("safebox: target directory cannot be empty")
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("safebox: cannot resolve absolute path: %w", err)
	}

	root := SessionRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoSessionFound
		}
		return nil, fmt.Errorf("safebox: failed to read session root directory: %w", err)
	}

	// Iterate in reverse (newest session directories first based on sess-<timestamp>-<pid> naming)
	var matching []*Session
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		sessDir := filepath.Join(root, entry.Name())
		sess, err := LoadSession(sessDir)
		if err != nil {
			continue
		}
		if strict {
			if sess.LowerDir == absTarget {
				return sess, nil
			}
		} else {
			if sess.LowerDir == absTarget {
				return sess, nil
			}
			if strings.HasPrefix(absTarget, sess.LowerDir+string(filepath.Separator)) {
				matching = append(matching, sess)
			}
		}
	}

	if len(matching) == 0 {
		return nil, ErrNoSessionFound
	}

	sort.Slice(matching, func(i, j int) bool {
		return matching[i].CreatedAt.After(matching[j].CreatedAt)
	})

	return matching[0], nil
}

// DiscardSession purges the session base directory and all its contents.
func DiscardSession(session *Session) error {
	if session == nil || session.BaseDir == "" {
		return nil
	}
	return os.RemoveAll(session.BaseDir)
}

// IsActive checks whether the session is currently locked by a live process.
// Returns (active, pid, err).
//
// active=true means <baseDir>/active exists, contains a parseable PID > 0,
// and that PID is alive per syscall.Kill(pid, 0).
// EPERM is treated as alive (process exists but owned by another UID / namespace).
// ESRCH (no such process) returns active=false (stale lock).
func (s *Session) IsActive() (bool, int, error) {
	if s == nil || s.BaseDir == "" {
		return false, 0, nil
	}
	lockPath := filepath.Join(s.BaseDir, "active")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil || pid <= 0 {
		return false, 0, nil
	}
	err = syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, pid, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, pid, nil
	}
	return false, pid, err
}

// ReleaseActiveLock removes the <baseDir>/active lockfile.
// Missing lockfile is not an error (already released). Defer-friendly.
func (s *Session) ReleaseActiveLock() error {
	if s == nil || s.BaseDir == "" {
		return nil
	}
	err := os.Remove(filepath.Join(s.BaseDir, "active"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

