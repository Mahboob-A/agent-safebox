package cli

import (
	"fmt"
	"os"

	"safebox/internal/revert"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

// RunDiff executes the 'safebox diff' command.
func RunDiff(args []string, tr *trace.Tracer) int {
	flags, err := ParseDiffApplyFlags(args, "diff")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n\n", ui.StyleDenied.Render("ERROR"), err)
		PrintUsageTo(os.Stderr)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		PrintSubcommandError("diff", fmt.Errorf("failed to get working directory: %w", err))
		return 1
	}

	var sess *revert.Session
	_ = tr.Step("session discovery", func() error {
		var sErr error
		sess, sErr = revert.MostRecentSession(cwd, false)
		return sErr
	})

	if sess != nil {
		if err := tr.Step("diff computation", func() error {
			if flags.Patch {
				return revert.RunShadowPatch(cwd, sess.UpperDir, os.Stdout, flags.Paths...)
			}
			return revert.RunShadowDiff(cwd, sess.UpperDir, os.Stdout, flags.Paths...)
		}); err != nil {
			PrintSubcommandError("diff", err)
			return 1
		}
		return 0
	}

	if err := tr.Step("diff computation", func() error {
		if flags.Patch {
			return revert.RunGitPatch(cwd, os.Stdout, flags.Paths...)
		}
		return revert.RunDiff(cwd, os.Stdout, flags.Paths...)
	}); err != nil {
		PrintSubcommandError("diff", err)
		return 1
	}

	return 0
}
