package cli

import (
	"fmt"
	"os"

	"safebox/internal/revert"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunApply executes the 'safebox apply' command.
func RunApply(args []string, tr *trace.Tracer) int {
	flags, err := ParseDiffApplyFlags(args, "apply")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n\n", ui.StyleDenied.Render("ERROR"), err)
		PrintUsageTo(os.Stderr)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		PrintSubcommandError("apply", fmt.Errorf("failed to get working directory: %w", err))
		return 1
	}

	var sess *revert.Session
	if err := tr.Step("session discovery", func() error {
		var sErr error
		sess, sErr = revert.MostRecentSession(cwd, true)
		return sErr
	}); err != nil {
		PrintSubcommandError("apply", err)
		return 1
	}

	if !flags.Yes && !flags.ForceDiscard {
		fmt.Fprintf(os.Stdout, "%s Apply shadow changes to working directory? [y/N]: ", ui.StyleMeta.Render("PROMPT"))
		var response string
		if _, err := fmt.Fscanln(os.Stdin, &response); err != nil || (response != "y" && response != "yes" && response != "Y" && response != "YES") {
			fmt.Fprintf(os.Stdout, "%s\n", ui.StyleMeta.Render("Apply cancelled."))
			return 0
		}
	}

	if err := tr.Step("apply changes", func() error {
		return revert.ApplyShadowChanges(cwd, sess.UpperDir)
	}); err != nil {
		PrintSubcommandError("apply", err)
		return 1
	}

	active, activePID, _ := sess.IsActive()
	if active && !flags.ForceDiscard {
		fmt.Fprintf(os.Stdout, "%s Applied changes. Session is still in use by safebox run PID %d; session will be cleaned up when that process exits.\n", ui.StyleAllowed.Render("OK"), activePID)
		return 0
	}

	if active && flags.ForceDiscard {
		fmt.Fprintf(os.Stderr, "%s Forced session cleanup while safebox run PID %d was active.\n", ui.StyleDenied.Render("WARNING:"), activePID)
	}

	_ = tr.Step("session cleanup", func() error {
		return revert.DiscardSession(sess)
	})

	fmt.Fprintf(os.Stdout, "%s\n", ui.StyleAllowed.Render("Shadow changes applied to working directory."))
	return 0
}
