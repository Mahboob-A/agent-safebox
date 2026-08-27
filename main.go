package main

import (
	"fmt"
	"os"

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
		// CLI routing stub for T1.1; T1.2 and T1.4 will hook namespace re-exec and syscall.Exec
		fmt.Fprintf(os.Stdout, "%s routing run command: %v\n", ui.StyleAllowed.Render("OK"), args)

	case "__child__":
		// Hidden entrypoint for namespace re-exec (T1.2)
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "%s safebox __child__: missing wrapped command\n", ui.StyleDenied.Render("ERROR"))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "%s entering child namespace handler\n", ui.StyleAllowed.Render("OK"))

	case "diff":
		// Phase 3 stub
		fmt.Fprintf(os.Stdout, "%s diff subcommand stub\n", ui.StyleMeta.Render("INFO"))

	case "revert":
		// Phase 3 stub
		fmt.Fprintf(os.Stdout, "%s revert subcommand stub\n", ui.StyleMeta.Render("INFO"))

	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "%s safebox: unknown command %q\n\n", ui.StyleDenied.Render("ERROR"), subcommand)
		printUsage()
		os.Exit(1)
	}
}
