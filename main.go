package main

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"

	"safebox/internal/exec"
	"safebox/internal/isolation"
	"safebox/internal/revert"
	"safebox/internal/ui"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: safebox <command> [arguments]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  run [--] <cmd...>   Run a command inside the sandbox\n")
	fmt.Fprintf(os.Stderr, "  diff                Show modified, added, and deleted files\n")
	fmt.Fprintf(os.Stderr, "  revert [--yes]      Discard all working tree changes\n")
	fmt.Fprintf(os.Stderr, "  help                Show help documentation\n")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "%s safebox: missing command\n\n", ui.StyleDenied.Render("ERROR"))
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	switch subcommand {
	case "run":
		// Strip leading "--" if provided
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "%s safebox run: no command specified\n\n", ui.StyleDenied.Render("ERROR"))
			printUsage()
			os.Exit(1)
		}
		if err := isolation.ReexecChild(args); err != nil {
			var exitErr *osexec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "%s %v\n", ui.StyleDenied.Render("ERROR"), err)
			os.Exit(1)
		}

	case "__child__":
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "%s safebox __child__: missing wrapped command\n", ui.StyleDenied.Render("ERROR"))
			os.Exit(1)
		}
		if err := isolation.ApplyLandlock(); err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", ui.StyleDenied.Render("ERROR"), err)
			os.Exit(1)
		}
		if err := exec.Run(args); err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: exec failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
			os.Exit(1)
		}

	case "diff":
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: failed to get working directory: %v\n", ui.StyleDenied.Render("ERROR"), err)
			os.Exit(1)
		}
		if err := revert.RunDiff(cwd, os.Stdout); err != nil {
			if errors.Is(err, revert.ErrNotGitRepo) {
				fmt.Fprintf(os.Stderr, "%s safebox diff: not a git repository (or any of the parent directories)\n", ui.StyleDenied.Render("ERROR"))
			} else {
				fmt.Fprintf(os.Stderr, "%s safebox diff: %v\n", ui.StyleDenied.Render("ERROR"), err)
			}
			os.Exit(1)
		}

	case "revert":
		force := false
		for _, arg := range os.Args[2:] {
			if arg == "--yes" || arg == "-y" || arg == "--yes=true" {
				force = true
			}
		}

		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: failed to get working directory: %v\n", ui.StyleDenied.Render("ERROR"), err)
			os.Exit(1)
		}

		if err := revert.Revert(cwd, force, os.Stdin, os.Stdout); err != nil {
			if errors.Is(err, revert.ErrNotGitRepo) {
				fmt.Fprintf(os.Stderr, "%s safebox revert: not a git repository (or any of the parent directories)\n", ui.StyleDenied.Render("ERROR"))
				os.Exit(1)
			} else if errors.Is(err, revert.ErrRevertCancelled) {
				os.Exit(0)
			} else {
				fmt.Fprintf(os.Stderr, "%s safebox revert: %v\n", ui.StyleDenied.Render("ERROR"), err)
				os.Exit(1)
			}
		}

	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "%s safebox: unknown command %q\n\n", ui.StyleDenied.Render("ERROR"), subcommand)
		printUsage()
		os.Exit(1)
	}
}
