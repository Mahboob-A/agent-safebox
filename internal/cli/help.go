package cli

import (
	"fmt"
	"io"
	"os"
)

// UsageText is the complete help text for safebox.
const UsageText = `safebox <command> [arguments]

Run an untrusted command or AI coding agent inside an unprivileged Linux
sandbox with Landlock filesystem containment and OverlayFS change capture.

Commands:
  run    [--quiet|-q] [--allow-path=<dir> ...] [--allow-path-rw=<dir> ...]
         [--probe] -- <cmd> [args...]
         Execute <cmd> inside the sandbox. The '--' delimiter is required.
         --allow-path grants read+execute access to <dir>.
         --allow-path-rw grants read+write access to <dir>.
         --probe prints the effective policy and exits without executing.

  diff   [--quiet|-q] [paths...]
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

// PrintUsage writes the usage guide to os.Stderr.
func PrintUsage() {
	PrintUsageTo(os.Stderr)
}

// PrintUsageTo writes the usage guide to the provided io.Writer.
func PrintUsageTo(w io.Writer) {
	fmt.Fprintln(w, UsageText)
}

// RunHelp writes the usage guide to w and returns 0.
func RunHelp(w io.Writer) int {
	PrintUsageTo(w)
	return 0
}
