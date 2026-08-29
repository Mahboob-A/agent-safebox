package main

import (
	"errors"
	"fmt"
	"io"
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

	binPath := ""
	if len(cmdArgs) > 0 {
		binPath = cmdArgs[0]
	}

	var landlockErr *isolation.ErrLandlockDenied
	if errors.As(err, &landlockErr) {
		return landlockErr.Hint(binPath)
	}

	var execDeniedErr *isolation.ErrExecDenied
	if errors.As(err, &execDeniedErr) {
		return execDeniedErr.Hint()
	}

	var notFoundErr *isolation.ErrExecNotFound
	if errors.As(err, &notFoundErr) {
		return notFoundErr.Hint()
	}

	if errors.Is(err, syscall.EACCES) {
		if binPath != "" && strings.Contains(binPath, "/") {
			return fmt.Sprintf("rerun with --allow-path=%s", filepath.Dir(binPath))
		}
		return "rerun with --allow-path=<dir>"
	}

	return ""
}

func hintForSubcommand(subcommand string, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, revert.ErrNoSessionFound) {
		return "run 'safebox run' first to create an overlay session"
	}
	if errors.Is(err, revert.ErrNotGitRepo) {
		return "run inside a git repository or use 'safebox run' to create an overlay session"
	}
	return ""
}

func printSubcommandError(subcommand string, err error) {
	fmt.Fprintf(os.Stderr, "%s safebox %s: %v\n", ui.StyleDenied.Render("ERROR"), subcommand, err)
	if hint := hintForSubcommand(subcommand, err); hint != "" {
		fmt.Fprintf(os.Stderr, "  -> hint: %s\n", hint)
	}
}

func hasShadowFlag(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--shadow=") || arg == "--shadow" {
			return true
		}
	}
	return false
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

func parseAllowPathsAndFlags(args []string, requireDoubleDash bool) (allowPathsRO []string, allowPathsRW []string, sessionDir string, quiet bool, cmdArgs []string, err error) {
	seenDoubleDash := false
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			seenDoubleDash = true
			for _, rest := range args[i+1:] {
				if strings.HasPrefix(rest, "--allow-path=") || rest == "--allow-path" ||
					strings.HasPrefix(rest, "--allow-path-rw=") || rest == "--allow-path-rw" {
					return nil, nil, "", false, nil, errors.New("--allow-path must precede the -- delimiter; current invocation places it inside the wrapped command arguments")
				}
			}
			cmdArgs = append(cmdArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--allow-path=") {
			pathVal := strings.TrimPrefix(arg, "--allow-path=")
			if pathVal != "" {
				allowPathsRO = append(allowPathsRO, pathVal)
			}
			i++
			continue
		}
		if arg == "--allow-path" && i+1 < len(args) {
			allowPathsRO = append(allowPathsRO, args[i+1])
			i += 2
			continue
		}
		if strings.HasPrefix(arg, "--allow-path-rw=") {
			pathVal := strings.TrimPrefix(arg, "--allow-path-rw=")
			if pathVal != "" {
				allowPathsRW = append(allowPathsRW, pathVal)
			}
			i++
			continue
		}
		if arg == "--allow-path-rw" && i+1 < len(args) {
			allowPathsRW = append(allowPathsRW, args[i+1])
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
			return nil, nil, "", false, nil, errors.New("safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)")
		}
		cmdArgs = append(cmdArgs, args[i:]...)
		break
	}
	if requireDoubleDash && !seenDoubleDash {
		return nil, nil, "", false, nil, errors.New("safebox: 'run' requires '--' before the wrapped command (e.g. safebox run -- <cmd>)")
	}
	return allowPathsRO, allowPathsRW, sessionDir, quiet, cmdArgs, nil
}

