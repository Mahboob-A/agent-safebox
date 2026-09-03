package cli

import (
	"errors"
	"fmt"
	"os"

	"safebox/internal/revert"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunRevert executes the 'safebox revert' command.
func RunRevert(args []string, tr *trace.Tracer) int {
	flags, err := ParseDiffApplyFlags(args, "revert")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n\n", ui.StyleDenied.Render("ERROR"), err)
		PrintUsageTo(os.Stderr)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		PrintSubcommandError("revert", fmt.Errorf("failed to get working directory: %w", err))
		return 1
	}

	var sess *revert.Session
	_ = tr.Step("session discovery", func() error {
		var sErr error
		sess, sErr = revert.MostRecentSession(cwd, true)
		return sErr
	})

	if sess != nil {
		active, activePID, _ := sess.IsActive()
		if active && !flags.ForceDiscard {
			fmt.Fprintf(os.Stderr, "%s safebox revert: cannot revert active session (safebox run PID %d is running). Terminate the running process before reverting, or use 'safebox apply' to capture changes.\n", ui.StyleDenied.Render("ERROR"), activePID)
			return 3
		}

		if active && flags.ForceDiscard {
			fmt.Fprintf(os.Stderr, "%s Forced session revert while safebox run PID %d was active.\n", ui.StyleDenied.Render("WARNING:"), activePID)
		}

		if !flags.Yes && !flags.ForceDiscard {
			fmt.Fprintf(os.Stdout, "%s Discard active overlay session changes? [y/N]: ", ui.StyleMeta.Render("PROMPT"))
			var response string
			if _, err := fmt.Fscanln(os.Stdin, &response); err != nil || (response != "y" && response != "yes" && response != "Y" && response != "YES") {
				fmt.Fprintf(os.Stdout, "%s\n", ui.StyleMeta.Render("Revert cancelled."))
				return 0
			}
		}
		if err := tr.Step("discard session", func() error {
			return revert.DiscardSession(sess)
		}); err != nil {
			PrintSubcommandError("revert", fmt.Errorf("failed to discard session: %w", err))
			return 1
		}
		fmt.Fprintf(os.Stdout, "%s\n", ui.StyleAllowed.Render("Overlay session discarded. Working directory remains unchanged."))
		return 0
	}

	if err := tr.Step("git revert", func() error {
		return revert.Revert(cwd, flags.Yes, os.Stdin, os.Stdout)
	}); err != nil {
		if errors.Is(err, revert.ErrRevertCancelled) {
			return 0
		}
		PrintSubcommandError("revert", err)
		return 1
	}

	return 0
}
