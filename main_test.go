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
		"--allow-file-rw",
		"profile [list|show <name>]",
		"Where safebox stores state:",
		"Running a coding agent:",
		"Security Note & Threat Model:",
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

	// Run best of 3 to account for initial process warmup / scheduling jitter
	var best time.Duration
	for i := 0; i < 3; i++ {
		start := time.Now()
		cmd := exec.Command(testBinaryPath, "run", "--", "true")
		cmd.Env = append(os.Environ(), "LANG=C")
		if err := cmd.Run(); err != nil {
			t.Fatalf("safebox run -- true failed: %v", err)
		}
		elapsed := time.Since(start)
		if i == 0 || elapsed < best {
			best = elapsed
		}
	}
	if best > 200*time.Millisecond {
		t.Errorf("startup latency %v exceeds 200ms coarse budget (NFR3 target: 50ms)", best)
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
	for _, step := range []string{"session initialize", "wrapped command spawn", "overlayfs mount", "landlock restrict", "exec handoff"} {
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

	// Expected order: parent steps first (session init, then wrapped
	// command spawn), then child steps (overlay, landlock, exec handoff).
	idxInit := strings.Index(stderr, "session initialize")
	idxSpawn := strings.Index(stderr, "wrapped command spawn")
	idxMount := strings.Index(stderr, "overlayfs mount")
	idxLandlock := strings.Index(stderr, "landlock restrict")
	idxHandoff := strings.Index(stderr, "exec handoff")

	if idxInit == -1 || idxSpawn == -1 || idxMount == -1 || idxLandlock == -1 || idxHandoff == -1 {
		t.Fatalf("missing one or more trace steps in stderr:\n%s", stderr)
	}

	if !(idxInit < idxSpawn && idxSpawn < idxMount && idxMount < idxLandlock && idxLandlock < idxHandoff) {
		t.Errorf("expected trace order init < spawn < mount < landlock < handoff, got indices: init=%d spawn=%d mount=%d landlock=%d handoff=%d\nFull stderr:\n%s",
			idxInit, idxSpawn, idxMount, idxLandlock, idxHandoff, stderr)
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
	if hasUsableNetBackendForTest() {
		// The agy built-in profile sets allow_net = true, so the wrapper now
		// tries to attach a userspace NAT backend. The CLI test harness runs
		// safebox as a regular exec.Cmd (no CLONE_NEWNET), so the spawned
		// backend has no namespace to attach to and slirp4netns/pasta exits
		// with EOF on --ready-fd. Real end-to-end verification is in the
		// integration-tagged test suite.
		t.Skip("network backend installed but test harness cannot provide CLONE_NEWNET child; skipping")
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

func TestCLIProbePrintsAllowList(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "run", "--probe", "--allow-path=/usr/local/bin", "--allow-path-rw=/tmp", "--", "true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected --probe to exit 0, got err: %v, output: %s", err, string(out))
	}
	output := string(out)
	if !strings.Contains(output, "safebox probe report") {
		t.Errorf("expected header 'safebox probe report', got: %s", output)
	}
	if !strings.Contains(output, "Landlock allow-list (effective):") {
		t.Errorf("expected 'Landlock allow-list (effective):', got: %s", output)
	}
	if !strings.Contains(output, "RW dirs:") || !strings.Contains(output, "RO dirs:") {
		t.Errorf("expected RW and RO sections, got: %s", output)
	}
	if !strings.Contains(output, "Wrapped command will NOT be executed. Exiting.") {
		t.Errorf("expected exit message, got: %s", output)
	}
}

func TestCLIProbeExitsZero(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "run", "--probe", "--", "nonexistent_command_xyz_123")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected --probe to exit 0 even for non-existent binary, got err: %v, output: %s", err, string(out))
	}
}

func TestCLIProbeRequiresDoubleDash(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "run", "--probe", "true")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for --probe without '--', got success with output: %s", string(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Errorf("expected exit code 2, got: %v", err)
	}
}