const usageText = `safebox <command> [arguments]

Run an untrusted command or AI coding agent inside an unprivileged Linux
sandbox with Landlock filesystem containment and OverlayFS change capture.

Commands:
  run    [--quiet|-q] [--allow-path=<dir> ...] [--allow-path-rw=<dir> ...]
         [--probe] -- <cmd> [args...]
         Execute <cmd> inside the sandbox. The '--' delimiter is required.
         --allow-path grants read+execute access to <dir>.
         --allow-path-rw grants read+write access to <dir>.
         --probe prints the effective policy and exits without executing.

  diff   [--quiet|-q]
         Show what changed in the active sandbox session.

  revert [--quiet|-q] [--yes|-y]
         Discard the active sandbox session without applying changes.

  apply  [--quiet|-q] [--yes|-y]
         Apply sandbox session changes to the host working directory.

  help   Show this help text.

Where safebox stores state:
  Sessions:    $SAFEBOX_SESSION_ROOT (default: $TMPDIR/safebox/sessions)
  Persistent:  v0.4: per-agent state under ~/.local/share/safebox/agents/<tool>/
  Profiles:    v0.4: ~/.config/safebox/profiles/<tool>.toml

Running a coding agent:
  Every agent stores its config, logs, and identity under a directory in
  your home folder. To run the agent, grant safebox access to its
  binary (read+execute) and its state dir (read+write):

    safebox run --allow-path=<binary_dir> --allow-path-rw=<state_dir> -- <bin> ...

  Examples (use the paths that match your install):

    safebox run --allow-path=/usr/local/bin --allow-path-rw=$HOME/.claude -- claude "task"
    safebox run --allow-path=$HOME/.local/bin --allow-path-rw=$HOME/.gemini -- agy "task"
    safebox run --allow-path=$HOME/.local/bin --allow-path-rw=$HOME/.codex -- codex "task"
    safebox run --allow-path=$HOME/.local/bin --allow-path-rw=$HOME/.puku-cli -- puku "task"

  In v0.4, safebox will auto-detect these paths from built-in profiles so
  you only need: safebox run -- <bin> "task".

On permission denial:
  safebox prints the exact --allow-path or --allow-path-rw flag needed.
  Read the hint, copy-paste the flag into your command, retry.`

func printUsage() {
	fmt.Fprintln(os.Stderr, usageText)
}

func printUsageTo(w io.Writer) {
	fmt.Fprintln(w, usageText)
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
		allowPathsRO, allowPathsRW, _, quiet, cmdArgs, err := parseAllowPathsAndFlags(args, true)
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
			return isolation.ReexecChild(allowPathsRO, allowPathsRW, sess.BaseDir, quiet, cmdArgs)
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
		allowPathsRO, allowPathsRW, sessionDir, quiet, cmdArgs, err := parseAllowPathsAndFlags(args, false)
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
			return isolation.ApplyLandlock(allowPathsRO, allowPathsRW)
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
		if hasShadowFlag(args) {
			fmt.Fprintf(os.Stderr, "%s safebox diff: unknown flag '--shadow'\n\n", ui.StyleDenied.Render("ERROR"))
			printUsage()
			os.Exit(2)
		}
		quiet := hasQuietFlag(args)
		tr := trace.New(!quiet)
		cwd, err := os.Getwd()
		if err != nil {
			printSubcommandError("diff", fmt.Errorf("failed to get working directory: %w", err))
			os.Exit(1)
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
		if hasShadowFlag(args) {
			fmt.Fprintf(os.Stderr, "%s safebox apply: unknown flag '--shadow'\n\n", ui.StyleDenied.Render("ERROR"))
			printUsage()
			os.Exit(2)
		}
		quiet := hasQuietFlag(args)
		tr := trace.New(!quiet)
		cwd, err := os.Getwd()
		if err != nil {
			printSubcommandError("apply", fmt.Errorf("failed to get working directory: %w", err))
			os.Exit(1)
		}

		var sess *revert.Session
		if err := tr.Step("session discovery", func() error {
			var sErr error
			sess, sErr = revert.MostRecentSession(cwd, true)
			return sErr
		}); err != nil {
			printSubcommandError("apply", err)
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
			return revert.ApplyShadowChanges(cwd, sess.UpperDir)
		}); err != nil {
			printSubcommandError("apply", err)
			os.Exit(1)
		}

		_ = tr.Step("session cleanup", func() error {
			return revert.DiscardSession(sess)
		})
		fmt.Fprintf(os.Stdout, "%s\n", ui.StyleAllowed.Render("Shadow changes applied to working directory."))

	case "help", "-h", "--help":
		printUsageTo(os.Stdout)
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "%s safebox: unknown command %q\n\n", ui.StyleDenied.Render("ERROR"), subcommand)
		printUsage()
		os.Exit(1)
	}
}
