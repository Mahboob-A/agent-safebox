package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"safebox/internal/isolation"
	"safebox/internal/revert"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunRun executes the 'safebox run' parent command.
func RunRun(args []string, tr *trace.Tracer) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "%s safebox run: no command specified\n\n", ui.StyleDenied.Render("ERROR"))
		PrintUsageTo(os.Stderr)
		return 1
	}

	flags, cmdArgs, err := ParseRunFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n\n", ui.StyleDenied.Render("ERROR"), err)
		PrintUsageTo(os.Stderr)
		return 2
	}

	if flags.Probe {
		return RunProbe(flags, cmdArgs)
	}

	if len(cmdArgs) == 0 {
		fmt.Fprintf(os.Stderr, "%s safebox: no command specified after '--'\n\n", ui.StyleDenied.Render("ERROR"))
		PrintUsageTo(os.Stderr)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s safebox: cannot resolve current directory: %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 1
	}

	var sess *revert.Session
	if err := tr.Step("session initialize", func() error {
		var sErr error
		sess, sErr = revert.CreateSession(cwd)
		return sErr
	}); err != nil {
		fmt.Fprintf(os.Stderr, "%s safebox: failed to initialize session: %v\n", ui.StyleDenied.Render("ERROR"), err)
		return 1
	}

	if err := tr.Step("wrapped command execution", func() error {
		return isolation.ReexecChild(flags.AllowPathsRO, flags.AllowPathsRW, sess.BaseDir, flags.Quiet, cmdArgs)
	}); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "%s safebox: exec failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
		if hint := HintFor(err, cmdArgs); hint != "" {
			fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
		}
		return 1
	}

	return 0
}
