package exec

import (
	"fmt"
	"os"
	osexec "os/exec"
	"syscall"
)

// Run replaces the current process image with the target command using syscall.Exec.
func Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	binPath, err := osexec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("%q not found: %w", args[0], err)
	}
	return syscall.Exec(binPath, args, os.Environ())
}
