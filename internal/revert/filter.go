package revert

import (
	"path/filepath"
	"strings"
)

// filterChanges returns the subset of changes whose paths fall under one of
// the supplied filter paths, after normalizing both sides to be relative to
// baseDir. An empty paths slice (or paths containing only "" or ".") returns
// changes unchanged.
func filterChanges(changes []FileChange, paths []string, baseDir string) []FileChange {
	if len(paths) == 0 {
		return changes
	}

	cleanRelPaths := make([]string, 0, len(paths))
	hasSpecificFilter := false
	for _, p := range paths {
		if p == "" || p == "." {
			continue
		}
		var cleanP string
		if filepath.IsAbs(p) {
			if rel, err := filepath.Rel(baseDir, p); err == nil &&
				!strings.HasPrefix(rel, "..") && rel != ".." {
				cleanP = filepath.Clean(rel)
			} else {
				hasSpecificFilter = true
				continue
			}
		} else {
			cleanP = filepath.Clean(p)
		}
		if cleanP == "." || cleanP == "" {
			continue
		}
		hasSpecificFilter = true
		cleanRelPaths = append(cleanRelPaths, cleanP)
	}

	if !hasSpecificFilter {
		return changes
	}
	if len(cleanRelPaths) == 0 {
		return []FileChange{}
	}

	filtered := []FileChange{}
	for _, c := range changes {
		var cleanRelC string
		if filepath.IsAbs(c.Path) {
			rel, err := filepath.Rel(baseDir, c.Path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			cleanRelC = filepath.Clean(rel)
		} else {
			cleanRelC = filepath.Clean(c.Path)
		}
		if cleanRelC == "." {
			cleanRelC = ""
		}

		for _, cleanP := range cleanRelPaths {
			if cleanRelC == cleanP ||
				strings.HasPrefix(cleanRelC, cleanP+string(filepath.Separator)) {
				filtered = append(filtered, c)
				break
			}
		}
	}
	return filtered
}
