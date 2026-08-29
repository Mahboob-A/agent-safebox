package main

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"safebox/internal/isolation"
	"safebox/internal/revert"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

func hintFor(err error, cmdArgs []string) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()
	if strings.Contains(strings.ToLower(errStr), "permission denied") || errors.Is(err, syscall.EACCES) {
		if len(cmdArgs) > 0 {
			bin := cmdArgs[0]
			if strings.Contains(bin, "/") {
				dir := filepath.Dir(bin)
				return fmt.Sprintf("rerun with --allow-path=%s", dir)
			}
			return "rerun with --allow-path=<dir>"
		}
	}
	return ""
}

func hintForSubcommand(subcommand string, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, revert.ErrNoSessionFound) {
		return "run safebox run first or pass --shadow=<dir>"
	}
	if subcommand == "apply" && strings.Contains(err.Error(), "--shadow=") {
		return "pass --shadow=<dir> or run safebox run to create a session"
	}
	if errors.Is(err, revert.ErrNotGitRepo) {
		return "run inside a git repository or use safebox run to create an overlay session"
	}
	return ""
}

func printSubcommandError(subcommand string, err error) {
	fmt.Fprintf(os.Stderr, "%s safebox %s: %v\n", ui.StyleDenied.Render("ERROR"), subcommand, err)
	if hint := hintForSubcommand(subcommand, err); hint != "" {
		fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
	}
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

func hasQuietFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--quiet" || arg == "-q" || arg == "--quiet=true" {
			return true
		}
	}
	return false
}

