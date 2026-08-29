package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"safebox/internal/isolation"
)

// RunProbe prints the effective security policy and resolved binary path, then returns 0.
func RunProbe(flags RunFlags, cmdArgs []string) int {
	var binDisplay string
	if len(cmdArgs) > 0 {
		binName := cmdArgs[0]
		resolved, err := exec.LookPath(binName)
		if err == nil {
			absResolved, aErr := filepath.Abs(resolved)
			if aErr == nil {
				resolved = absResolved
			}
			if resolved != binName {
				binDisplay = fmt.Sprintf("%s (resolved from %q)", resolved, binName)
			} else {
				binDisplay = resolved
			}
		} else {
			binDisplay = fmt.Sprintf("(unresolvable: %q)", binName)
		}
	} else {
		binDisplay = "(none specified)"
	}

	report, err := isolation.ProbeLandlock(flags.AllowPathsRO, flags.AllowPathsRW)
	if err != nil {
		fmt.Fprintf(os.Stderr, "safebox: failed to probe landlock policy: %v\n", err)
		return 1
	}

	fmt.Println("safebox probe report")
	fmt.Printf("binary:      %s\n", binDisplay)
	fmt.Printf("working dir: %s (will be mounted as OverlayFS read-write)\n\n", report.WorkingDir)

	fmt.Println("Landlock allow-list (effective):")
	fmt.Println("  RW dirs:")
	for _, rw := range report.EffectiveRW {
		source := ""
		for _, userRW := range flags.AllowPathsRW {
			if rw == userRW || rw == filepath.Clean(userRW) {
				source = "          (from --allow-path-rw)"
				break
			}
		}
		fmt.Printf("    %s%s\n", rw, source)
	}

	fmt.Println("  RO dirs:")
	for _, ro := range report.EffectiveRO {
		source := ""
		for _, userRO := range flags.AllowPathsRO {
			if ro == userRO || ro == filepath.Clean(userRO) {
				source = "          (from --allow-path)"
				break
			}
		}
		fmt.Printf("    %s%s\n", ro, source)
	}

	fmt.Println("\nNetwork:    isolated (no CLONE_NEWNET bypass)")
	fmt.Println("Namespaces: user, mount, network, ipc, uts, pid")
	fmt.Println("Init shim:  yes (PID 1 forwards SIGINT/SIGTERM/SIGHUP/SIGQUIT)")
	fmt.Println("\nWrapped command will NOT be executed. Exiting.")

	return 0
}
