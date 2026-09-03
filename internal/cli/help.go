package cli

import (
	"fmt"
	"io"
	"os"
)

// UsageText is the complete help text for safebox.
const UsageText = `safebox <command> [arguments]

Unprivileged Linux sandbox and change-management engine for AI coding agents and developer commands.

Commands:
  run       Execute a command or agent in the isolated sandbox (requires '--', supports --allow-path-rw, --allow-file-rw, --allow-net, --probe)
  diff      Show uncommitted changes staged in the active session (-p for unified diff)
  cat       Stream contents of a staged file from the active session to stdout
  apply     Atomically commit staged sandbox changes to the host working directory
  revert    Discard staged sandbox changes and clean up the session
  profile   Inspect registered agent security profiles: profile [list|show <name>]
  version   Print Safebox version and build information
  help      Show this help message

Global Flags:
  -v, --version  Show version and build information
  -h, --help     Show this help message

Quick Start Examples:
  safebox run -- echo "Running inside sandbox"
  safebox run --allow-net --allow-path=~/.local/bin -- agy
  safebox diff -p
  safebox cat server.go | less
  safebox apply --yes
  safebox revert --yes

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
  Read the hint, copy-paste the flag into your command, retry.

Documentation:
  Command Reference:   https://github.com/Mahboob-A/agent-safebox/blob/main/COMMANDS.md
  System Architecture: https://github.com/Mahboob-A/agent-safebox/blob/main/ARCHITECTURE.md`

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
