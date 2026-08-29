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

	// Test repo with a new file and a deleted file
	buf.Reset()
	newFile := filepath.Join(gitDir, "demo.txt")
	if err := os.WriteFile(newFile, []byte("demo\n"), 0600); err != nil {
		t.Fatalf("failed to write demo.txt: %v", err)
	}
	initialFile := filepath.Join(gitDir, "initial.txt")
	if err := os.Remove(initialFile); err != nil {
		t.Fatalf("failed to remove initial.txt: %v", err)
	}

	if err := RunDiff(gitDir, &buf); err != nil {
		t.Fatalf("RunDiff failed on dirty repo: %v", err)
	}
	if !strings.Contains(buf.String(), "+ [ADDED]") || !strings.Contains(buf.String(), "demo.txt") {
		t.Errorf("expected '+ [ADDED] demo.txt' in output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "- [DELETED]") || !strings.Contains(buf.String(), "initial.txt") {
		t.Errorf("expected '- [DELETED] initial.txt' in output, got: %s", buf.String())
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

func setupGitRepoWithThreeChanges(t *testing.T) string {
	gitDir := setupTestGitRepo(t)

	// Change 1: root file modified
	if err := os.WriteFile(filepath.Join(gitDir, "a.txt"), []byte("modified a\n"), 0600); err != nil {
		t.Fatalf("failed to write a.txt: %v", err)
	}
	// Change 2: subdir file added
	if err := os.MkdirAll(filepath.Join(gitDir, "subdir", "nested"), 0700); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "subdir", "b.txt"), []byte("added b\n"), 0600); err != nil {
		t.Fatalf("failed to write b.txt: %v", err)
	}
	// Change 3: nested file added
	if err := os.WriteFile(filepath.Join(gitDir, "subdir", "nested", "c.txt"), []byte("added c\n"), 0600); err != nil {
		t.Fatalf("failed to write c.txt: %v", err)
	}
	return gitDir
}

func TestRunDiff_PathFilterRelativePaths(t *testing.T) {
	gitDir := setupGitRepoWithThreeChanges(t)

	// 1. Relative exact match
	var buf bytes.Buffer
	if err := RunDiff(gitDir, &buf, "subdir/b.txt"); err != nil {
		t.Fatalf("RunDiff failed: %v", err)
	}
	if !strings.Contains(buf.String(), "subdir/b.txt") {
		t.Errorf("expected subdir/b.txt in output, got: %s", buf.String())
	}
	if strings.Contains(buf.String(), "a.txt") || strings.Contains(buf.String(), "c.txt") {
		t.Errorf("unexpected files in relative exact match output: %s", buf.String())
	}

	// 2. Relative prefix match
	buf.Reset()
	if err := RunDiff(gitDir, &buf, "subdir"); err != nil {
		t.Fatalf("RunDiff failed: %v", err)
	}
	if !strings.Contains(buf.String(), "subdir/b.txt") || !strings.Contains(buf.String(), "subdir/nested/c.txt") {
		t.Errorf("expected subdir children in output, got: %s", buf.String())
	}
	if strings.Contains(buf.String(), "a.txt") {
		t.Errorf("unexpected a.txt in relative prefix match output: %s", buf.String())
	}
}

func TestRunDiff_PathFilterAbsolutePaths(t *testing.T) {
	gitDir := setupGitRepoWithThreeChanges(t)

	// 1. Absolute exact match
	var buf bytes.Buffer
	absB := filepath.Join(gitDir, "subdir", "b.txt")
	if err := RunDiff(gitDir, &buf, absB); err != nil {
		t.Fatalf("RunDiff failed: %v", err)
	}
	if !strings.Contains(buf.String(), "subdir/b.txt") {
		t.Errorf("expected subdir/b.txt in output, got: %s", buf.String())
	}
	if strings.Contains(buf.String(), "a.txt") {
		t.Errorf("unexpected a.txt in absolute exact match output: %s", buf.String())
	}

	// 2. Absolute prefix match
	buf.Reset()
	absSubdir := filepath.Join(gitDir, "subdir")
	if err := RunDiff(gitDir, &buf, absSubdir); err != nil {
		t.Fatalf("RunDiff failed: %v", err)
	}
	if !strings.Contains(buf.String(), "subdir/b.txt") || !strings.Contains(buf.String(), "subdir/nested/c.txt") {
		t.Errorf("expected subdir children in output, got: %s", buf.String())
	}

	// 3. Absolute path outside baseDir returns clean diff (no error)
	buf.Reset()
	if err := RunDiff(gitDir, &buf, "/tmp/nonexistent-outside-dir"); err != nil {
		t.Fatalf("RunDiff failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Working tree is clean") {
		t.Errorf("expected clean working tree message for outside path, got: %s", buf.String())
	}
}

func TestRunDiff_PathFilterEmptyPath(t *testing.T) {
	gitDir := setupGitRepoWithThreeChanges(t)

	// Empty path slice returns all changes
	var buf bytes.Buffer
	if err := RunDiff(gitDir, &buf); err != nil {
		t.Fatalf("RunDiff failed: %v", err)
	}
	if !strings.Contains(buf.String(), "a.txt") || !strings.Contains(buf.String(), "subdir/b.txt") || !strings.Contains(buf.String(), "subdir/nested/c.txt") {
		t.Errorf("expected all 3 changes in unfiltered diff, got: %s", buf.String())
	}

	// Single empty string "" returns all changes
	buf.Reset()
	if err := RunDiff(gitDir, &buf, ""); err != nil {
		t.Fatalf("RunDiff with empty string failed: %v", err)
	}
	if !strings.Contains(buf.String(), "a.txt") || !strings.Contains(buf.String(), "subdir/b.txt") {
		t.Errorf("expected all changes with empty string path, got: %s", buf.String())
	}
}
