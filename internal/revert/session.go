package revert

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	// ErrNoSessionFound is returned when no active session matches the requested directory.
	ErrNoSessionFound = errors.New("safebox: no active session found for directory")
)

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
func MostRecentSession(targetDir string) (*Session, error) {
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

	var matching []*Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessDir := filepath.Join(root, entry.Name())
		sess, err := LoadSession(sessDir)
		if err != nil {
			continue
		}
		if sess.LowerDir == absTarget || strings.HasPrefix(absTarget, sess.LowerDir+string(filepath.Separator)) {
			matching = append(matching, sess)
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