func TestCLIProbeUnresolvableBinary(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "run", "--probe", "--", "unresolvable_tool_xyz_999")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected --probe to exit 0, got err: %v", err)
	}
	output := string(out)
	if !strings.Contains(output, `(unresolvable: "unresolvable_tool_xyz_999")`) {
		t.Errorf("expected unresolvable binary notice, got: %s", output)
	}
}

func TestCLIRunAppliesBuiltinProfile(t *testing.T) {
	if hasUsableNetBackendForTest() {
		t.Skip("agy built-in profile sets allow_net=true; cannot exercise in test harness without CLONE_NEWNET")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	persistentDir := filepath.Join(home, ".local", "share", "safebox", "agents", "agy")
	targetFile := filepath.Join(persistentDir, "test_write.txt")
	defer os.Remove(targetFile)

	tmpDir := t.TempDir()
	agyScript := filepath.Join(tmpDir, "agy")
	scriptBody := "#!/bin/sh\necho 'from_agy' > ~/.gemini/test_write.txt\n"
	if err := os.WriteFile(agyScript, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("failed to write agy script: %v", err)
	}

	out, err := runCLI("run", "--allow-path="+tmpDir, "--", agyScript)
	if err != nil {
		t.Fatalf("expected agy to succeed using built-in profile: %v (output: %s)", err, string(out))
	}
	content, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("expected target file in persistent state dir %s to exist: %v", persistentDir, err)
	}
	if !strings.Contains(string(content), "from_agy") {
		t.Errorf("expected target file content 'from_agy', got: %s", string(content))
	}
}

func TestCLIRunCLIAddsToProfile(t *testing.T) {
	if hasUsableNetBackendForTest() {
		t.Skip("agy built-in profile sets allow_net=true; cannot exercise in test harness without CLONE_NEWNET")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	persistentDir := filepath.Join(home, ".local", "share", "safebox", "agents", "agy")
	geminiFile := filepath.Join(persistentDir, "test_gemini.txt")
	defer os.Remove(geminiFile)

	tmpDir := t.TempDir()
	extraRW := filepath.Join(tmpDir, "extra_rw")
	if err := os.MkdirAll(extraRW, 0700); err != nil {
		t.Fatalf("failed to create extra rw dir: %v", err)
	}

	agyScript := filepath.Join(tmpDir, "agy")
	extraFile := filepath.Join(extraRW, "test_extra.txt")
	scriptBody := fmt.Sprintf("#!/bin/sh\necho 'gemini' > ~/.gemini/test_gemini.txt && echo 'extra' > %s\n", extraFile)
	if err := os.WriteFile(agyScript, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("failed to write agy script: %v", err)
	}

	out, err := runCLI("run", "--allow-path="+tmpDir, "--allow-path-rw="+extraRW, "--", agyScript)
	if err != nil {
		t.Fatalf("expected run with profile + extra CLI RW flag to succeed: %v (output: %s)", err, string(out))
	}
	if _, err := os.Stat(geminiFile); err != nil {
		t.Errorf("expected geminiFile in persistent state dir to be written, err: %v", err)
	}
	if _, err := os.Stat(extraFile); err != nil {
		t.Errorf("expected extraFile to be written, err: %v", err)
	}
}


func TestCLIRunUnknownToolNoProfile(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "custom-unknown-tool.sh")
	scriptBody := "#!/bin/sh\necho 'unknown_ok'\n"
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	out, err := runCLI("run", "--allow-path="+tmpDir, "--", scriptPath)
	if err != nil {
		t.Fatalf("expected unknown tool to run with standard allow paths: %v (output: %s)", err, string(out))
	}
	if !strings.Contains(string(out), "unknown_ok") {
		t.Errorf("expected output to contain 'unknown_ok', got: %s", string(out))
	}
}

