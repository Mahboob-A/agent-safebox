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
  run     [--quiet|-q] [--allow-path=<dir> ...] [--allow-path-rw=<dir> ...]
          [--allow-file-rw=<file> ...] [--persistent-state=<host>:<mount> ...]
          [--allow-net] [--probe] -- <cmd> [args...]
          Execute <cmd> inside the sandbox. The '--' delimiter is required.
          --allow-path grants read+execute access to <dir>.
          --allow-path-rw grants read+write access to <dir>.
          --allow-file-rw grants read+write access to single file <file>.
          --persistent-state bind-mounts <host> at <mount> before Landlock lockdown.
          --allow-net grants full internet egress via userspace NAT
          (pasta or slirp4netns). Off by default. Domain-restricted mode
          is planned for v0.5; --allow-network=<domain> is no longer accepted.
          --probe prints the effective policy and exits without executing.

  diff    [--quiet|-q] [-p|--patch] [paths...]
          Show what changed in the active sandbox session (non-blocking).
          Use -p or --patch to display a unified line-by-line diff.

  cat     [-h|--help] <file...>
          Print the contents of a staged file from the active session (or host).

  revert  [--quiet|-q] [--yes|-y] [--force-discard]
          Discard the active sandbox session without applying changes.
          Refuses deletion if a safebox run is actively running (exit code 3).
          --force-discard overrides active session protection with a warning.

  apply   [--quiet|-q] [--yes|-y] [--force-discard]
          Apply sandbox session changes to the host working directory.
          If safebox run is active, changes are synced and session is kept.
          --force-discard applies changes and immediately cleans up session.

  profile [list|show <name>]
          Inspect registered agent profiles (built-in and custom user profiles).

  help    Show this help text.

Exit Codes:
  0: Success
  1: Command execution or general error
  2: Usage or argument parsing error
  3: Active session safety refusal on revert
  4: Network backend unavailable (install slirp4netns or pasta)
  5: Persistent state bind mount denied (NFR1 hard-fail)
  6: Active session collision in working directory
  7: Network namespace isolation verification failure

Where safebox stores state:
  Sessions:    $SAFEBOX_SESSION_ROOT (default: $TMPDIR/safebox/sessions)
  Persistent:  $XDG_STATE_HOME/safebox/agents/<tool>/ (default: ~/.local/share/safebox/agents/<tool>/)
  Profiles:    ~/.config/safebox/profiles/<tool>.toml

Running a coding agent:
  safebox includes built-in profiles for 16 major AI coding agents (agy, claude,
  codex, puku, cursor, kilo, aider, etc.) that automatically grant required
  state directory permissions, persistent state bind-mounts, and cloud LLM
  egress without manual flags:

    safebox run -- <agent> <args...>

  Custom user profiles can be placed in ~/.config/safebox/profiles/<name>.toml
  to override or extend built-in profiles.

Security Note & Threat Model:
  Built-in profiles match by argv[0] substring. If your PATH contains an untrusted
  binary whose name collides with a known agent, that binary will receive the agent's
  RW grants AND the agent's persistent-state bind-mount, exposing auth tokens.
  Use a full path to the trusted binary or remove the untrusted one from PATH.

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
