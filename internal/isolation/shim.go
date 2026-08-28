package isolation

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"os/signal"
	"syscall"
)

// RunShim executes the wrapped command as a supervised child process (PID 2)
// inside the PID namespace while keeping the current process as PID 1 to
// forward signals and reap reparented orphan processes.
func RunShim(args []string) error {
	if len(args) == 0 {
		return errors.New("safebox: no command specified")
	}

	binPath, err := osexec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("safebox: %q not found: %w", args[0], err)
	}

	sigCh := make(chan os.Signal, 16)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(sigCh)

	childPid, err := syscall.ForkExec(binPath, args, &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{0, 1, 2},
	})
	if err != nil {
		return fmt.Errorf("safebox: fork/exec failed: %w", err)
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				if sysSig, ok := sig.(syscall.Signal); ok {
					_ = syscall.Kill(childPid, sysSig)
				}
			case <-done:
				return
			}
		}
	}()

	var status syscall.WaitStatus
	for {
		pid, waitErr := syscall.Wait4(-1, &status, 0, nil)
		if pid == childPid {
			close(done)
			if status.Signaled() {
				os.Exit(128 + int(status.Signal()))
			}
			os.Exit(status.ExitStatus())
		}
		if waitErr != nil {
			if errors.Is(waitErr, syscall.ECHILD) {
				break
			}
			break
		}
	}
	return nil
}