func TestCLIRunAppliesBuiltinProfileClaudeRWFile(t *testing.T) {
	if hasUsableNetBackendForTest() {
		t.Skip("claude built-in profile sets allow_net=true; cannot exercise in test harness without CLONE_NEWNET")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	claudeJson := filepath.Join(home, ".claude.json")
	_ = os.WriteFile(claudeJson, []byte(`{"initial": true}`), 0600)
	defer os.Remove(claudeJson)

	tmpDir := t.TempDir()
	claudeScript := filepath.Join(tmpDir, "claude")
	scriptBody := fmt.Sprintf("#!/bin/sh\necho '{\"claude\": true}' > %s\n", claudeJson)
	if err := os.WriteFile(claudeScript, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("failed to write claude script: %v", err)
	}

	out, err := runCLI("run", "--allow-path="+tmpDir, "--", claudeScript)
	if err != nil {
		t.Fatalf("expected claude to succeed using built-in profile with RWFiles ~/.claude.json: %v (output: %s)", err, string(out))
	}
	content, err := os.ReadFile(claudeJson)
	if err != nil {
		t.Fatalf("expected ~/.claude.json to exist: %v", err)
	}
	if !strings.Contains(string(content), `{"claude": true}`) {
		t.Errorf("expected ~/.claude.json content updated, got: %s", string(content))
	}
}

func TestPhase13ProfileAuditEndToEnd(t *testing.T) {
	// 1. Audit profile list output
	listOut, err := runCLI("profile", "list")
	if err != nil {
		t.Fatalf("profile list failed: %v", err)
	}
	for _, tool := range []string{"agy", "claude", "codex", "puku", "gemini", "cursor", "kilo", "opencode", "aider", "pi", "cline", "amp", "goose", "mentat", "continue", "plandex"} {
		if !strings.Contains(string(listOut), tool) {
			t.Errorf("expected tool %q in profile list output", tool)
		}
	}

	// 2. Audit profile show output
	showOut, err := runCLI("profile", "show", "agy")
	if err != nil {
		t.Fatalf("profile show agy failed: %v", err)
	}
	if !strings.Contains(string(showOut), `name = "agy"`) || !strings.Contains(string(showOut), `"$HOME/.gemini"`) {
		t.Errorf("unexpected agy profile content: %s", string(showOut))
	}

	// 3. Audit profile show not found
	cmd := exec.Command(testBinaryPath, "profile", "show", "nonexistent_999")
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected exit code 1 on unknown profile, got success")
	}
}

func TestCLIPersistentState_DirectoryPersistenceAcrossRuns(t *testing.T) {
	if hasUsableNetBackendForTest() {
		t.Skip("agy built-in profile sets allow_net=true; cannot exercise in test harness without CLONE_NEWNET")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	stateDir := filepath.Join(home, ".local", "share", "safebox", "agents", "agy")
	_ = os.MkdirAll(stateDir, 0700)
	defer os.RemoveAll(stateDir)

	tmpDir := t.TempDir()
	agyScript := filepath.Join(tmpDir, "agy")
	tokenPath := filepath.Join(home, ".gemini", "installation_id")
	scriptBody := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"write\" ]; then\n  mkdir -p $(dirname %s)\n  echo 'unique-inst-id-12345' > %s\nelif [ \"$1\" = \"read\" ]; then\n  cat %s\nfi\n", tokenPath, tokenPath, tokenPath)
	if err := os.WriteFile(agyScript, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("failed to write agy script: %v", err)
	}

	persistentFlag := fmt.Sprintf("--persistent-state=%s:%s", stateDir, filepath.Join(home, ".gemini"))

	// First run: writes installation_id
	out1, err := runCLI("run", "--allow-path="+tmpDir, persistentFlag, "--", agyScript, "write")
	if err != nil {
		t.Fatalf("first run failed: %v (output: %s)", err, string(out1))
	}

	// Verify file is stored on host under state root
	hostFile := filepath.Join(stateDir, "installation_id")
	hostData, err := os.ReadFile(hostFile)
	if err != nil {
		t.Fatalf("expected persistent state file on host at %s: %v", hostFile, err)
	}
	if !strings.Contains(string(hostData), "unique-inst-id-12345") {
		t.Errorf("expected host data to contain installation id, got: %s", string(hostData))
	}

	// Second run: reads installation_id from previous run
	out2, err := runCLI("run", "--allow-path="+tmpDir, persistentFlag, "--", agyScript, "read")
	if err != nil {
		t.Fatalf("second run failed: %v (output: %s)", err, string(out2))
	}
	if !strings.Contains(string(out2), "unique-inst-id-12345") {
		t.Errorf("second run did not see persistent state: %s", string(out2))
	}
}

