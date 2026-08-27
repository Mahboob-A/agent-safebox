package main

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"

	"safebox/internal/exec"
	"safebox/internal/isolation"
	"safebox/internal/revert"
	"safebox/internal/ui"
)

func printSubcommandError(subcommand string, err error) {
	fmt.Fprintf(os.Stderr, "%s safebox %s: %v\n", ui.StyleDenied.Render("ERROR"), subcommand, err)
}

func parseShadowFlag(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--shadow=") {
			return strings.TrimPrefix(arg, "--shadow=")
		}
	}
	return ""
}

func hasYesFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" || arg == "--yes=true" {
			return true
		}
	}
	return false
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: safebox <command> [arguments]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  run [--] <cmd...>             Run a command inside the sandbox\n")
	fmt.Fprintf(os.Stderr, "  diff [--shadow=<dir>]         Show modified, added, and deleted files\n")
	fmt.Fprintf(os.Stderr, "  revert [--yes|-y]             Discard all working tree changes\n")
	fmt.Fprintf(os.Stderr, "  apply --shadow=<dir> [--yes]  Apply shadow changes to working directory\n")
	fmt.Fprintf(os.Stderr, "  help                          Show help documentation\n")
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
		shadowUpper := parseShadowFlag(args)
		cwd, err := os.Getwd()
		if err != nil {
			printSubcommandError("diff", fmt.Errorf("failed to get working directory: %w", err))
			os.Exit(1)
		}

		if shadowUpper != "" {
			if _, err := os.Stat(shadowUpper); err != nil {
				printSubcommandError("diff", fmt.Errorf("shadow directory %q does not exist: %w", shadowUpper, err))
				os.Exit(1)
			}
			if err := revert.RunShadowDiff(cwd, shadowUpper, os.Stdout); err != nil {
				printSubcommandError("diff", err)
				os.Exit(1)
			}
			os.Exit(0)
		}

		if err := revert.RunDiff(cwd, os.Stdout); err != nil {
			printSubcommandError("diff", err)
			os.Exit(1)
		}

	case "revert":
		force := hasYesFlag(args)
		cwd, err := os.Getwd()
		if err != nil {
			printSubcommandError("revert", fmt.Errorf("failed to get working directory: %w", err))
			os.Exit(1)
		}

		if err := revert.Revert(cwd, force, os.Stdin, os.Stdout); err != nil {
			if errors.Is(err, revert.ErrRevertCancelled) {
				os.Exit(0)
			}
			printSubcommandError("revert", err)
			os.Exit(1)
		}

	case "apply":
		shadowUpper := parseShadowFlag(args)
		if shadowUpper == "" {
			printSubcommandError("apply", errors.New("--shadow=<dir> argument is required"))
			os.Exit(1)
		}

		if _, err := os.Stat(shadowUpper); err != nil {
			printSubcommandError("apply", fmt.Errorf("shadow directory %q does not exist: %w", shadowUpper, err))
			os.Exit(1)
		}

		force := hasYesFlag(args)
		if !force {
			fmt.Fprintf(os.Stdout, "%s Apply shadow changes to working directory? [y/N]: ", ui.StyleMeta.Render("PROMPT"))
			var response string
			if _, err := fmt.Fscanln(os.Stdin, &response); err != nil || (response != "y" && response != "yes" && response != "Y" && response != "YES") {
				fmt.Fprintf(os.Stdout, "%s\n", ui.StyleMeta.Render("Apply cancelled."))
				os.Exit(0)
			}
		}

		cwd, err := os.Getwd()
		if err != nil {
			printSubcommandError("apply", fmt.Errorf("failed to get working directory: %w", err))
			os.Exit(1)
		}

		if err := revert.ApplyShadowChanges(cwd, shadowUpper); err != nil {
			printSubcommandError("apply", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "%s\n", ui.StyleAllowed.Render("Shadow changes applied to working directory."))

	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "%s safebox: unknown command %q\n\n", ui.StyleDenied.Render("ERROR"), subcommand)
		printUsage()
		os.Exit(1)
	}
}
