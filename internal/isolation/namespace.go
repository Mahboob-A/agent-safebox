package isolation

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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

// ReexecChild re-executes the current binary (/proc/self/exe) with the hidden
// "__child__" subcommand inside fresh unprivileged user, mount, and network namespaces.
func ReexecChild(cmdArgs []string) error {
	exePath, err := os.Executable()
	if err != nil {
		exePath = "/proc/self/exe"
	}

	childArgs := append([]string{"__child__"}, cmdArgs...)
	cmd := exec.Command(exePath, childArgs...)
	cmd.SysProcAttr = DefaultSysProcAttr()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr
		}
		return fmt.Errorf("safebox: namespace isolation failed: %w", err)
	}
	return nil
}
