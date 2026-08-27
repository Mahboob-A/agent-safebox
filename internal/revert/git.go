package revert

import (
	"bytes"
	"errors"
	"fmt"
	osexec "os/exec"
	"strings"
)

type FileChangeType string

const (
	ChangeAdded     FileChangeType = "ADDED"
	ChangeModified  FileChangeType = "MODIFIED"
	ChangeDeleted   FileChangeType = "DELETED"
	ChangeUntracked FileChangeType = "UNTRACKED"
)

type FileChange struct {
	Path       string
	Type       FileChangeType
	StatusCode string
}

var ErrNotGitRepo = errors.New("not a git repository (or any of the parent directories)")

// IsGitRepo reports whether the given directory is inside a valid git working tree.
func IsGitRepo(dir string) (bool, error) {
	cmd := osexec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	err := cmd.Run()
	if err != nil {
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, fmt.Errorf("safebox: failed to check git repository: %w", err)
	}
	return true, nil
}

// GetStatus queries git status --porcelain=v1 -uall and returns structured file changes.
func GetStatus(dir string) ([]FileChange, error) {
	isRepo, err := IsGitRepo(dir)
	if err != nil {
		return nil, err
	}
	if !isRepo {
		return nil, ErrNotGitRepo
	}

	cmd := osexec.Command("git", "-C", dir, "status", "--porcelain=v1", "-uall")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("safebox: git status failed: %w", err)
	}

	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return []FileChange{}, nil
	}

	var changes []FileChange
	lines := strings.Split(string(trimmed), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		statusCode := line[0:2]
		filePath := strings.TrimSpace(line[2:])

		// Handle renamed files (e.g. R  old.txt -> new.txt)
		if idx := strings.Index(filePath, " -> "); idx != -1 {
			filePath = filePath[idx+4:]
		}

		var changeType FileChangeType
		switch {
		case statusCode == "??":
			changeType = ChangeUntracked
		case statusCode == "A " || statusCode == "AM" || statusCode == "AD":
			changeType = ChangeAdded
		case strings.Contains(statusCode, "D"):
			changeType = ChangeDeleted
		case strings.Contains(statusCode, "M") || strings.Contains(statusCode, "R"):
			changeType = ChangeModified
		default:
			changeType = ChangeModified
		}

		changes = append(changes, FileChange{
			Path:       filePath,
			Type:       changeType,
			StatusCode: statusCode,
		})
	}
	return changes, nil
}
