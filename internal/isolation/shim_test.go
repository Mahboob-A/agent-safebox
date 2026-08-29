package isolation

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestRunShimEmptyArgs(t *testing.T) {
	_, err := RunShim([]string{}, nil)
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}

func TestRunShimNonExistentBinary(t *testing.T) {
	_, err := RunShim([]string{"/non/existent/binary_xyz_123"}, nil)
	if err == nil {
		t.Fatal("expected error for non-existent binary, got nil")
	}
}

// HelperProcess is executed in a subprocess by tests requiring os.Exit isolation.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SHIM_HELPER") != "1" {
		return
	}
	args := os.Args[3:]
	code, _ := RunShim(args, nil)
	os.Exit(code)
}

func TestRunShimExitCodePropagation(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "sh", "-c", "exit 42")
	cmd.Env = append(os.Environ(), "GO_WANT_SHIM_HELPER=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code, got nil error")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 42 {
		t.Fatalf("expected exit code 42, got %d", exitErr.ExitCode())
	}
}

func TestRunShimSignalForwarding(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "sleep", "10")
	cmd.Env = append(os.Environ(), "GO_WANT_SHIM_HELPER=1")

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper process: %v", err)
	}

	// Allow child process time to spawn and enter sleep
	time.Sleep(100 * time.Millisecond)

	// Send SIGTERM to the shim supervisor process
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}

	err := cmd.Wait()
	if err == nil {
		t.Fatal("expected process to terminate via signal, got nil error")
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// Child terminated by SIGTERM (exit code 143 = 128 + 15 or signaled)
		ws, ok := exitErr.Sys().(syscall.WaitStatus)
		if ok {
			if !ws.Signaled() && ws.ExitStatus() != 143 {
				t.Fatalf("expected signaled termination or exit code 143, got status %v", ws)
			}
		}
	}
}

func TestRunShimZombieReaping(t *testing.T) {
	// Spawns a background grandchild process, then waits briefly.
	// The grandchild reparents to the shim (PID 1) and exits.
	// The shim's wait4(-1, ...) loop must reap it cleanly.
	script := "(sleep 0.05 &); sleep 0.2"
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "sh", "-c", script)
	cmd.Env = append(os.Environ(), "GO_WANT_SHIM_HELPER=1")

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected clean run, got %v", err)
	}
}
