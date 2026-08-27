package revert

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safebox/internal/ui"
)

func TestFormatChangesClean(t *testing.T) {
	out := FormatChanges([]FileChange{})
	expected := ui.StyleMeta.Render("Working tree is clean. No changes detected.")
	if out != expected {
		t.Errorf("expected clean message %q, got %q", expected, out)
	}
}

func TestFormatChangesTypes(t *testing.T) {
	changes := []FileChange{
		{Path: "new.txt", Type: ChangeAdded, StatusCode: "A "},
		{Path: "untracked.txt", Type: ChangeUntracked, StatusCode: "??"},
		{Path: "edit.txt", Type: ChangeModified, StatusCode: " M"},
		{Path: "remove.txt", Type: ChangeDeleted, StatusCode: " D"},
	}

	out := FormatChanges(changes)

	if !strings.Contains(out, "+ [ADDED]") {
		t.Errorf("expected '+ [ADDED]' in output, got: %s", out)
	}
	if !strings.Contains(out, "new.txt") || !strings.Contains(out, "untracked.txt") {
		t.Errorf("expected added file paths in output, got: %s", out)
	}
	if !strings.Contains(out, "~ [MODIFIED]") || !strings.Contains(out, "edit.txt") {
		t.Errorf("expected modified file path in output, got: %s", out)
	}
	if !strings.Contains(out, "- [DELETED]") || !strings.Contains(out, "remove.txt") {
		t.Errorf("expected deleted file path in output, got: %s", out)
	}
}

func TestRunDiff(t *testing.T) {
	gitDir := setupTestGitRepo(t)

	// Test clean repo
	var buf bytes.Buffer
	if err := RunDiff(gitDir, &buf); err != nil {
		t.Fatalf("RunDiff failed on clean repo: %v", err)
	}
	if !strings.Contains(buf.String(), "Working tree is clean") {
		t.Errorf("expected clean working tree message, got: %s", buf.String())
	}

	// Test repo with a new file
	buf.Reset()
	newFile := filepath.Join(gitDir, "demo.txt")
	if err := os.WriteFile(newFile, []byte("demo\n"), 0600); err != nil {
		t.Fatalf("failed to write demo.txt: %v", err)
	}

	if err := RunDiff(gitDir, &buf); err != nil {
		t.Fatalf("RunDiff failed on dirty repo: %v", err)
	}
	if !strings.Contains(buf.String(), "+ [ADDED]") || !strings.Contains(buf.String(), "demo.txt") {
		t.Errorf("expected '+ [ADDED] demo.txt' in output, got: %s", buf.String())
	}

	// Test non-git repo
	nonGitDir := t.TempDir()
	buf.Reset()
	err := RunDiff(nonGitDir, &buf)
	if err == nil {
		t.Fatal("expected error on non-git dir, got nil")
	}
	if !errors.Is(err, ErrNotGitRepo) {
		t.Errorf("expected ErrNotGitRepo, got: %v", err)
	}
}
