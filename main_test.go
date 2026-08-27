package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func runCLIInDirWithStdin(dir string, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.Command(testBinaryPath, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Env = append(os.Environ(), "LANG=C")
	return cmd.CombinedOutput()
}

func runCLIInDir(dir string, args ...string) ([]byte, error) {
	return runCLIInDirWithStdin(dir, nil, args...)
}

func runGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
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

func setupCLITestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGitInDir(t, dir, "init")
	runGitInDir(t, dir, "config", "user.name", "Test User")
	runGitInDir(t, dir, "config", "user.email", "test@example.com")

	initialFile := filepath.Join(dir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("initial content\n"), 0600); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}
	runGitInDir(t, dir, "add", "initial.txt")
	runGitInDir(t, dir, "commit", "-m", "initial commit")

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
	runGitInDir(t, dir, "add", "to_delete.txt")
	runGitInDir(t, dir, "commit", "-m", "add to_delete")

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

func TestCLIRevertCleanRepo(t *testing.T) {
	dir := setupCLITestGitRepo(t)
	out, err := runCLIInDir(dir, "revert", "--yes")
	if err != nil {
		t.Fatalf("revert --yes failed on clean repo: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Working tree restored.") {
		t.Errorf("expected 'Working tree restored.' in output, got: %s", string(out))
	}
}

func TestCLIRevertWithChangesForced(t *testing.T) {
	dir := setupCLITestGitRepo(t)

	// Dirty the repo
	newFile := filepath.Join(dir, "created.txt")
	if err := os.WriteFile(newFile, []byte("untracked\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	initialFile := filepath.Join(dir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("modified\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Test -y shorthand
	out, err := runCLIInDir(dir, "revert", "-y")
	if err != nil {
		t.Fatalf("revert -y failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Working tree restored.") {
		t.Errorf("expected 'Working tree restored.' in output, got: %s", string(out))
	}

	// Verify working tree is clean via diff
	diffOut, err := runCLIInDir(dir, "diff")
	if err != nil {
		t.Fatalf("diff after revert failed: %v, output: %s", err, string(diffOut))
	}
	if !strings.Contains(string(diffOut), "Working tree is clean") {
		t.Errorf("expected clean working tree after revert, got: %s", string(diffOut))
	}
}

func TestCLIRevertConfirmationYes(t *testing.T) {
	dir := setupCLITestGitRepo(t)

	newFile := filepath.Join(dir, "created.txt")
	if err := os.WriteFile(newFile, []byte("untracked\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	out, err := runCLIInDirWithStdin(dir, strings.NewReader("y\n"), "revert")
	if err != nil {
		t.Fatalf("revert with 'y' failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Are you sure you want to discard") {
		t.Errorf("expected confirmation prompt in output, got: %s", string(out))
	}
	if !strings.Contains(string(out), "Working tree restored.") {
		t.Errorf("expected 'Working tree restored.' in output, got: %s", string(out))
	}

	// Verify file is removed
	if _, err := os.Stat(newFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected untracked file to be deleted, err: %v", err)
	}
}

func TestCLIRevertConfirmationNo(t *testing.T) {
	dir := setupCLITestGitRepo(t)

	newFile := filepath.Join(dir, "created.txt")
	if err := os.WriteFile(newFile, []byte("untracked\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	out, err := runCLIInDirWithStdin(dir, strings.NewReader("n\n"), "revert")
	if err != nil {
		t.Fatalf("revert with 'n' should exit cleanly with code 0, got err: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Revert cancelled. Pass --yes to skip confirmation.") {
		t.Errorf("expected cancellation notice in output, got: %s", string(out))
	}

	// Verify file is preserved
	if _, err := os.Stat(newFile); err != nil {
		t.Errorf("expected untracked file to still exist after cancellation, err: %v", err)
	}
}

func TestCLIRevertNonGitDirectory(t *testing.T) {
	nonGitDir := t.TempDir()
	out, err := runCLIInDir(nonGitDir, "revert", "--yes")
	if err == nil {
		t.Fatalf("expected revert to fail in non-git dir, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "not a git repository") {
		t.Errorf("expected 'not a git repository' in error output, got: %s", string(out))
	}
}

func TestCLILifecycleRunDiffRevert(t *testing.T) {
	dir := setupCLITestGitRepo(t)

	// Add another tracked file to test deletion and restoration
	toDeleteFile := filepath.Join(dir, "to_delete.txt")
	if err := os.WriteFile(toDeleteFile, []byte("tracked to delete\n"), 0600); err != nil {
		t.Fatalf("failed to write to_delete.txt: %v", err)
	}
	runGitInDir(t, dir, "add", "to_delete.txt")
	runGitInDir(t, dir, "commit", "-m", "add to_delete.txt")

	// Phase 1: Run sandboxed command that creates, modifies, and deletes files
	runCmd := "touch created_in_sandbox.txt && echo 'dirty mutation' >> initial.txt && rm to_delete.txt"
	runOut, runErr := runCLIInDir(dir, "run", "--", "sh", "-c", runCmd)
	if runErr != nil {
		t.Fatalf("safebox run failed: %v, output: %s", runErr, string(runOut))
	}

	// Phase 2: Verify change visibility via safebox diff
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

	// Phase 3: Execute one-command revert with --yes
	revertOut, revertErr := runCLIInDir(dir, "revert", "--yes")
	if revertErr != nil {
		t.Fatalf("safebox revert --yes failed: %v, output: %s", revertErr, string(revertOut))
	}
	if !strings.Contains(string(revertOut), "Working tree restored.") {
		t.Errorf("expected 'Working tree restored.' in revert output, got: %s", string(revertOut))
	}

	// Phase 3 verification: safebox diff reports clean working tree
	postDiffOut, postDiffErr := runCLIInDir(dir, "diff")
	if postDiffErr != nil {
		t.Fatalf("safebox diff after revert failed: %v, output: %s", postDiffErr, string(postDiffOut))
	}
	if !strings.Contains(string(postDiffOut), "Working tree is clean. No changes detected.") {
		t.Errorf("expected clean working tree message after revert, got: %s", string(postDiffOut))
	}

	// Direct filesystem verification
	createdFile := filepath.Join(dir, "created_in_sandbox.txt")
	if _, err := os.Stat(createdFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected created_in_sandbox.txt to be removed by revert, err: %v", err)
	}

	initialContent, err := os.ReadFile(filepath.Join(dir, "initial.txt"))
	if err != nil {
		t.Fatalf("failed to read initial.txt: %v", err)
	}
	if string(initialContent) != "initial content\n" {
		t.Errorf("expected initial.txt restored to 'initial content\\n', got: %s", string(initialContent))
	}

	deleteContent, err := os.ReadFile(toDeleteFile)
	if err != nil {
		t.Fatalf("failed to read to_delete.txt: %v", err)
	}
	if string(deleteContent) != "tracked to delete\n" {
		t.Errorf("expected to_delete.txt restored to 'tracked to delete\\n', got: %s", string(deleteContent))
	}
}

func TestCLIRevertYesEqualsTrue(t *testing.T) {
	dir := setupCLITestGitRepo(t)

	dirtyFile := filepath.Join(dir, "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("dirty\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	out, err := runCLIInDir(dir, "revert", "--yes=true")
	if err != nil {
		t.Fatalf("revert --yes=true failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Working tree restored.") {
		t.Errorf("expected 'Working tree restored.' in output, got: %s", string(out))
	}

	if _, err := os.Stat(dirtyFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected dirty.txt to be deleted, err: %v", err)
	}
}

func TestCLIShadowLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")
	workDir := filepath.Join(tmpDir, "work")
	mergedDir := filepath.Join(tmpDir, "merged")

	for _, d := range []string{lowerDir, upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// 1. Initial non-git workspace state
	if err := os.WriteFile(filepath.Join(lowerDir, "base.txt"), []byte("base v1\n"), 0600); err != nil {
		t.Fatalf("failed to write base.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lowerDir, "to_delete.txt"), []byte("to delete\n"), 0600); err != nil {
		t.Fatalf("failed to write to_delete.txt: %v", err)
	}

	// 2. Perform mutations inside an unprivileged OverlayFS subprocess
	helperCmd := exec.Command(os.Args[0], "-test.run=TestCLIShadowLifecycleHelper")
	helperCmd.Env = append(os.Environ(),
		"GO_WANT_CLI_SHADOW_HELPER=1",
		"SAFEBOX_TEST_LOWER="+lowerDir,
		"SAFEBOX_TEST_UPPER="+upperDir,
		"SAFEBOX_TEST_WORK="+workDir,
		"SAFEBOX_TEST_MERGED="+mergedDir,
	)
	helperCmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
	}
	if out, err := helperCmd.CombinedOutput(); err != nil {
		t.Fatalf("shadow helper failed: %v, output: %s", err, string(out))
	}

	// 3. Run safebox diff --shadow=<upperDir> from lowerDir (non-git dir)
	diffOut, diffErr := runCLIInDir(lowerDir, "diff", "--shadow="+upperDir)
	if diffErr != nil {
		t.Fatalf("safebox diff --shadow failed: %v, output: %s", diffErr, string(diffOut))
	}
	diffStr := string(diffOut)
	if !strings.Contains(diffStr, "+ [ADDED]") || !strings.Contains(diffStr, "created.txt") {
		t.Errorf("expected added created.txt in diff output, got: %s", diffStr)
	}
	if !strings.Contains(diffStr, "~ [MODIFIED]") || !strings.Contains(diffStr, "base.txt") {
		t.Errorf("expected modified base.txt in diff output, got: %s", diffStr)
	}
	if !strings.Contains(diffStr, "- [DELETED]") || !strings.Contains(diffStr, "to_delete.txt") {
		t.Errorf("expected deleted to_delete.txt in diff output, got: %s", diffStr)
	}

	// 4. Run safebox apply --shadow=<upperDir> --yes
	applyOut, applyErr := runCLIInDir(lowerDir, "apply", "--shadow="+upperDir, "--yes")
	if applyErr != nil {
		t.Fatalf("safebox apply --shadow failed: %v, output: %s", applyErr, string(applyOut))
	}
	if !strings.Contains(string(applyOut), "Shadow changes applied to working directory.") {
		t.Errorf("expected confirmation in apply output, got: %s", string(applyOut))
	}

	// 5. Direct filesystem verification of lowerDir
	baseContent, err := os.ReadFile(filepath.Join(lowerDir, "base.txt"))
	if err != nil {
		t.Fatalf("failed to read base.txt: %v", err)
	}
	if string(baseContent) != "base v2 modified\n" {
		t.Errorf("expected base.txt updated, got: %s", string(baseContent))
	}

	createdContent, err := os.ReadFile(filepath.Join(lowerDir, "created.txt"))
	if err != nil {
		t.Fatalf("failed to read created.txt: %v", err)
	}
	if string(createdContent) != "newly created\n" {
		t.Errorf("expected created.txt created, got: %s", string(createdContent))
	}

	if _, err := os.Stat(filepath.Join(lowerDir, "to_delete.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected to_delete.txt to be removed from lowerDir, err: %v", err)
	}
}

func TestCLIApplyRequiresShadow(t *testing.T) {
	out, err := runCLI("apply")
	if err == nil {
		t.Fatalf("expected error when --shadow is missing, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "--shadow=<dir> argument is required") {
		t.Errorf("expected '--shadow=<dir> argument is required' in output, got: %s", string(out))
	}
}

func TestCLIApplyNonExistentDir(t *testing.T) {
	out, err := runCLI("apply", "--shadow=/non/existent/shadow/dir", "--yes")
	if err == nil {
		t.Fatalf("expected error for non-existent shadow dir, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "does not exist") {
		t.Errorf("expected 'does not exist' in output, got: %s", string(out))
	}
}

func TestCLIDiffNonExistentShadowDir(t *testing.T) {
	out, err := runCLI("diff", "--shadow=/non/existent/shadow/dir")
	if err == nil {
		t.Fatalf("expected error for non-existent shadow dir in diff, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "does not exist") {
		t.Errorf("expected 'does not exist' in output, got: %s", string(out))
	}
}

func TestCLIApplyConfirmationYes(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")

	if err := os.MkdirAll(lowerDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(upperDir, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("data\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, err := runCLIInDirWithStdin(lowerDir, strings.NewReader("y\n"), "apply", "--shadow="+upperDir)
	if err != nil {
		t.Fatalf("expected interactive apply to succeed, got: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Shadow changes applied to working directory.") {
		t.Errorf("expected apply success message, got: %s", string(out))
	}
}

func TestCLIApplyConfirmationNo(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	upperDir := filepath.Join(tmpDir, "upper")

	if err := os.MkdirAll(lowerDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(upperDir, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("data\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, err := runCLIInDirWithStdin(lowerDir, strings.NewReader("n\n"), "apply", "--shadow="+upperDir)
	if err != nil {
		t.Fatalf("expected interactive apply cancel to exit 0, got: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Apply cancelled.") {
		t.Errorf("expected 'Apply cancelled.' in output, got: %s", string(out))
	}
	if _, err := os.Stat(filepath.Join(lowerDir, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("expected new.txt NOT to be applied to lowerDir")
	}
}

func TestCLIShadowLifecycleHelper(t *testing.T) {
	if os.Getenv("GO_WANT_CLI_SHADOW_HELPER") != "1" {
		return
	}

	lowerDir := os.Getenv("SAFEBOX_TEST_LOWER")
	upperDir := os.Getenv("SAFEBOX_TEST_UPPER")
	workDir := os.Getenv("SAFEBOX_TEST_WORK")
	mergedDir := os.Getenv("SAFEBOX_TEST_MERGED")

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	if err := syscall.Mount("overlay", mergedDir, "overlay", 0, opts); err != nil {
		os.Stderr.WriteString("mount error: " + err.Error() + "\n")
		os.Exit(1)
	}

	// 1. Modify
	if err := os.WriteFile(filepath.Join(mergedDir, "base.txt"), []byte("base v2 modified\n"), 0600); err != nil {
		os.Stderr.WriteString("modify error: " + err.Error() + "\n")
		os.Exit(2)
	}

	// 2. Add
	if err := os.WriteFile(filepath.Join(mergedDir, "created.txt"), []byte("newly created\n"), 0600); err != nil {
		os.Stderr.WriteString("create error: " + err.Error() + "\n")
		os.Exit(3)
	}

	// 3. Delete
	if err := os.Remove(filepath.Join(mergedDir, "to_delete.txt")); err != nil {
		os.Stderr.WriteString("delete error: " + err.Error() + "\n")
		os.Exit(4)
	}

	if err := syscall.Unmount(mergedDir, 0); err != nil {
		os.Stderr.WriteString("unmount error: " + err.Error() + "\n")
		os.Exit(5)
	}

	os.Exit(0)
}