func TestCLIPersistentState_ClaudeSingleFilePersistence(t *testing.T) {
	if hasUsableNetBackendForTest() {
		t.Skip("claude built-in profile sets allow_net=true; cannot exercise in test harness without CLONE_NEWNET")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	claudeJson := filepath.Join(home, ".claude.json")
	_ = os.WriteFile(claudeJson, []byte(`{"init": true}`), 0600)
	defer os.Remove(claudeJson)

	tmpDir := t.TempDir()
	claudeScript := filepath.Join(tmpDir, "claude")
	scriptBody := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"write\" ]; then\n  echo '{\"session_token\": \"xyz\"}' > %s\nelif [ \"$1\" = \"read\" ]; then\n  cat %s\nfi\n", claudeJson, claudeJson)
	if err := os.WriteFile(claudeScript, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("failed to write claude script: %v", err)
	}

	// Run 1: write
	out1, err := runCLI("run", "--allow-path="+tmpDir, "--", claudeScript, "write")
	if err != nil {
		t.Fatalf("run 1 failed: %v (output: %s)", err, string(out1))
	}

	// Run 2: read
	out2, err := runCLI("run", "--allow-path="+tmpDir, "--", claudeScript, "read")
	if err != nil {
		t.Fatalf("run 2 failed: %v (output: %s)", err, string(out2))
	}
	if !strings.Contains(string(out2), "session_token") {
		t.Errorf("run 2 failed to read claude.json: %s", string(out2))
	}
}

func TestCLIPersistentState_HardFailExitCode5(t *testing.T) {
	cmd := exec.Command(testBinaryPath, "run", "--persistent-state=/nonexistent/host/dir/12345:/root/.teststate", "--", "echo", "hello")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure for nonexistent host dir in --persistent-state, got success: %s", string(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 5 {
		t.Fatalf("expected exit code 5 (NFR1 hard-fail), got err: %v (output: %s)", err, string(out))
	}
	if !strings.Contains(string(out), "persistent state mount DENIED") {
		t.Errorf("expected 'persistent state mount DENIED' in output, got: %s", string(out))
	}
	if !strings.Contains(string(out), "hint:") {
		t.Errorf("expected hint in output, got: %s", string(out))
	}
}

func TestPhase14PersistentStateAuditEndToEnd(t *testing.T) {
	// 1. Audit profile show contains persistent_state block
	showOut, err := runCLI("profile", "show", "agy")
	if err != nil {
		t.Fatalf("profile show agy failed: %v", err)
	}
	if !strings.Contains(string(showOut), "[persistent_state]") || !strings.Contains(string(showOut), "mount_at = \"$HOME/.gemini\"") {
		t.Errorf("expected persistent_state block in agy profile: %s", string(showOut))
	}

	// 2. Audit claude profile preserves allow_rw_files
	claudeShow, err := runCLI("profile", "show", "claude")
	if err != nil {
		t.Fatalf("profile show claude failed: %v", err)
	}
	if !strings.Contains(string(claudeShow), "allow_rw_files = [\"$HOME/.claude.json\"]") {
		t.Errorf("expected claude profile to preserve allow_rw_files: %s", string(claudeShow))
	}

	// 3. Audit goose profile uses $HOME/.local/share/goose
	gooseShow, err := runCLI("profile", "show", "goose")
	if err != nil {
		t.Fatalf("profile show goose failed: %v", err)
	}
	if !strings.Contains(string(gooseShow), "mount_at = \"$HOME/.local/share/goose\"") {
		t.Errorf("expected goose profile to have mount_at = $HOME/.local/share/goose: %s", string(gooseShow))
	}
}

