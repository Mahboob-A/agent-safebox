package revert

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// copyFile copies data from src to dst preserving file permissions.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func isEXDEV(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return errors.Is(linkErr.Err, syscall.EXDEV)
	}
	return false
}

func removeLower(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if errors.Is(pathErr.Err, syscall.EISDIR) ||
			errors.Is(pathErr.Err, syscall.ENOTEMPTY) ||
			errors.Is(pathErr.Err, syscall.EEXIST) {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				return fmt.Errorf("removeLower: %w (fallback from %v)", rmErr, err)
			}
			return nil
		}
	}
	return err
}

func cleanupStaging(stagingDir string) {
	if err := os.RemoveAll(stagingDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "[safebox] warning: failed to clean up staging directory %q: %v\n", stagingDir, err)
	}
}

// ApplyShadowChanges discovers all changes in upperDir relative to lowerDir and synchronizes them to lowerDir.
// Changes are verified and copied to an isolated staging directory under SessionRoot before being atomically
// renamed onto lowerDir paths, preventing partial corruption of lowerDir if preparation fails.
func ApplyShadowChanges(lowerDir, upperDir string) error {
	if lowerDir == "" || upperDir == "" {
		return errors.New("safebox: lower and upper directories cannot be empty")
	}

	changes, err := ScanShadowChanges(lowerDir, upperDir)
	if err != nil {
		return fmt.Errorf("safebox: failed to scan shadow changes: %w", err)
	}
	if len(changes) == 0 {
		return nil
	}

	sessionID := filepath.Base(filepath.Dir(upperDir))
	if sessionID == "" || sessionID == "." || sessionID == string(filepath.Separator) {
		sessionID = fmt.Sprintf("sess-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	stagingDir := filepath.Join(SessionRoot(), "apply-staging", sessionID)
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return fmt.Errorf("safebox: failed to create apply staging directory: %w", err)
	}

	// Stage all Added and Modified files
	for _, change := range changes {
		if change.Type == ChangeAdded || change.Type == ChangeModified {
			upperPath := filepath.Join(upperDir, change.Path)
			stagingPath := filepath.Join(stagingDir, change.Path)

			info, err := os.Stat(upperPath)
			if err != nil {
				cleanupStaging(stagingDir)
				return fmt.Errorf("safebox: failed to stat shadow file %q: %w", change.Path, err)
			}
			if err := copyFile(upperPath, stagingPath, info.Mode().Perm()); err != nil {
				cleanupStaging(stagingDir)
				return fmt.Errorf("safebox: failed to stage shadow file %q: %w", change.Path, err)
			}
		}
	}

	// Apply verified changes to lowerDir
	for _, change := range changes {
		lowerPath := filepath.Join(lowerDir, change.Path)
		stagingPath := filepath.Join(stagingDir, change.Path)

		switch change.Type {
		case ChangeAdded, ChangeModified:
			if err := os.MkdirAll(filepath.Dir(lowerPath), 0755); err != nil {
				cleanupStaging(stagingDir)
				return fmt.Errorf("safebox: failed to create directory for %q: %w", change.Path, err)
			}

			if err := os.Rename(stagingPath, lowerPath); err != nil {
				if isEXDEV(err) {
					// Cross-filesystem link fallback: copy to temp sibling, fsync, and atomic rename
					info, statErr := os.Stat(stagingPath)
					if statErr != nil {
						cleanupStaging(stagingDir)
						return fmt.Errorf("safebox: failed to stat staged file %q: %w", change.Path, statErr)
					}
					tempSibling := filepath.Join(filepath.Dir(lowerPath), fmt.Sprintf(".safebox-tmp-%s-%d-%d", filepath.Base(lowerPath), os.Getpid(), time.Now().UnixNano()))
					if copyErr := copyFile(stagingPath, tempSibling, info.Mode().Perm()); copyErr != nil {
						_ = os.Remove(tempSibling)
						cleanupStaging(stagingDir)
						return fmt.Errorf("safebox: failed to copy across devices for %q: %w", change.Path, copyErr)
					}
					if renErr := os.Rename(tempSibling, lowerPath); renErr != nil {
						_ = os.Remove(tempSibling)
						cleanupStaging(stagingDir)
						return fmt.Errorf("safebox: failed to rename temp file for %q: %w", change.Path, renErr)
					}
				} else {
					cleanupStaging(stagingDir)
					return fmt.Errorf("safebox: failed to rename staged file %q to lower: %w", change.Path, err)
				}
			} else {
				if dirFD, dErr := os.Open(filepath.Dir(lowerPath)); dErr == nil {
					if sErr := dirFD.Sync(); sErr != nil {
						fmt.Fprintf(os.Stderr, "[safebox] warning: failed to fsync directory %q: %v\n", filepath.Dir(lowerPath), sErr)
					}
					_ = dirFD.Close()
				}
			}

		case ChangeDeleted:
			if err := removeLower(lowerPath); err != nil {
				cleanupStaging(stagingDir)
				return fmt.Errorf("safebox: failed to remove deleted file %q from lower: %w", change.Path, err)
			}
			if dirFD, dErr := os.Open(filepath.Dir(lowerPath)); dErr == nil {
				if sErr := dirFD.Sync(); sErr != nil {
					fmt.Fprintf(os.Stderr, "[safebox] warning: failed to fsync directory %q: %v\n", filepath.Dir(lowerPath), sErr)
				}
				_ = dirFD.Close()
			}
		}
	}

	cleanupStaging(stagingDir)
	return nil
}

// DiscardShadowChanges purges the shadow workspace base directory and all its contents without altering lowerDir.
func DiscardShadowChanges(baseDir string) error {
	if baseDir == "" {
		return nil
	}
	return os.RemoveAll(baseDir)
}
