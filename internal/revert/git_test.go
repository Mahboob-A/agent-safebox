package revert

import (
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := osexec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v, output: %s", args, err, string(out))
	}
}

func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	// Create initial commit
	initialFile := filepath.Join(dir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("initial content\n"), 0600); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}
	runGit(t, dir, "add", "initial.txt")
	runGit(t, dir, "commit", "-m", "initial commit")

	return dir
}

func TestIsGitRepo(t *testing.T) {
	nonGitDir := t.TempDir()
	isRepo, err := IsGitRepo(nonGitDir)
	if err != nil {
		t.Fatalf("unexpected error for non-git dir: %v", err)
	}
	if isRepo {
		t.Errorf("expected false for non-git dir, got true")
	}

	gitDir := setupTestGitRepo(t)
	isRepo, err = IsGitRepo(gitDir)
	if err != nil {
		t.Fatalf("unexpected error for git dir: %v", err)
	}
	if !isRepo {
		t.Errorf("expected true for git dir, got false")
	}
}

func TestGetStatusNonGitRepo(t *testing.T) {
	nonGitDir := t.TempDir()
	_, err := GetStatus(nonGitDir)
	if err == nil {
		t.Fatal("expected error for non-git repo, got nil")
	}
	if !errors.Is(err, ErrNotGitRepo) {
		t.Errorf("expected ErrNotGitRepo, got: %v", err)
	}
}

func TestGetStatusCleanRepo(t *testing.T) {
	gitDir := setupTestGitRepo(t)
	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("unexpected error on clean repo: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes on clean repo, got %d: %+v", len(changes), changes)
	}
}

func TestGetStatusChanges(t *testing.T) {
	gitDir := setupTestGitRepo(t)

	// 1. Untracked file
	untrackedFile := filepath.Join(gitDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("new file\n"), 0600); err != nil {
		t.Fatalf("failed to write untracked file: %v", err)
	}

	// 2. Modified committed file
	modifiedFile := filepath.Join(gitDir, "initial.txt")
	if err := os.WriteFile(modifiedFile, []byte("modified content\n"), 0600); err != nil {
		t.Fatalf("failed to write modified file: %v", err)
	}

	// 3. Deleted committed file
	toBeDeletedFile := filepath.Join(gitDir, "delete_me.txt")
	if err := os.WriteFile(toBeDeletedFile, []byte("to be deleted\n"), 0600); err != nil {
		t.Fatalf("failed to write delete_me file: %v", err)
	}
	runGit(t, gitDir, "add", "delete_me.txt")
	runGit(t, gitDir, "commit", "-m", "add delete_me")
	if err := os.Remove(toBeDeletedFile); err != nil {
		t.Fatalf("failed to remove delete_me file: %v", err)
	}

	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d: %+v", len(changes), changes)
	}

	changeMap := make(map[string]FileChange)
	for _, c := range changes {
		changeMap[c.Path] = c
	}

	if changeMap["untracked.txt"].Type != ChangeUntracked || changeMap["untracked.txt"].StatusCode != "??" {
		t.Errorf("unexpected untracked change: %+v", changeMap["untracked.txt"])
	}
	if changeMap["initial.txt"].Type != ChangeModified || changeMap["initial.txt"].StatusCode != " M" {
		t.Errorf("unexpected modified change: %+v", changeMap["initial.txt"])
	}
	if changeMap["delete_me.txt"].Type != ChangeDeleted || !strings.Contains(changeMap["delete_me.txt"].StatusCode, "D") {
		t.Errorf("unexpected deleted change: %+v", changeMap["delete_me.txt"])
	}
}

func TestGetStatusStagedAdded(t *testing.T) {
	gitDir := setupTestGitRepo(t)
	stagedFile := filepath.Join(gitDir, "staged.txt")
	if err := os.WriteFile(stagedFile, []byte("staged content\n"), 0600); err != nil {
		t.Fatalf("failed to write staged file: %v", err)
	}
	runGit(t, gitDir, "add", "staged.txt")

	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Path != "staged.txt" || changes[0].Type != ChangeAdded || changes[0].StatusCode != "A " {
		t.Errorf("unexpected staged change: %+v", changes[0])
	}
}

func TestGetStatusRename(t *testing.T) {
	gitDir := setupTestGitRepo(t)
	runGit(t, gitDir, "mv", "initial.txt", "renamed.txt")

	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Path != "renamed.txt" || changes[0].Type != ChangeModified || changes[0].StatusCode != "R " {
		t.Errorf("unexpected rename change: %+v", changes[0])
	}
}

func TestGetStatusUTF8(t *testing.T) {
	gitDir := setupTestGitRepo(t)
	utf8File := filepath.Join(gitDir, "файл.txt")
	if err := os.WriteFile(utf8File, []byte("utf8 content\n"), 0600); err != nil {
		t.Fatalf("failed to write utf8 file: %v", err)
	}

	changes, err := GetStatus(gitDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Type != ChangeUntracked || changes[0].Path != "файл.txt" {
		t.Errorf("expected ChangeUntracked with unquoted path 'файл.txt', got %+v", changes[0])
	}
}
