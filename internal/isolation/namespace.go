package isolation

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// DefaultSysProcAttr returns the SysProcAttr configured with user, mount,
// and network namespace clone flags and container root UID/GID mappings.
func DefaultSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET |
			syscall.CLONE_NEWIPC | syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
	}
}

// ChildHandle is the parent's handle to a running namespaced child process.
// The parent uses it to:
//   - obtain the child's PID once cmd.Start() returns
//   - close the net-ready gate (signaling the child to proceed past its
//     pre-egress barrier; see RunChild for the child-side read)
//   - wait for the child to exit and surface its exit code
type ChildHandle struct {
	Cmd      *exec.Cmd
	NetReady *os.File // parent's write-end of the net-ready pipe; Close() signals the child
}

// CloseNetReady signals the child that egress setup is complete (or skipped).
// Safe to call multiple times; subsequent calls are no-ops once the fd is closed.
// On exit-error paths, the parent should always call this to unblock the child
// even when the parent is about to terminate.
func (h *ChildHandle) CloseNetReady() {
	if h == nil || h.NetReady == nil {
		return
	}
	_ = h.NetReady.Close()
	h.NetReady = nil
}

// Wait blocks until the child exits and returns its error (if any).
// An exec.ExitError carries the child's exit code.
func (h *ChildHandle) Wait() error {
	h.CloseNetReady()
	return h.Cmd.Wait()
}

// StartChild prepares and starts the namespaced child process. Unlike
// ReexecChild this does NOT block waiting for the child to exit; the caller
// is expected to:
//  1. Read h.Cmd.Process.Pid after Start() returns.
//  2. Attach any userspace-NAT backend to that PID (netpolicy.SetupEgress).
//  3. Wait for backend readiness (slirp4netns --ready-fd, etc.).
//  4. Call h.CloseNetReady() to release the child past its pre-egress barrier.
//  5. Call h.Wait() to block on the child.
//
// If the child is started without a network backend (default-deny),
// CloseNetReady should be called immediately after Start to let the child
// proceed straight to applyEgressConfig (which is a no-op without
// flags.NetConfigPath) and onward.
func StartChild(allowPathsRO, allowPathsRW, allowFilesRW, persistentStateMounts []string, netConfigPath, sessionDir string, quiet bool, cmdArgs []string) (*ChildHandle, error) {
	exePath, err := os.Executable()
	if err != nil {
		exePath = "/proc/self/exe"
	}

	var childArgs []string
	childArgs = append(childArgs, "__child__")
	if quiet {
		childArgs = append(childArgs, "--quiet")
	}
	if sessionDir != "" {
		childArgs = append(childArgs, "--session-dir="+sessionDir)
	}
	if netConfigPath != "" {
		childArgs = append(childArgs, "--net-config="+netConfigPath)
	}
	for _, p := range allowPathsRO {
		childArgs = append(childArgs, "--allow-path="+p)
	}
	for _, p := range allowPathsRW {
		childArgs = append(childArgs, "--allow-path-rw="+p)
	}
	for _, f := range allowFilesRW {
		childArgs = append(childArgs, "--allow-file-rw="+f)
	}
	for _, m := range persistentStateMounts {
		childArgs = append(childArgs, "--persistent-state="+m)
	}
	childArgs = append(childArgs, cmdArgs...)

	// Allocate the net-ready pipe BEFORE cmd.Start so the child inherits
	// the read-end fd. The child blocks on a 1-byte read from this fd until
	// the parent signals egress readiness (or default-deny / setup failure).
	netReadyR, netReadyW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("safebox: create net-ready pipe: %w", err)
	}
	// Append the read-end to cmd.ExtraFiles so it appears as fd 3 (the
	// first slot). Communicate the fd number to the child via env var.
	netReadyFD := 3

	cmd := exec.Command(exePath, childArgs...)
	cmd.SysProcAttr = DefaultSysProcAttr()
	cmd.ExtraFiles = []*os.File{netReadyR}
	cmd.Env = append(os.Environ(),
		"SAFEBOX_NET_READY_FD="+strconv.Itoa(netReadyFD),
	)
	if netConfigPath != "" {
		cmd.Env = append(cmd.Env, "SAFEBOX_NET_CONFIG="+netConfigPath)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		_ = netReadyR.Close()
		_ = netReadyW.Close()
		return nil, fmt.Errorf("safebox: namespace isolation failed: %w", err)
	}
	// Parent no longer needs the read-end; it lives in the child now.
	_ = netReadyR.Close()

	return &ChildHandle{Cmd: cmd, NetReady: netReadyW}, nil
}

// ReexecChild is a backwards-compatible wrapper that blocks waiting for the
// child to exit. New callers should prefer StartChild + ChildHandle.Wait
// when they need to attach network backends between the child's clone and
// the rest of its setup.
func ReexecChild(allowPathsRO, allowPathsRW, allowFilesRW, persistentStateMounts []string, netConfigPath, sessionDir string, quiet bool, cmdArgs []string) error {
	handle, err := StartChild(allowPathsRO, allowPathsRW, allowFilesRW, persistentStateMounts, netConfigPath, sessionDir, quiet, cmdArgs)
	if err != nil {
		return err
	}
	if err := handle.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr
		}
		return fmt.Errorf("safebox: namespace isolation failed: %w", err)
	}
	return nil
}

// MountProc remounts a fresh instance of procfs over /proc inside the child's
// private mount and PID namespace. This ensures /proc/self accurately points to
// container PID 1 rather than host PID 1 (systemd), preventing crashes in runtimes
// (such as Bun, WebKit, and Node) that assert on /proc/self status.
func MountProc() error {
	return syscall.Mount("proc", "/proc", "proc", syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, "")
}
