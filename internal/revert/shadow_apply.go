package revert

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// ApplyShadowChanges discovers all changes in upperDir relative to lowerDir and synchronizes them to lowerDir.
// Added and modified files are copied to lowerDir; deleted whiteout files are removed from lowerDir.
func ApplyShadowChanges(lowerDir, upperDir string) error {
	if lowerDir == "" || upperDir == "" {
		return errors.New("safebox: lower and upper directories cannot be empty")
	}

	changes, err := ScanShadowChanges(lowerDir, upperDir)
	if err != nil {
		return fmt.Errorf("safebox: failed to scan shadow changes: %w", err)
	}

	for _, change := range changes {
		lowerPath := filepath.Join(lowerDir, change.Path)
		upperPath := filepath.Join(upperDir, change.Path)

		switch change.Type {
		case ChangeAdded, ChangeModified:
			info, err := os.Stat(upperPath)
			if err != nil {
				return fmt.Errorf("safebox: failed to stat shadow file %q: %w", change.Path, err)
			}
			if err := copyFile(upperPath, lowerPath, info.Mode().Perm()); err != nil {
				return fmt.Errorf("safebox: failed to copy shadow file %q to lower: %w", change.Path, err)
			}

		case ChangeDeleted:
			if err := os.Remove(lowerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("safebox: failed to remove deleted file %q from lower: %w", change.Path, err)
			}
		}
	}

	return nil
}

// DiscardShadowChanges purges the shadow workspace base directory and all its contents without altering lowerDir.
func DiscardShadowChanges(baseDir string) error {
	if baseDir == "" {
		return nil
	}
	return os.RemoveAll(baseDir)
}
