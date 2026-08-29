package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func TestCLIRunRequiresDoubleDash(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "run", "python3", "hello.py")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for run without '--', got success with output: %s", string(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2 for missing '--', got err: %v", err)
	}
	expectedMsg := "safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)"
	if !strings.Contains(string(out), expectedMsg) {
		t.Fatalf("expected output to contain %q, got: %s", expectedMsg, string(out))
	}
}

func TestCLIRunDoubleDashAccepted(t *testing.T) {
	out, err := runCLI("run", "--", "echo", "hello")
	if err != nil {
		t.Fatalf("safebox run -- echo hello failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("expected output to contain 'hello', got: %s", string(out))
	}
}

func TestCLIRunAllowPathBeforeDoubleDash(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok\n"), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	out, err := runCLI("run", "--allow-path="+tmpDir, "--", scriptPath)
	if err != nil {
		t.Fatalf("safebox run with --allow-path before -- failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("expected output to contain 'ok', got: %s", string(out))
	}
}

func TestCLIRunAllowPathMissingDoubleDash(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok\n"), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	cmd := exec.Command(testBinaryPath, "run", "--allow-path="+tmpDir, scriptPath)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for run with --allow-path but missing '--', got success with output: %s", string(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2 for missing '--', got err: %v", err)
	}
	expectedMsg := "safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)"
	if !strings.Contains(string(out), expectedMsg) {
		t.Fatalf("expected output to contain %q, got: %s", expectedMsg, string(out))
	}
}

func TestCLIRunUIDMapping(t *testing.T) {
	stdout, stderr, err := runCLISplit("", "run", "--", "id", "-u")
	if err != nil {
		t.Fatalf("run id -u failed: %v, stderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != "0" {
		t.Errorf("expected container UID 0, got %s", strings.TrimSpace(stdout))
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
	if !strings.Contains(string(out), "Permission denied") && !strings.Contains(string(out), "permission denied") {
		t.Errorf("expected 'Permission denied' in output, got: %s", string(out))
	}
}

func TestCLIRunLandlockEtcShadowDenial(t *testing.T) {
	out, err := runCLI("run", "--", "cat", "/etc/shadow")
	if err == nil {
		t.Fatalf("expected cat /etc/shadow to fail, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "Permission denied") && !strings.Contains(string(out), "permission denied") {
		t.Errorf("expected 'Permission denied' in output, got: %s", string(out))
	}
}

func TestCLIRunLandlockEtcPasswdAllowed(t *testing.T) {
	out, err := runCLI("run", "--", "cat", "/etc/passwd")
	if err != nil {
		t.Fatalf("expected cat /etc/passwd to succeed, got: %v (output: %s)", err, string(out))
	}
	if !strings.Contains(string(out), "root:") {
		t.Errorf("expected /etc/passwd content to contain 'root:', got: %s", string(out))
	}
}

func TestCLIRunAllowPathFlag(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "custom.sh")
	scriptContent := "#!/bin/sh\necho \"CUSTOM_ALLOW_PATH_SUCCESS\"\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create custom test script: %v", err)
	}

	// Execution without --allow-path should fail
	out, err := runCLI("run", "--", scriptPath)
	if err == nil {
		t.Fatalf("expected execution without --allow-path to fail, got success: %s", string(out))
	}

	// Execution with --allow-path should succeed
	out, err = runCLI("run", "--allow-path="+tmpDir, "--", scriptPath)
	if err != nil {
		t.Fatalf("expected execution with --allow-path to succeed, got: %v (output: %s)", err, string(out))
	}
	if !strings.Contains(string(out), "CUSTOM_ALLOW_PATH_SUCCESS") {
		t.Errorf("expected output to contain 'CUSTOM_ALLOW_PATH_SUCCESS', got: %s", string(out))
	}
}

func TestCLIRunAllowPathRWEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "rw_created.txt")
	out, err := runCLI("run", "--allow-path-rw="+tmpDir, "--", "sh", "-c", fmt.Sprintf("echo 'written' > %s", targetFile))
	if err != nil {
		t.Fatalf("expected run with --allow-path-rw to succeed, got: %v (output: %s)", err, string(out))
	}
	content, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("expected target file to exist on host after --allow-path-rw write: %v", err)
	}
	if !strings.Contains(string(content), "written") {
		t.Errorf("expected target file to contain 'written', got: %s", string(content))
	}
}