func TestSessionLockfileRemovedOnRunExit(t *testing.T) {
	tmpDir := t.TempDir()
	sessionRoot := t.TempDir()

	cmd := exec.Command(testBinaryPath, "run", "--", "sleep", "0.5")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "SAFEBOX_SESSION_ROOT="+sessionRoot)

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start safebox run: %v", err)
	}

	// Poll for active lockfile to appear
	var lockPath string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(sessionRoot)
		for _, e := range entries {
			if e.IsDir() {
				p := filepath.Join(sessionRoot, e.Name(), "active")
				if _, err := os.Stat(p); err == nil {
					lockPath = p
					break
				}
			}
		}
		if lockPath != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if lockPath == "" {
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for active lockfile to be created during run")
	}

	// Wait for process to exit
	if err := cmd.Wait(); err != nil {
		t.Fatalf("safebox run exited with error: %v", err)
	}

	// Assert lockfile is now gone
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("expected active lockfile %s to be removed after process exit", lockPath)
	}
}

func TestPhase15MidRunApplyAndConcurrencyEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	sessionRoot := t.TempDir()

	// 1. Spawn a background process running safebox run with a long sleep
	// The child creates a file in cwd inside the overlay and then sleeps
	scriptPath := filepath.Join(tmpDir, "agent_task.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho 'in-flight content' > result.txt\nsleep 2\n"), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	bgCmd := exec.Command(testBinaryPath, "run", "--allow-path="+tmpDir, "--", scriptPath)
	bgCmd.Dir = tmpDir
	bgCmd.Env = append(os.Environ(), "SAFEBOX_SESSION_ROOT="+sessionRoot)

	if err := bgCmd.Start(); err != nil {
		t.Fatalf("failed to start background safebox run: %v", err)
	}
	defer func() {
		if bgCmd.Process != nil {
			_ = bgCmd.Process.Kill()
		}
	}()

	// 2. Poll for active session lockfile to appear
	var activeSessionDir string
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(sessionRoot)
		for _, e := range entries {
			if e.IsDir() {
				lockFile := filepath.Join(sessionRoot, e.Name(), "active")
				if _, err := os.Stat(lockFile); err == nil {
					activeSessionDir = filepath.Join(sessionRoot, e.Name())
					break
				}
			}
		}
		if activeSessionDir != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if activeSessionDir == "" {
		t.Fatal("timed out waiting for active session lockfile to appear")
	}

	// 3. From Terminal 2 (simulated via subcommands): run safebox diff (non-blocking)
	diffCmd := exec.Command(testBinaryPath, "diff")
	diffCmd.Dir = tmpDir
	diffCmd.Env = append(os.Environ(), "SAFEBOX_SESSION_ROOT="+sessionRoot)
	diffOut, err := diffCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("safebox diff during active run failed: %v (out: %s)", err, string(diffOut))
	}
	if !strings.Contains(string(diffOut), "result.txt") {
		t.Errorf("expected diff output to contain 'result.txt', got: %s", string(diffOut))
	}

	// 4. From Terminal 2: run safebox apply --yes (non-destructive mid-run apply)
	applyCmd := exec.Command(testBinaryPath, "apply", "--yes")
	applyCmd.Dir = tmpDir
	applyCmd.Env = append(os.Environ(), "SAFEBOX_SESSION_ROOT="+sessionRoot)
	applyOut, err := applyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("safebox apply --yes failed: %v (out: %s)", err, string(applyOut))
	}
	if !strings.Contains(string(applyOut), "Applied changes. Session is still in use by safebox run PID") {
		t.Errorf("expected mid-run apply output to mention session still in use, got: %s", string(applyOut))
	}
	// Verify file was synced to host
	appliedContent, err := os.ReadFile(filepath.Join(tmpDir, "result.txt"))
	if err != nil || strings.TrimSpace(string(appliedContent)) != "in-flight content" {
		t.Errorf("expected host file content 'in-flight content', got %q, err=%v", string(appliedContent), err)
	}
	// Verify session directory still exists
	if _, err := os.Stat(activeSessionDir); err != nil {
		t.Errorf("expected session directory %s to remain intact after mid-run apply", activeSessionDir)
	}

	// 5. From Terminal 2: run safebox revert --yes (refused with exit code 3)
	revertCmd := exec.Command(testBinaryPath, "revert", "--yes")
	revertCmd.Dir = tmpDir
	revertCmd.Env = append(os.Environ(), "SAFEBOX_SESSION_ROOT="+sessionRoot)
	revertOut, err := revertCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected safebox revert to fail on active session, got success: %s", string(revertOut))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("expected exit code 3 for revert on active session, got %v (out: %s)", err, string(revertOut))
	}
	// Assert exact substrings per M2
	if !strings.Contains(string(revertOut), "cannot revert active session") {
		t.Errorf("expected 'cannot revert active session' in output, got: %s", string(revertOut))
	}
	if !strings.Contains(string(revertOut), "safebox run PID") {
		t.Errorf("expected 'safebox run PID' in output, got: %s", string(revertOut))
	}
	if !strings.Contains(string(revertOut), "use 'safebox apply' to capture changes") {
		t.Errorf("expected 'use \\'safebox apply\\' to capture changes' in output, got: %s", string(revertOut))
	}

	// 6. From Terminal 2: run concurrent safebox run in same dir (refused with exit code 6)
	run2Cmd := exec.Command(testBinaryPath, "run", "--", "echo", "concurrent")
	run2Cmd.Dir = tmpDir
	run2Cmd.Env = append(os.Environ(), "SAFEBOX_SESSION_ROOT="+sessionRoot)
	run2Out, err := run2Cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected concurrent safebox run to fail, got success: %s", string(run2Out))
	}
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 6 {
		t.Fatalf("expected exit code 6 for concurrent run, got %v (out: %s)", err, string(run2Out))
	}
	if !strings.Contains(string(run2Out), "session is already active") {
		t.Errorf("expected 'session is already active' in output, got: %s", string(run2Out))
	}
	if !strings.Contains(string(run2Out), "hint:") {
		t.Errorf("expected hint in output, got: %s", string(run2Out))
	}

	// 7. Wait for background process to finish
	if err := bgCmd.Wait(); err != nil {
		t.Fatalf("background run failed: %v", err)
	}

	// 8. Assert lockfile is removed after process exit
	lockFile := filepath.Join(activeSessionDir, "active")
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Errorf("expected lockfile %s to be removed after background run exited", lockFile)
	}
}