func parseAllowPathsAndFlags(args []string, requireDoubleDash bool) (allowPaths []string, sessionDir string, quiet bool, cmdArgs []string, err error) {
	seenDoubleDash := false
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			seenDoubleDash = true
			for _, rest := range args[i+1:] {
				if strings.HasPrefix(rest, "--allow-path=") || rest == "--allow-path" {
					return nil, "", false, nil, errors.New("--allow-path must precede the -- delimiter; current invocation places it inside the wrapped command arguments")
				}
			}
			cmdArgs = append(cmdArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--allow-path=") {
			pathVal := strings.TrimPrefix(arg, "--allow-path=")
			if pathVal != "" {
				allowPaths = append(allowPaths, pathVal)
			}
			i++
			continue
		}
		if arg == "--allow-path" && i+1 < len(args) {
			allowPaths = append(allowPaths, args[i+1])
			i += 2
			continue
		}
		if strings.HasPrefix(arg, "--session-dir=") {
			sessionDir = strings.TrimPrefix(arg, "--session-dir=")
			i++
			continue
		}
		if arg == "--quiet" || arg == "-q" || arg == "--quiet=true" {
			quiet = true
			i++
			continue
		}
		if requireDoubleDash {
			return nil, "", false, nil, errors.New("safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)")
		}
		cmdArgs = append(cmdArgs, args[i:]...)
		break
	}
	if requireDoubleDash && !seenDoubleDash {
		return nil, "", false, nil, errors.New("safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)")
	}
	return allowPaths, sessionDir, quiet, cmdArgs, nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: safebox <command> [arguments]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  run [--quiet|-q] [--allow-path=<dir> ...] -- <cmd...>    Run a command inside the sandbox\n")
	fmt.Fprintf(os.Stderr, "  diff [--quiet|-q] [--shadow=<dir>]                       Show modified, added, and deleted files\n")
	fmt.Fprintf(os.Stderr, "  revert [--quiet|-q] [--yes|-y]                           Discard session or working tree changes\n")
	fmt.Fprintf(os.Stderr, "  apply [--quiet|-q] [--shadow=<dir>] [--yes]              Apply shadow changes to working directory\n")
	fmt.Fprintf(os.Stderr, "  help                                                     Show help documentation\n")
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
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "%s safebox run: no command specified\n\n", ui.StyleDenied.Render("ERROR"))
			printUsage()
			os.Exit(1)
		}
		allowPaths, _, quiet, cmdArgs, err := parseAllowPathsAndFlags(args, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n\n", ui.StyleDenied.Render("ERROR"), err)
			printUsage()
			os.Exit(2)
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "%s safebox run: no command specified\n\n", ui.StyleDenied.Render("ERROR"))
			printUsage()
			os.Exit(1)
		}

		tr := trace.New(!quiet)
		cwd, err := os.Getwd()
		if err != nil {
			printSubcommandError("run", fmt.Errorf("failed to get working directory: %w", err))
			os.Exit(1)
		}

		var sess *revert.Session
		if err := tr.Step("session initialize", func() error {
			var sErr error
			sess, sErr = revert.CreateSession(cwd)
			return sErr
		}); err != nil {
			printSubcommandError("run", fmt.Errorf("failed to create overlay session: %w", err))
			os.Exit(1)
		}

		if err := tr.Step("wrapped command execution", func() error {
			return isolation.ReexecChild(allowPaths, sess.BaseDir, quiet, cmdArgs)
		}); err != nil {
			var exitErr *osexec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "%s %v\n", ui.StyleDenied.Render("ERROR"), err)
			if hint := hintFor(err, cmdArgs); hint != "" {
				fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
			}
			os.Exit(1)
		}

	case "__child__":
		runtime.LockOSThread()
		allowPaths, sessionDir, quiet, cmdArgs, err := parseAllowPathsAndFlags(args, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox __child__: %v\n", ui.StyleDenied.Render("ERROR"), err)
			os.Exit(1)
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "%s safebox __child__: missing wrapped command\n", ui.StyleDenied.Render("ERROR"))
			os.Exit(1)
		}

		tr := trace.NewChild(!quiet)

		if sessionDir != "" {
			upperDir := filepath.Join(sessionDir, "upper")
			workDir := filepath.Join(sessionDir, "work")
			mergedDir := filepath.Join(sessionDir, "merged")
			lowerDir, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s safebox __child__: cannot get working directory: %v\n", ui.StyleDenied.Render("ERROR"), err)
				os.Exit(1)
			}
			if err := tr.Step("overlayfs mount", func() error {
				return isolation.MountSessionOverlay(lowerDir, upperDir, workDir, mergedDir)
			}); err != nil {
				fmt.Fprintf(os.Stderr, "%s safebox __child__: %v\n", ui.StyleDenied.Render("ERROR"), err)
				os.Exit(1)
			}
			defer isolation.UnmountOverlay(mergedDir)
			if err := os.Chdir(mergedDir); err != nil {
				fmt.Fprintf(os.Stderr, "%s safebox __child__: cannot change directory to overlay: %v\n", ui.StyleDenied.Render("ERROR"), err)
				os.Exit(1)
			}
		}

		if err := tr.Step("landlock restrict", func() error {
			return isolation.ApplyLandlock(allowPaths...)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", ui.StyleDenied.Render("ERROR"), err)
			if hint := hintFor(err, cmdArgs); hint != "" {
				fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
			}
			os.Exit(1)
		}

		if err := isolation.RunShim(cmdArgs, tr); err != nil {
			fmt.Fprintf(os.Stderr, "%s safebox: exec failed: %v\n", ui.StyleDenied.Render("ERROR"), err)
			if hint := hintFor(err, cmdArgs); hint != "" {
				fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
			}
			os.Exit(1)
		}

	case "diff":
		quiet := hasQuietFlag(args)
		tr := trace.New(!quiet)
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
			if err := tr.Step("diff computation", func() error {
				return revert.RunShadowDiff(cwd, shadowUpper, os.Stdout)
			}); err != nil {
				printSubcommandError("diff", err)
				os.Exit(1)
			}
			os.Exit(0)
		}

		// Check for active session
		var sess *revert.Session
		_ = tr.Step("session discovery", func() error {
			var sErr error
			sess, sErr = revert.MostRecentSession(cwd, false)
			return sErr
		})

		if sess != nil {
			if err := tr.Step("diff computation", func() error {
				return revert.RunShadowDiff(cwd, sess.UpperDir, os.Stdout)
			}); err != nil {
				printSubcommandError("diff", err)
				os.Exit(1)
			}
			os.Exit(0)
		}

		// Fallback to git diff
		if err := tr.Step("diff computation", func() error {
			return revert.RunDiff(cwd, os.Stdout)
		}); err != nil {
			printSubcommandError("diff", err)
			os.Exit(1)
		}

	case "revert":
		quiet := hasQuietFlag(args)
		tr := trace.New(!quiet)
		force := hasYesFlag(args)
		cwd, err := os.Getwd()
		if err != nil {
			printSubcommandError("revert", fmt.Errorf("failed to get working directory: %w", err))
			os.Exit(1)
		}

		// Check for active session (strict matching for destructive revert)
		var sess *revert.Session
		_ = tr.Step("session discovery", func() error {
			var sErr error
			sess, sErr = revert.MostRecentSession(cwd, true)
			return sErr
		})

		if sess != nil {
			if !force {
				fmt.Fprintf(os.Stdout, "%s Discard active overlay session changes? [y/N]: ", ui.StyleMeta.Render("PROMPT"))
				var response string
				if _, err := fmt.Fscanln(os.Stdin, &response); err != nil || (response != "y" && response != "yes" && response != "Y" && response != "YES") {
					fmt.Fprintf(os.Stdout, "%s\n", ui.StyleMeta.Render("Revert cancelled."))
					os.Exit(0)
				}
			}
			if err := tr.Step("discard session", func() error {
				return revert.DiscardSession(sess)
			}); err != nil {
				printSubcommandError("revert", fmt.Errorf("failed to discard session: %w", err))
				os.Exit(1)
			}
			fmt.Fprintf(os.Stdout, "%s\n", ui.StyleAllowed.Render("Overlay session discarded. Working directory remains unchanged."))
			os.Exit(0)
		}

		// Fallback to git revert
		if err := tr.Step("git revert", func() error {
			return revert.Revert(cwd, force, os.Stdin, os.Stdout)
		}); err != nil {
			if errors.Is(err, revert.ErrRevertCancelled) {
				os.Exit(0)
			}
			printSubcommandError("revert", err)
			os.Exit(1)
		}

	case "apply":
		quiet := hasQuietFlag(args)
		tr := trace.New(!quiet)
		shadowUpper := parseShadowFlag(args)
		cwd, err := os.Getwd()
		if err != nil {
			printSubcommandError("apply", fmt.Errorf("failed to get working directory: %w", err))
			os.Exit(1)
		}

		var sess *revert.Session
		if shadowUpper == "" {
			if err := tr.Step("session discovery", func() error {
				var sErr error
				sess, sErr = revert.MostRecentSession(cwd, true)
				return sErr
			}); err != nil {
				printSubcommandError("apply", err)
				os.Exit(1)
			}
			shadowUpper = sess.UpperDir
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

		if err := tr.Step("apply changes", func() error {
			return revert.ApplyShadowChanges(cwd, shadowUpper)
		}); err != nil {
			printSubcommandError("apply", err)
			os.Exit(1)
		}

		if sess != nil {
			_ = tr.Step("session cleanup", func() error {
				return revert.DiscardSession(sess)
			})
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