func TestCLIRunAllowPathRWDoesNotGrantRWToOtherPaths(t *testing.T) {
	tmpDirRW := t.TempDir()
	tmpDirForbidden := t.TempDir()
	targetFile := filepath.Join(tmpDirForbidden, "forbidden.txt")
	out, err := runCLI("run", "--allow-path-rw="+tmpDirRW, "--", "sh", "-c", fmt.Sprintf("echo 'fail' > %s", targetFile))
	if err == nil {
		t.Fatalf("expected write to non-allowed path to fail, got success: %s", string(out))
	}
	if _, err := os.Stat(targetFile); err == nil {
		t.Fatalf("forbidden file was created despite not being in --allow-path-rw")
	}
}

func TestCLIRunAllowPathRWMissingDoubleDash(t *testing.T) {
	tmpDir := t.TempDir()
	cmd := exec.Command(testBinaryPath, "run", "--allow-path-rw="+tmpDir, "echo", "hi")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for run with --allow-path-rw but missing '--', got success with output: %s", string(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2 for missing '--', got err: %v", err)
	}
	expectedMsg := "safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)"
	if !strings.Contains(string(out), expectedMsg) {
		t.Fatalf("expected output to contain %q, got: %s", expectedMsg, string(out))
	}
}

func TestCLIActionableHintOnDenial(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "denied.sh")
	scriptContent := "#!/bin/sh\necho \"denied\"\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	out, err := runCLI("run", "--", scriptPath)
	if err == nil {
		t.Fatalf("expected denial error, got success: %s", string(out))
	}
	expectedHint := "hint: rerun with --allow-path=" + tmpDir
	if !strings.Contains(string(out), expectedHint) {
		t.Errorf("expected output to contain hint %q, got: %s", expectedHint, string(out))
	}
}