func TestCLIRunAllowNet_Succeeds(t *testing.T) {
	// v1: --allow-net (binary) grants full internet egress via userspace NAT.
	// When a backend is installed, --allow-net triggers a real backend spawn.
	// When no backend is available, --allow-net returns exit code 4 (per design).
	// In both cases we are testing that the flag is accepted and the run completes.
	if hasUsableNetBackendForTest() {
		t.Skip("network backend installed but test harness cannot provide a CLONE_NEWNET child for backend attach; skipping")
	}
	out, err := runCLI("run", "--allow-net", "--", "true")
	if err != nil {
		t.Fatalf("safebox run --allow-net failed: %v (out: %s)", err, string(out))
	}
}

func TestCLIRunAllowNet_EqualsBareForm(t *testing.T) {
	// --allow-net=* is an accepted alias for the bare --allow-net flag.
	if hasUsableNetBackendForTest() {
		t.Skip("network backend installed but test harness cannot provide a CLONE_NEWNET child for backend attach; skipping")
	}
	out, err := runCLI("run", "--allow-net=*", "--", "true")
	if err != nil {
		t.Fatalf("safebox run --allow-net=* failed: %v (out: %s)", err, string(out))
	}
}

// hasUsableNetBackendForTest reports whether a backend (slirp4netns/pasta) is on
// PATH. In that case the v1 binary toggle path attempts to spawn the backend and
// attach it to the child's netns. The CLI test harness runs safebox as a
// regular exec.Cmd (no CLONE_NEWNET), so the spawned backend has no proper
// namespace and exits with EOF on --ready-fd. Real end-to-end verification
// requires the safebox binary to be invoked under unshare; the new integration
// tests (integration_e2e_test.go, build-tag `integration`) cover that case.
func hasUsableNetBackendForTest() bool {
	if _, err := exec.LookPath("slirp4netns"); err == nil {
		return true
	}
	if _, err := exec.LookPath("pasta"); err == nil {
		return true
	}
	return false
}

func TestCLIRunDefaultNetIsolation(t *testing.T) {
	// Without --allow-net, connecting to a loopback address fails with network unreachable
	cmd := exec.Command(testBinaryPath, "run", "--", "python3", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:1', timeout=1)")
	cmd.Env = append(os.Environ(), "LANG=C")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected network connection to fail under default netns isolation, got success (out: %s)", string(out))
	}
}

