package cli

import (
	"fmt"
	"os"

	"safebox/internal/trace"
	"safebox/internal/ui"
)

// HasQuiet checks whether -q, --quiet, or --quiet=true is present in args.
func HasQuiet(args []string) bool {
	for _, arg := range args {
		if arg == "-q" || arg == "--quiet" || arg == "--quiet=true" {
			return true
		}
	}
	return false
}

// Dispatch parses the subcommand from args and invokes the matching runner.
func Dispatch(args []string, tr *trace.Tracer) int {
	if len(args) == 0 {
		return RunHelp(os.Stdout)
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "run":
		return RunRun(subArgs, tr)
	case "__child__":
		return RunChild(subArgs, tr)
	case "diff":
		return RunDiff(subArgs, tr)
	case "revert":
		return RunRevert(subArgs, tr)
	case "apply":
		return RunApply(subArgs, tr)
	case "profile":
		return RunProfile(subArgs, tr)
	case "cat":
		return RunCat(subArgs, tr)
	case "help", "-h", "--help":
		return RunHelp(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "%s safebox: unknown command %q\n\n", ui.StyleDenied.Render("ERROR"), subcommand)
		PrintUsageTo(os.Stderr)
		return 1
	}
}