func TestCLIHintForBinaryInUnallowedDir(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	fakeAgy := filepath.Join(binDir, "agy")
	if err := os.WriteFile(fakeAgy, []byte("#!/bin/sh\necho agy_ok\n"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	out, err := runCLI("run", "--", fakeAgy)
	if err == nil {
		t.Fatalf("expected failure when running binary outside allow-list, got success: %s", string(out))
	}
	expectedHint := "hint: rerun with --allow-path=" + binDir
	if !strings.Contains(string(out), expectedHint) {
		t.Errorf("expected output to contain %q, got: %s", expectedHint, string(out))
	}
}

func TestCLIHintForStateDirWriteDenied(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	stateDir := filepath.Join(tmpDir, "state")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}
	agentScript := filepath.Join(binDir, "agent.sh")
	scriptBody := fmt.Sprintf("#!/bin/sh\necho data > %s/log.txt\n", stateDir)
	if err := os.WriteFile(agentScript, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("failed to write agent script: %v", err)
	}

	out, err := runCLI("run", "--allow-path="+binDir, "--", agentScript)
	if err == nil {
		t.Fatalf("expected write to state dir outside allow-path-rw to fail, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "Permission denied") && !strings.Contains(string(out), "permission denied") {
		t.Errorf("expected permission denied in output, got: %s", string(out))
	}
}

func TestCLIHintForMissingExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	missingPath := filepath.Join(tmpDir, "tools", "missing_cmd")
	out, err := runCLI("run", "--", missingPath)
	if err == nil {
		t.Fatalf("expected failure for missing executable, got success: %s", string(out))
	}
	expectedHint := "hint: rerun with --allow-path=" + filepath.Dir(missingPath)
	if !strings.Contains(string(out), expectedHint) {
		t.Errorf("expected output to contain %q, got: %s", expectedHint, string(out))
	}
}

func TestCLIRunAllowPathAfterDoubleDashFails(t *testing.T) {
	out, err := runCLI("run", "--", "--allow-path=/tmp", "--", "/bin/true")
	if err == nil {
		t.Fatalf("expected error when --allow-path placed after --, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "--allow-path must precede") {
		t.Errorf("expected explicit error about --allow-path placement, got: %s", string(out))
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

func TestCLIRunPIDIsolation(t *testing.T) {
	stdout, stderr, err := runCLISplit("", "run", "--", "sh", "-c", "echo $PPID")
	if err != nil {
		t.Fatalf("run failed: %v, stderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != "1" {
		t.Errorf("expected wrapped process PPID == 1 in stdout, got %s", strings.TrimSpace(stdout))
	}
}

func TestCLIRunSignalForwarding(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "run", "--", "sleep", "10")
	cmd.Env = append(os.Environ(), "LANG=C")

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start safebox run: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM to safebox process: %v", err)
	}

	err := cmd.Wait()
	if err == nil {
		t.Fatal("expected non-zero exit on signal termination, got nil error")
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		ws, ok := exitErr.Sys().(syscall.WaitStatus)
		if ok {
			if !ws.Signaled() && ws.ExitStatus() != 143 {
				t.Fatalf("expected signaled termination or exit code 143, got status %v", ws)
			}
		}
	}
}

func TestCLIRunZombieReaping(t *testing.T) {
	out, err := runCLI("run", "--", "sh", "-c", "(sleep 0.05 &); sleep 0.2")
	if err != nil {
		t.Fatalf("run failed: %v, output: %s", err, string(out))
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
	for _, expected := range []string{
		"safebox <command> [arguments]",
		"Commands:",
		"--allow-path-rw",
		"Where safebox stores state:",
		"Running a coding agent:",
		"On permission denial:",
	} {
		if !strings.Contains(string(out), expected) {
			t.Errorf("expected %q in help output, got: %s", expected, string(out))
		}
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
	if !strings.Contains(string(revertOut), "Working tree restored.") && !strings.Contains(string(revertOut), "Overlay session discarded") {
		t.Errorf("expected 'Working tree restored.' or 'Overlay session discarded' in revert output, got: %s", string(revertOut))
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

func TestCLIDiffRejectsShadowFlag(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "diff", "--shadow=/tmp/shadow")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error when passing --shadow to diff, got success: %s", string(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2 for --shadow in diff, got: %v", err)
	}
	if !strings.Contains(string(out), "unknown flag '--shadow'") {
		t.Errorf("expected 'unknown flag \\'--shadow\\'' in output, got: %s", string(out))
	}
}

func TestCLIApplyRejectsShadowFlag(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "apply", "--shadow=/tmp/shadow")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error when passing --shadow to apply, got success: %s", string(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2 for --shadow in apply, got: %v", err)
	}
	if !strings.Contains(string(out), "unknown flag '--shadow'") {
		t.Errorf("expected 'unknown flag \\'--shadow\\'' in output, got: %s", string(out))
	}
}

func TestCLIApplyRequiresSession(t *testing.T) {
	freshDir := t.TempDir()
	out, err := runCLIInDir(freshDir, "apply")
	if err == nil {
		t.Fatalf("expected error when no session exists, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "no active session") {
		t.Errorf("expected error about missing session, got: %s", string(out))
	}
}

func TestCLIApplyInteractiveYes(t *testing.T) {
	dir := t.TempDir()
	_, err := runCLIInDir(dir, "run", "--", "touch", "interactive_yes.txt")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	out, err := runCLIInDirWithStdin(dir, strings.NewReader("y\n"), "apply")
	if err != nil {
		t.Fatalf("expected interactive apply to succeed, got: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Shadow changes applied to working directory.") {
		t.Errorf("expected apply success message, got: %s", string(out))
	}
	if _, err := os.Stat(filepath.Join(dir, "interactive_yes.txt")); err != nil {
		t.Errorf("expected interactive_yes.txt to exist in working dir after apply: %v", err)
	}
}

func TestCLIApplyInteractiveNo(t *testing.T) {
	dir := t.TempDir()
	_, err := runCLIInDir(dir, "run", "--", "touch", "interactive_no.txt")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	out, err := runCLIInDirWithStdin(dir, strings.NewReader("n\n"), "apply")
	if err != nil {
		t.Fatalf("expected interactive apply cancel to exit 0, got: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Apply cancelled.") {
		t.Errorf("expected 'Apply cancelled.' in output, got: %s", string(out))
	}
	if _, err := os.Stat(filepath.Join(dir, "interactive_no.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected interactive_no.txt NOT to be applied to working dir, err: %v", err)
	}
}

func TestCLISandboxOverlayWriteIsolation(t *testing.T) {
	testDir := t.TempDir()
	out, err := runCLIInDir(testDir, "run", "--", "touch", "isolated.txt")
	if err != nil {
		t.Fatalf("safebox run failed: %v, output: %s", err, string(out))
	}

	// Immediately after run, isolated.txt must NOT exist on host
	if _, err := os.Stat(filepath.Join(testDir, "isolated.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected isolated.txt NOT to exist on host immediately after run")
	}
}

func TestCLISandboxAutoLifecycle(t *testing.T) {
	testDir := t.TempDir()

	// Initial file
	initFile := filepath.Join(testDir, "init.txt")
	if err := os.WriteFile(initFile, []byte("initial\n"), 0600); err != nil {
		t.Fatalf("failed to create init file: %v", err)
	}

	// 1. Run commands in sandbox: add file, modify file
	out, err := runCLIInDir(testDir, "run", "--", "sh", "-c", "touch added.txt && echo 'mutated' >> init.txt")
	if err != nil {
		t.Fatalf("safebox run failed: %v, output: %s", err, string(out))
	}

	// Host remains untouched
	if _, err := os.Stat(filepath.Join(testDir, "added.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected added.txt NOT to exist on host before apply")
	}

	// 2. Auto-diff without --shadow
	diffOut, diffErr := runCLIInDir(testDir, "diff")
	if diffErr != nil {
		t.Fatalf("safebox diff failed: %v, output: %s", diffErr, string(diffOut))
	}
	diffStr := string(diffOut)
	if !strings.Contains(diffStr, "+ [ADDED]") || !strings.Contains(diffStr, "added.txt") {
		t.Errorf("expected added.txt in diff, got: %s", diffStr)
	}
	if !strings.Contains(diffStr, "~ [MODIFIED]") || !strings.Contains(diffStr, "init.txt") {
		t.Errorf("expected init.txt in diff, got: %s", diffStr)
	}

	// 3. Auto-apply without --shadow
	applyOut, applyErr := runCLIInDir(testDir, "apply", "--yes")
	if applyErr != nil {
		t.Fatalf("safebox apply --yes failed: %v, output: %s", applyErr, string(applyOut))
	}

	// Now changes are on host
	if _, err := os.Stat(filepath.Join(testDir, "added.txt")); err != nil {
		t.Fatalf("expected added.txt to exist on host after apply: %v", err)
	}
	content, err := os.ReadFile(initFile)
	if err != nil || !strings.Contains(string(content), "mutated") {
		t.Fatalf("expected init.txt to contain 'mutated' on host, got: %s", string(content))
	}

	// 4. Test revert lifecycle in fresh dir
	revertDir := t.TempDir()
	out, err = runCLIInDir(revertDir, "run", "--", "touch", "to_discard.txt")
	if err != nil {
		t.Fatalf("safebox run failed: %v, output: %s", err, string(out))
	}

	revertOut, revertErr := runCLIInDir(revertDir, "revert", "--yes")
	if revertErr != nil {
		t.Fatalf("safebox revert --yes failed: %v, output: %s", revertErr, string(revertOut))
	}
	if !strings.Contains(string(revertOut), "Overlay session discarded") && !strings.Contains(string(revertOut), "Working tree restored") {
		t.Errorf("expected session discarded message, got: %s", string(revertOut))
	}

	if _, err := os.Stat(filepath.Join(revertDir, "to_discard.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected to_discard.txt NOT to exist after revert")
	}
}

func TestCLISandboxSubdirRevertDoesNotDiscardParentSession(t *testing.T) {
	parent := t.TempDir()
	sub := filepath.Join(parent, "sub")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}

	// Run from parent
	out, err := runCLIInDir(parent, "run", "--", "sh", "-c", "echo P > new_file.txt")
	if err != nil {
		t.Fatalf("parent run: %v\n%s", err, out)
	}

	// Revert from sub - must NOT discard parent's session
	_, _ = runCLIInDir(sub, "revert", "--yes")

	// Verify parent session still discoverable (diff shows ADDED)
	out, err = runCLIInDir(parent, "diff")
	if err != nil {
		t.Fatalf("parent diff after subdir revert: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "[ADDED]") {
		t.Errorf("expected parent's session to be intact after subdir revert; got: %s", string(out))
	}

	// Clean up
	_, _ = runCLIInDir(parent, "revert", "--yes")
}

func TestCLISandboxSubdirApplyDoesNotApplyParentSession(t *testing.T) {
	parent := t.TempDir()
	sub := filepath.Join(parent, "sub")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}

	out, err := runCLIInDir(parent, "run", "--", "sh", "-c", "echo P > new_file.txt")
	if err != nil {
		t.Fatalf("parent run: %v\n%s", err, out)
	}

	// Apply from sub - must NOT apply parent's session
	_, _ = runCLIInDir(sub, "apply", "--yes")

	// Parent's new_file.txt must NOT exist on host
	if _, err := os.Stat(filepath.Join(parent, "new_file.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("parent's session leaked to host via subdir apply")
	}

	// Clean up
	_, _ = runCLIInDir(parent, "revert", "--yes")
}

func runCLISplit(dir string, args ...string) (string, string, error) {
	cmd := exec.Command(testBinaryPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LANG=C")
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}

func TestCLIRunDefaultTraceOutput(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, err := runCLISplit(dir, "run", "--", "echo", "trace_payload")
	if err != nil {
		t.Fatalf("run failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "trace_payload") {
		t.Errorf("expected trace_payload in stdout, got: %q", stdout)
	}
	if !strings.Contains(stderr, "[safebox]") {
		t.Fatalf("expected [safebox] prefix in stderr, got: %q", stderr)
	}
	if !strings.Contains(stderr, "[safebox:child]") {
		t.Fatalf("expected [safebox:child] prefix in stderr, got: %q", stderr)
	}
	for _, step := range []string{"session initialize", "wrapped command execution", "overlayfs mount", "landlock restrict", "exec handoff"} {
		if !strings.Contains(stderr, step) {
			t.Errorf("expected step %q in stderr trace, got:\n%s", step, stderr)
		}
	}
}

func TestCLIRunTraceOrder(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, err := runCLISplit(dir, "run", "--", "echo", "order_test")
	if err != nil {
		t.Fatalf("run failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "order_test") {
		t.Errorf("expected order_test in stdout, got: %q", stdout)
	}

	idxInit := strings.Index(stderr, "session initialize")
	idxMount := strings.Index(stderr, "overlayfs mount")
	idxLandlock := strings.Index(stderr, "landlock restrict")
	idxHandoff := strings.Index(stderr, "exec handoff")
	idxWrapped := strings.Index(stderr, "wrapped command execution")

	if idxInit == -1 || idxMount == -1 || idxLandlock == -1 || idxHandoff == -1 || idxWrapped == -1 {
		t.Fatalf("missing one or more trace steps in stderr:\n%s", stderr)
	}

	if !(idxInit < idxMount && idxMount < idxLandlock && idxLandlock < idxHandoff && idxHandoff < idxWrapped) {
		t.Errorf("expected trace order init < mount < landlock < handoff < wrapped, got indices: init=%d mount=%d landlock=%d handoff=%d wrapped=%d\nFull stderr:\n%s",
			idxInit, idxMount, idxLandlock, idxHandoff, idxWrapped, stderr)
	}
}

func TestCLIRunExecHandoffTiming(t *testing.T) {
	dir := t.TempDir()
	_, stderr, err := runCLISplit(dir, "run", "--", "echo", "timing_test")
	if err != nil {
		t.Fatalf("run failed: %v, stderr: %s", err, stderr)
	}
	lines := strings.Split(stderr, "\n")
	foundHandoff := false
	for _, line := range lines {
		if strings.Contains(line, "exec handoff") {
			foundHandoff = true
			if !strings.Contains(line, "[safebox:child]") {
				t.Errorf("expected [safebox:child] prefix on exec handoff line, got: %s", line)
			}
			if strings.HasSuffix(strings.TrimSpace(line), " 0s") {
				t.Errorf("expected non-zero duration for exec handoff, got: %s", line)
			}
		}
	}
	if !foundHandoff {
		t.Fatalf("did not find 'exec handoff' line in stderr:\n%s", stderr)
	}
}

func TestCLIRunQuietFlagSuppressesTrace(t *testing.T) {
	dir := t.TempDir()
	for _, flag := range []string{"--quiet", "-q"} {
		stdout, stderr, err := runCLISplit(dir, "run", flag, "--", "echo", "quiet_payload")
		if err != nil {
			t.Fatalf("run with %s failed: %v, stderr: %s", flag, err, stderr)
		}
		if !strings.Contains(stdout, "quiet_payload") {
			t.Errorf("expected quiet_payload in stdout with %s, got: %q", flag, stdout)
		}
		if strings.Contains(stderr, "[safebox]") {
			t.Errorf("expected no [safebox] trace lines in stderr with %s, got: %q", flag, stderr)
		}
	}
}

func TestCLISubcommandsTraceOutput(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runCLISplit(dir, "run", "--", "touch", "item.txt")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	// diff trace
	_, diffErr, err := runCLISplit(dir, "diff")
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if !strings.Contains(diffErr, "[safebox]") || !strings.Contains(diffErr, "diff computation") {
		t.Errorf("expected diff trace in stderr, got: %q", diffErr)
	}

	// diff quiet
	_, diffQuietErr, err := runCLISplit(dir, "diff", "--quiet")
	if err != nil {
		t.Fatalf("diff --quiet failed: %v", err)
	}
	if strings.Contains(diffQuietErr, "[safebox]") {
		t.Errorf("expected quiet diff without trace, got: %q", diffQuietErr)
	}

	// apply trace
	_, applyErr, err := runCLISplit(dir, "apply", "--yes")
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !strings.Contains(applyErr, "[safebox]") || !strings.Contains(applyErr, "apply changes") {
		t.Errorf("expected apply trace in stderr, got: %q", applyErr)
	}

	// revert trace
	_, _, _ = runCLISplit(dir, "run", "--", "touch", "revert_item.txt")
	_, revertErr, err := runCLISplit(dir, "revert", "--yes")
	if err != nil {
		t.Fatalf("revert failed: %v", err)
	}
	if !strings.Contains(revertErr, "[safebox]") || !strings.Contains(revertErr, "discard session") {
		t.Errorf("expected revert trace in stderr, got: %q", revertErr)
	}
}

func TestCLIToolPathDenialAndHint(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "custom_tool.sh")
	scriptContent := "#!/bin/sh\necho 'custom tool running'\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write custom tool script: %v", err)
	}

	// 1. Running without --allow-path must fail with permission denied and hint
	_, stderr, err := runCLISplit("", "run", "--", scriptPath)
	if err == nil {
		t.Fatalf("expected custom tool to fail without allow-path, got success")
	}
	if !strings.Contains(stderr, "permission denied") && !strings.Contains(stderr, "DENIED") {
		t.Errorf("expected permission denied or DENIED in stderr, got: %s", stderr)
	}
	expectedHint := fmt.Sprintf("--allow-path=%s", tmpDir)
	if !strings.Contains(stderr, expectedHint) {
		t.Errorf("expected hint with %q, got: %s", expectedHint, stderr)
	}

	// 2. Running with --allow-path must succeed
	stdout, stderr, err := runCLISplit("", "run", "--allow-path="+tmpDir, "--", scriptPath)
	if err != nil {
		t.Fatalf("expected custom tool to succeed with --allow-path, got err: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "custom tool running") {
		t.Errorf("expected 'custom tool running' in stdout, got: %s", stdout)
	}
}

func TestCLIRealAgyAgentIntegration(t *testing.T) {
	agyPath := "/root/.local/bin/agy"
	if _, err := os.Stat(agyPath); err != nil {
		t.Skipf("real agent binary %s not available on this host: %v", agyPath, err)
	}

	// 1. Without allow-path: must fail and suggest allow-path
	_, stderr, err := runCLISplit("", "run", "--", agyPath, "--version")
	if err == nil {
		t.Fatalf("expected real agy binary to fail without allow-path, got success")
	}
	if !strings.Contains(stderr, "--allow-path=/root/.local/bin") {
		t.Errorf("expected hint to contain '--allow-path=/root/.local/bin', got: %s", stderr)
	}

	// 2. With allow-path: must succeed and report version
	stdout, stderr, err := runCLISplit("", "run", "--allow-path=/root/.local/bin", "--", agyPath, "--version")
	if err != nil {
		t.Fatalf("safebox run with --allow-path failed for real agy: %v, stderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("expected non-empty version output from agy, got: %q", stdout)
	}
}

func TestCLISessionAutomaticPrune(t *testing.T) {
	sessionRoot := t.TempDir()
	t.Setenv("SAFEBOX_SESSION_ROOT", sessionRoot)

	// Create an artificially stale session (>24h ago)
	oldSessDir := filepath.Join(sessionRoot, "sess-old-probe")
	if err := os.MkdirAll(filepath.Join(oldSessDir, "upper"), 0700); err != nil {
		t.Fatal(err)
	}
	oldMeta := fmt.Sprintf(`{"id":"sess-old-probe","base_dir":%q,"lower_dir":%q,"created_at":"2020-01-01T00:00:00Z"}`, oldSessDir, t.TempDir())
	if err := os.WriteFile(filepath.Join(oldSessDir, "session.json"), []byte(oldMeta), 0600); err != nil {
		t.Fatal(err)
	}

	// Run safebox run, which triggers automatic session pruning
	cmd := exec.Command(testBinaryPath, "run", "--quiet", "--", "true")
	cmd.Env = append(os.Environ(), "SAFEBOX_SESSION_ROOT="+sessionRoot, "LANG=C")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("safebox run failed: %v, output: %s", err, string(out))
	}

	// Assert stale session was purged
	if _, err := os.Stat(oldSessDir); !os.IsNotExist(err) {
		t.Errorf("expected stale session directory %s to be pruned by safebox run", oldSessDir)
	}
}

func TestCLIBenchmarkStartupLatency(t *testing.T) {
	if os.Getenv("SKIP_LATENCY_TEST") != "" {
		t.Skip("latency test skipped via env")
	}

	const iterations = 20
	var totalDuration time.Duration
	samples := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		cmd := exec.Command(testBinaryPath, "run", "--quiet", "--", "true")
		cmd.Env = append(os.Environ(), "LANG=C")
		if err := cmd.Run(); err != nil {
			t.Fatalf("iteration %d failed: %v", i, err)
		}
		elapsed := time.Since(start)
		samples = append(samples, elapsed)
		totalDuration += elapsed
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p50 := samples[len(samples)*50/100]
	p95 := samples[len(samples)*95/100]
	avgLatency := totalDuration / iterations

	t.Logf("Startup latency over %d runs: avg=%v p50=%v p95=%v", iterations, avgLatency, p50, p95)

	// NFR3 sets 50ms startup overhead target (relaxed to 200ms/300ms coarse threshold to absorb VM/container scheduling jitter)
	budget := 200 * time.Millisecond
	if isRaceEnabled {
		budget = 300 * time.Millisecond
	}
	if avgLatency > budget {
		t.Errorf("average startup latency %v exceeds %v budget (NFR3)", avgLatency, budget)
	}
}
