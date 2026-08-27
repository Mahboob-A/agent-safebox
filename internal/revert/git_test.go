package revert

import (
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"
)

func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit := func(args ...string) {
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

	runGit("init")
	runGit("config", "user.name", "Test User")
	runGit("config", "user.email", "test@example.com")

	// Create initial commit
	initialFile := filepath.Join(dir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("initial content\n"), 0600); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}
	runGit("add", "initial.txt")
	runGit("commit", "-m", "initial commit")

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
	addCmd := osexec.Command("git", "-C", gitDir, "add", "delete_me.txt")
	if err := addCmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	commitCmd := osexec.Command("git", "-C", gitDir, "commit", "-m", "add delete_me")
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
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

	changeMap := make(map[string]FileChangeType)
	for _, c := range changes {
		changeMap[c.Path] = c.Type
	}

	if changeMap["untracked.txt"] != ChangeUntracked {
		t.Errorf("expected untracked.txt to be ChangeUntracked, got %v", changeMap["untracked.txt"])
	}
	if changeMap["initial.txt"] != ChangeModified {
		t.Errorf("expected initial.txt to be ChangeModified, got %v", changeMap["initial.txt"])
	}
	if changeMap["delete_me.txt"] != ChangeDeleted {
		t.Errorf("expected delete_me.txt to be ChangeDeleted, got %v", changeMap["delete_me.txt"])
	}
}
