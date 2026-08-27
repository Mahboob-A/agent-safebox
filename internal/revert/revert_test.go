package revert

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRevertForced(t *testing.T) {
	gitDir := setupTestGitRepo(t)

	// 1. Modify initial.txt
	initialFile := filepath.Join(gitDir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("dirty content\n"), 0600); err != nil {
		t.Fatalf("failed to write dirty file: %v", err)
	}

	// 2. Create untracked file
	untrackedFile := filepath.Join(gitDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked content\n"), 0600); err != nil {
		t.Fatalf("failed to write untracked file: %v", err)
	}

	// 3. Create untracked nested directory and file
	nestedDir := filepath.Join(gitDir, "sub", "dir")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	nestedFile := filepath.Join(nestedDir, "nested.txt")
	if err := os.WriteFile(nestedFile, []byte("nested\n"), 0600); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	var buf bytes.Buffer
	if err := Revert(gitDir, true, nil, &buf); err != nil {
		t.Fatalf("Revert failed: %v", err)
	}

	if !strings.Contains(buf.String(), "Working tree restored.") {
		t.Errorf("expected success message in output, got: %s", buf.String())
	}

	// Verify working tree is clean
	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes after revert, got %d: %+v", len(changes), changes)
	}

	// Verify content of initial.txt is restored
	content, err := os.ReadFile(initialFile)
	if err != nil {
		t.Fatalf("failed to read initial.txt: %v", err)
	}
	if string(content) != "initial content\n" {
		t.Errorf("expected restored content 'initial content\\n', got: %s", string(content))
	}

	// Verify untracked files are gone
	if _, err := os.Stat(untrackedFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected untracked.txt to be removed, err: %v", err)
	}
	if _, err := os.Stat(nestedDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected nested directory to be removed, err: %v", err)
	}
}

func TestRevertConfirmedYes(t *testing.T) {
	gitDir := setupTestGitRepo(t)

	initialFile := filepath.Join(gitDir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("changed\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	var buf bytes.Buffer
	in := strings.NewReader("y\n")
	if err := Revert(gitDir, false, in, &buf); err != nil {
		t.Fatalf("Revert failed on 'y' confirmation: %v", err)
	}

	if !strings.Contains(buf.String(), "Are you sure you want to discard") {
		t.Errorf("expected confirmation prompt in output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "Working tree restored.") {
		t.Errorf("expected success message, got: %s", buf.String())
	}

	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes after confirmed revert, got %d", len(changes))
	}
}

func TestRevertConfirmedYesLong(t *testing.T) {
	gitDir := setupTestGitRepo(t)

	initialFile := filepath.Join(gitDir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("changed\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	var buf bytes.Buffer
	in := strings.NewReader("YES\n")
	if err := Revert(gitDir, false, in, &buf); err != nil {
		t.Fatalf("Revert failed on 'YES' confirmation: %v", err)
	}

	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes after confirmed revert, got %d", len(changes))
	}
}

func TestRevertCancelledNo(t *testing.T) {
	gitDir := setupTestGitRepo(t)

	initialFile := filepath.Join(gitDir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("dirty content\n"), 0600); err != nil {
		t.Fatalf("failed to write dirty file: %v", err)
	}

	var buf bytes.Buffer
	in := strings.NewReader("n\n")
	err := Revert(gitDir, false, in, &buf)
	if err == nil {
		t.Fatal("expected ErrRevertCancelled, got nil")
	}
	if !errors.Is(err, ErrRevertCancelled) {
		t.Errorf("expected ErrRevertCancelled, got: %v", err)
	}

	if !strings.Contains(buf.String(), "Revert cancelled. Pass --yes to skip confirmation.") {
		t.Errorf("expected cancellation message, got: %s", buf.String())
	}

	// Verify working tree modifications are preserved
	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 change remaining after cancellation, got %d", len(changes))
	}
}

func TestRevertCancelledEmptyOrEOF(t *testing.T) {
	gitDir := setupTestGitRepo(t)

	// Test empty line
	var buf bytes.Buffer
	err := Revert(gitDir, false, strings.NewReader("\n"), &buf)
	if !errors.Is(err, ErrRevertCancelled) {
		t.Errorf("expected ErrRevertCancelled on empty input, got: %v", err)
	}

	// Test nil reader
	buf.Reset()
	err = Revert(gitDir, false, nil, &buf)
	if !errors.Is(err, ErrRevertCancelled) {
		t.Errorf("expected ErrRevertCancelled on nil reader, got: %v", err)
	}
}

func TestRevertNonGitRepo(t *testing.T) {
	nonGitDir := t.TempDir()
	var buf bytes.Buffer
	err := Revert(nonGitDir, true, nil, &buf)
	if err == nil {
		t.Fatal("expected error on non-git dir, got nil")
	}
	if !errors.Is(err, ErrNotGitRepo) {
		t.Errorf("expected ErrNotGitRepo, got: %v", err)
	}
}

func TestRevertCleanRepo(t *testing.T) {
	gitDir := setupTestGitRepo(t)
	var buf bytes.Buffer
	if err := Revert(gitDir, true, nil, &buf); err != nil {
		t.Fatalf("Revert failed on clean repo: %v", err)
	}
	if !strings.Contains(buf.String(), "Working tree restored.") {
		t.Errorf("expected 'Working tree restored.', got: %s", buf.String())
	}
}
