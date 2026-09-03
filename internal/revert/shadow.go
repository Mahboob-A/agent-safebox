package revert

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// IsWhiteout checks if the given FileInfo represents an OverlayFS whiteout character device (major 0, minor 0).
func IsWhiteout(info os.FileInfo) bool {
	if info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return stat.Rdev == 0
}

func isWhiteout(info os.FileInfo) bool {
	return IsWhiteout(info)
}

// ScanShadowChanges walks upperDir and compares relative paths against lowerDir to classify changes as Added, Modified, or Deleted.
func ScanShadowChanges(lowerDir, upperDir string) ([]FileChange, error) {
	if upperDir == "" {
		return nil, errors.New("safebox: upper directory cannot be empty")
	}

	info, err := os.Stat(upperDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []FileChange{}, nil
		}
		return nil, fmt.Errorf("safebox: failed to stat upper directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("safebox: upper directory %q is not a directory", upperDir)
	}

	var changes []FileChange

	err = filepath.WalkDir(upperDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == upperDir {
			return nil
		}

		relPath, err := filepath.Rel(upperDir, path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Check for OverlayFS whiteout (deleted file)
		if isWhiteout(info) {
			changes = append(changes, FileChange{
				Path: relPath,
				Type: ChangeDeleted,
			})
			return nil
		}

		// If it's a directory, skip adding to changes (walk continues into children)
		if d.IsDir() {
			return nil
		}

		// Check if the file exists in lowerDir
		if lowerDir != "" {
			lowerFile := filepath.Join(lowerDir, relPath)
			if _, err := os.Stat(lowerFile); err == nil {
				// Exists in lowerDir: Modified
				changes = append(changes, FileChange{
					Path: relPath,
					Type: ChangeModified,
				})
				return nil
			}
		}

		// Does not exist in lowerDir: Added
		changes = append(changes, FileChange{
			Path: relPath,
			Type: ChangeAdded,
		})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("safebox: failed to walk shadow upper directory: %w", err)
	}

	// Sort changes by Path ascending
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})

	return changes, nil
}

// RunShadowDiff scans upperDir against lowerDir, formats the changes using Lipgloss styling, and writes them to out.
// If paths are supplied, only changes falling under those paths are displayed.
func RunShadowDiff(lowerDir, upperDir string, out io.Writer, paths ...string) error {
	changes, err := ScanShadowChanges(lowerDir, upperDir)
	if err != nil {
		return err
	}
	if len(paths) > 0 {
		changes = filterChanges(changes, paths, lowerDir)
	}
	formatted := FormatChanges(changes)
	_, err = fmt.Fprintln(out, formatted)
	return err
}