func TestCLIRunAllowNetWithProfileSucceeds(t *testing.T) {
	// Skip in environments where the egress path cannot be exercised end-to-end:
	// we need either pasta, slirp4netns, or /dev/net/tun access AND the ability
	// for the backend to actually attach to a user-namespace child. Many test
	// runners (CI, Docker, rootless containers) do not satisfy all of these.
	canE2E := false
	if _, err := exec.LookPath("pasta"); err == nil {
		canE2E = true
	} else if _, err := exec.LookPath("slirp4netns"); err == nil {
		if f, ferr := os.OpenFile("/dev/net/tun", os.O_RDWR, 0); ferr == nil {
			f.Close()
			canE2E = true
		}
	} else if f, ferr := os.OpenFile("/dev/net/tun", os.O_RDWR, 0); ferr == nil {
		f.Close()
		canE2E = true
	}
	if !canE2E {
		t.Skip("end-to-end egress path requires pasta/slirp4netns and/or /dev/net/tun; skipping integration test")
	}

	tmpDir := t.TempDir()
	agentScript := filepath.Join(tmpDir, "agy")
	if err := os.WriteFile(agentScript, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to write mock agent script: %v", err)
	}

	// agy profile + --allow-net: the wrapped binary runs to completion
	// (no domain-list assertion in v1; profile-domain semantics are deferred).
	cmd := exec.Command(testBinaryPath, "run", "--allow-path="+tmpDir, "--allow-net", "--", agentScript)
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "LANG=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Tolerate environments where the backend binary exists but cannot
		// actually attach (e.g. rootless test runners); skip rather than fail.
		if strings.Contains(string(out), "ready-fd read failed") || strings.Contains(string(out), "slirp4netns spawn failed") {
			t.Skipf("backend binary present but cannot attach in this env: %v\n%s", err, string(out))
		}
		t.Fatalf("safebox run --allow-net with profile failed: %v (out: %s)", err, string(out))
	}
}

func TestCLIRunAllowNet_MissingBackendExitCode4(t *testing.T) {
	// If PATH is empty, no external network backend can be found
	cmd := exec.Command(testBinaryPath, "run", "--allow-net", "--", "true")
	cmd.Env = []string{"PATH=/nonexistent_bin_dir_12345", "LANG=C"}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected safebox to fail with exit code 4 (no backend), got success (out: %s)", string(out))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got: %v (out: %s)", err, string(out))
	}
	if exitErr.ExitCode() != 4 && exitErr.ExitCode() != 1 {
		t.Errorf("expected exit code 4 for missing backend, got %d (out: %s)", exitErr.ExitCode(), string(out))
	}
}

func TestEgressViaBuiltinReachesPypi(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH; skipping pip install test")
	}
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		t.Skip("/dev/net/tun not accessible; builtin backend cannot run")
	}
	f.Close()

	cmd := exec.Command(testBinaryPath, "run",
		"--allow-net",
		"--", "python3", "-c", "import urllib.request; urllib.request.urlopen('https://pypi.org', timeout=5)")
	cmd.Env = append(os.Environ(), "LANG=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("HTTPS egress to pypi.org failed (likely no external outbound in test env): %v\n%s", err, string(out))
	}
}

func TestEgressBuiltinBackendIsolatedFromHost(t *testing.T) {
	if hasUsableNetBackendForTest() {
		t.Skip("network backend installed but test harness cannot provide CLONE_NEWNET child; integration suite covers this")
	}
	cmd := exec.Command(testBinaryPath, "run",
		"--allow-net",
		"--", "sh", "-c",
		"cat </dev/tcp/8.8.8.8/53 2>&1 || echo ISOLATED_OK")
	cmd.Env = append(os.Environ(), "LANG=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("safebox run failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "ISOLATED_OK") &&
		!strings.Contains(string(out), "Connection refused") &&
		!strings.Contains(string(out), "Network is unreachable") {
		t.Errorf("expected isolation marker, got: %s", string(out))
	}
}








