package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testBinaryPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "safebox_test_*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	testBinaryPath = filepath.Join(tmpDir, "safebox")
	buildCmd := exec.Command("go", "build", "-o", testBinaryPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		panic("failed to build test binary: " + string(out))
	}

	code := m.Run()
	os.Exit(code)
}

func runCLI(args ...string) ([]byte, error) {
	cmd := exec.Command(testBinaryPath, args...)
	cmd.Env = append(os.Environ(), "LANG=C")
	return cmd.CombinedOutput()
}

func TestCLIRunEmptyArgs(t *testing.T) {
	out, err := runCLI("run")
	if err == nil {
		t.Fatalf("expected error for empty run, got success with output: %s", string(out))
	}
	if !strings.Contains(string(out), "no command specified") {
		t.Errorf("expected 'no command specified' in output, got: %s", string(out))
	}
}

func TestCLIRunUIDMapping(t *testing.T) {
	out, err := runCLI("run", "--", "id", "-u")
	if err != nil {
		t.Fatalf("run id -u failed: %v, output: %s", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "0" {
		t.Errorf("expected container UID 0, got %s", strings.TrimSpace(string(out)))
	}
}

func TestCLIRunNetworkIsolation(t *testing.T) {
	out, err := runCLI("run", "--", "ping", "-c", "1", "-W", "1", "8.8.8.8")
	if err == nil {
		t.Fatalf("expected network ping to fail, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "Network is unreachable") {
		t.Errorf("expected 'Network is unreachable' in output, got: %s", string(out))
	}
}

func TestCLIRunLandlockDenial(t *testing.T) {
	out, err := runCLI("run", "--", "ls", "/root")
	if err == nil {
		t.Fatalf("expected ls /root to fail, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "Permission denied") {
		t.Errorf("expected 'Permission denied' in output, got: %s", string(out))
	}
}

func TestCLIRunExitCodePropagation(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "run", "--", "sh", "-c", "exit 42")
	cmd.Env = append(os.Environ(), "LANG=C")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code 42, got success")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 42 {
		t.Errorf("expected exit code 42, got %d", exitErr.ExitCode())
	}
}

func TestCLIUnknownSubcommand(t *testing.T) {
	out, err := runCLI("nosuchcommand")
	if err == nil {
		t.Fatal("expected non-zero exit for unknown subcommand")
	}
	if !strings.Contains(string(out), "unknown command") {
		t.Errorf("expected 'unknown command' in output, got: %s", string(out))
	}
}

func TestCLIHelpCommand(t *testing.T) {
	out, err := runCLI("help")
	if err != nil {
		t.Fatalf("help command failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Usage: safebox") {
		t.Errorf("expected usage header in help output, got: %s", string(out))
	}
}

func TestRunLatencyBudget(t *testing.T) {
	if os.Getenv("SKIP_LATENCY_TEST") != "" {
		t.Skip("latency test skipped via env")
	}

	start := time.Now()
	cmd := exec.Command(testBinaryPath, "run", "--", "true")
	cmd.Env = append(os.Environ(), "LANG=C")
	if err := cmd.Run(); err != nil {
		t.Fatalf("safebox run -- true failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("startup latency %v exceeds 200ms coarse budget (NFR3 target: 50ms)", elapsed)
	}
}

func runCLIInDir(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command(testBinaryPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LANG=C")
	return cmd.CombinedOutput()
}

func setupCLITestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
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

	initialFile := filepath.Join(dir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("initial content\n"), 0600); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}
	runGit("add", "initial.txt")
	runGit("commit", "-m", "initial commit")

	return dir
}

func TestCLIDiffCleanRepo(t *testing.T) {
	dir := setupCLITestGitRepo(t)
	out, err := runCLIInDir(dir, "diff")
	if err != nil {
		t.Fatalf("diff failed on clean repo: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Working tree is clean. No changes detected.") {
		t.Errorf("expected clean working tree message, got: %s", string(out))
	}
}

func TestCLIDiffWithChanges(t *testing.T) {
	dir := setupCLITestGitRepo(t)

	// Create new untracked file
	newFile := filepath.Join(dir, "created.txt")
	if err := os.WriteFile(newFile, []byte("new file\n"), 0600); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}

	// Modify committed file
	initialFile := filepath.Join(dir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("modified content\n"), 0600); err != nil {
		t.Fatalf("failed to write modified file: %v", err)
	}

	out, err := runCLIInDir(dir, "diff")
	if err != nil {
		t.Fatalf("diff failed on modified repo: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "+ [ADDED]") || !strings.Contains(string(out), "created.txt") {
		t.Errorf("expected '+ [ADDED] created.txt' in output, got: %s", string(out))
	}
	if !strings.Contains(string(out), "~ [MODIFIED]") || !strings.Contains(string(out), "initial.txt") {
		t.Errorf("expected '~ [MODIFIED] initial.txt' in output, got: %s", string(out))
	}
}

func TestCLIDiffNonGitDirectory(t *testing.T) {
	nonGitDir := t.TempDir()
	out, err := runCLIInDir(nonGitDir, "diff")
	if err == nil {
		t.Fatalf("expected diff to fail in non-git dir, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "not a git repository") {
		t.Errorf("expected 'not a git repository' in error output, got: %s", string(out))
	}
}

func TestCLISandboxRunAndDiff(t *testing.T) {
	dir := setupCLITestGitRepo(t)

	// Create another file to be committed and deleted
	toDeleteFile := filepath.Join(dir, "to_delete.txt")
	if err := os.WriteFile(toDeleteFile, []byte("will be deleted\n"), 0600); err != nil {
		t.Fatalf("failed to write to_delete.txt: %v", err)
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
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
	runGit("add", "to_delete.txt")
	runGit("commit", "-m", "add to_delete")

	// Run sandboxed command that creates, modifies, and deletes files inside the working tree
	runCmdScript := "touch created_in_sandbox.txt && echo 'modified' >> initial.txt && rm to_delete.txt"
	runOut, runErr := runCLIInDir(dir, "run", "--", "sh", "-c", runCmdScript)
	if runErr != nil {
		t.Fatalf("safebox run failed: %v, output: %s", runErr, string(runOut))
	}

	// Verify post-run change visibility via safebox diff
	diffOut, diffErr := runCLIInDir(dir, "diff")
	if diffErr != nil {
		t.Fatalf("safebox diff failed: %v, output: %s", diffErr, string(diffOut))
	}
	diffStr := string(diffOut)
	if !strings.Contains(diffStr, "+ [ADDED]") || !strings.Contains(diffStr, "created_in_sandbox.txt") {
		t.Errorf("expected added created_in_sandbox.txt, got: %s", diffStr)
	}
	if !strings.Contains(diffStr, "~ [MODIFIED]") || !strings.Contains(diffStr, "initial.txt") {
		t.Errorf("expected modified initial.txt, got: %s", diffStr)
	}
	if !strings.Contains(diffStr, "- [DELETED]") || !strings.Contains(diffStr, "to_delete.txt") {
		t.Errorf("expected deleted to_delete.txt, got: %s", diffStr)
	}
}

func TestCLIDiffFromSubdirectory(t *testing.T) {
	dir := setupCLITestGitRepo(t)
	subDir := filepath.Join(dir, "sub", "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	nestedFile := filepath.Join(subDir, "nested_file.txt")
	if err := os.WriteFile(nestedFile, []byte("nested content\n"), 0600); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	out, err := runCLIInDir(subDir, "diff")
	if err != nil {
		t.Fatalf("diff from subdir failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "+ [ADDED]") || !strings.Contains(string(out), "nested_file.txt") {
		t.Errorf("expected '+ [ADDED]' with nested_file.txt in output, got: %s", string(out))
	}
}
