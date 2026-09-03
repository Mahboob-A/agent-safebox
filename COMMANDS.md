# Safebox Command Reference

This document is the authoritative command reference for Safebox. It documents every subcommand, all supported flags, execution options, and realistic terminal outputs.

---

## Command Quick Reference

| Command | Primary Function | Common Options |
| :--- | :--- | :--- |
| [`safebox run`](#1-safebox-run) | Executes commands or agents in an isolated Linux sandbox | `--allow-path`, `--allow-net`, `--probe`, `--quiet` |
| [`safebox diff`](#2-safebox-diff) | Inspects uncommitted changes staged in the active session | `-p` / `--patch`, `--quiet`, `[paths...]` |
| [`safebox cat`](#3-safebox-cat) | Streams staged file contents directly to standard output | `-q` / `--quiet`, `[paths...]` |
| [`safebox apply`](#4-safebox-apply) | Atomically commits staged sandbox changes to host workspace | `-y` / `--yes`, `--force-discard` |
| [`safebox revert`](#5-safebox-revert) | Discards staged changes and tears down active session | `-y` / `--yes`, `--force-discard` |
| [`safebox profile`](#6-safebox-profile) | Inspects built-in and user-defined agent security profiles | `list`, `show <name>` |
| [`safebox help`](#7-safebox-help) | Displays global usage summary and flag documentation | `-h`, `--help` |

---

## 1. safebox run

### Synopsis
```bash
safebox run [options] -- <command> [args...]
```
The `--` delimiter is strictly required to separate Safebox flags from the command and arguments being sandboxed.

### Options
| Flag | Type | Description |
| :--- | :--- | :--- |
| `--` | Delimiter | **Mandatory.** Separates Safebox options from target command. |
| `-q`, `--quiet` | Boolean | Suppresses timing trace badges (`[safebox]`) on standard error. |
| `--allow-path=<dir>` | Path | Grants read and execute access to `<dir>` (e.g., `~/.local/bin`). |
| `--allow-path-rw=<dir>` | Path | Grants read and write access to `<dir>` inside Landlock. |
| `--allow-file-rw=<file>` | Path | Grants read and write access to a single specific `<file>`. |
| `--persistent-state=<host>:<mount>` | Mapping | Bind-mounts `<host>` directory to `<mount>` path before Landlock lockdown. |
| `--allow-net` | Boolean | Enables outbound internet egress via userspace NAT (`slirp4netns` or `pasta`). |
| `--probe` | Boolean | Prints effective Landlock allowlists and exits without executing. |

---

### Examples

#### Example 1.1: Basic Sandboxed Execution
Run a simple command inside unprivileged Linux user, mount, PID, IPC, and UTS namespaces:
```bash
safebox run -- echo "Hello from Safebox!"
```
**Output**:
```text
  -> no profile found for echo
[safebox] profile resolution           ok  18.24µs
[safebox] session initialize           ok  22.145892ms
[safebox] wrapped command spawn        ok  3.145021ms
[safebox:child] procfs mount                 ok  42.115µs
[safebox:child] overlayfs mount              ok  412.339µs
[safebox:child] landlock restrict            ok  6.812451ms
[safebox:child] exec handoff                 ok  815.112µs
Hello from Safebox!
```

#### Example 1.2: Pre-Flight Policy Probing (`--probe`)
Inspect effective read-only, read-write, and file allowlists without executing the command:
```bash
safebox run --probe --allow-net --allow-path=/root/.local/bin -- agy
```
**Output**:
```text
  -> using profile: agy
LANDLOCK PROBE REPORT:
  Working Dir: /root/go-safebox/workspace/agent-safebox
  Effective RW Paths:
    - /root/go-safebox/workspace/agent-safebox
    - /root/.gemini
  Effective RO Paths:
    - /usr
    - /usr/local
    - /lib
    - /lib64
    - /etc/ld.so.conf.d
    - /etc/ssl
    - /etc/pki
    - /root/.local/bin
  Effective RW Files:
    - /dev/null
    - /dev/zero
  Effective RO Files:
    - /dev/urandom
    - /dev/random
    - /etc/ld.so.cache
    - /etc/ld.so.conf
    - /etc/nsswitch.conf
    - /etc/passwd
    - /etc/group
    - /etc/localtime
```

#### Example 1.3: Autonomous AI Coding Agent Execution
Launch an AI coding agent with userspace internet access and user-installed binary path:
```bash
safebox run --allow-net --allow-path=~/.local/bin -- agy -p "create healthcheck endpoint"
```
**Output**:
```text
  -> using profile: agy
[safebox] profile resolution           ok  14.22µs
[safebox] session initialize           ok  26.812901ms
[safebox] wrapped command spawn        ok  2.812401ms
[safebox] egress setup                 ok  slirp4netns
[safebox:child] netpolicy apply              ok  280.112µs
[safebox:child] procfs mount                 ok  52.181µs
[safebox:child] overlayfs mount              ok  380.412µs
[safebox:child] persistent state mount       ok  65.119µs
[safebox:child] landlock restrict            ok  7.112091ms
[safebox:child] exec handoff                 ok  620.182µs
Created healthcheck.go with GET /health returning 200 OK.
```

#### Example 1.4: Testing an Untrusted Repository
Safely compile and test untrusted code without risking host credentials or system files:
```bash
cd /tmp && mkdir -p untrusted-repo && cd untrusted-repo
git clone --depth 1 https://github.com/codeplea/tinyexpr.git .
safebox run -- make smoke
```
**Output**:
```text
  -> no profile found for make
[safebox] profile resolution           ok  19.12µs
[safebox] session initialize           ok  24.112901ms
[safebox] wrapped command spawn        ok  2.912401ms
[safebox:child] procfs mount                 ok  45.181µs
[safebox:child] overlayfs mount              ok  320.412µs
[safebox:child] landlock restrict            ok  6.812091ms
[safebox:child] exec handoff                 ok  712.182µs
gcc -Wall -Wshadow -O2 -o smoke smoke.c tinyexpr.c -lm
./smoke
ALL TESTS PASSED (10080/10080)
```

---

## 2. safebox diff

### Synopsis
```bash
safebox diff [options] [paths...]
```

### Options
| Flag | Type | Description |
| :--- | :--- | :--- |
| `-p`, `--patch` | Boolean | Generates a Git-compatible unified line-by-line diff. |
| `-q`, `--quiet` | Boolean | Suppresses step timing badges on standard error. |
| `[paths...]` | Positional | Optional list of paths or subtrees to filter diff output. |

---

### Examples

#### Example 2.1: High-Level Status Summary
Inspect staged additions, modifications, and deletions from the host terminal:
```bash
safebox diff
```
**Output**:
```text
[safebox] session discovery            ok  812.441µs
+ [ADDED] healthcheck.go
~ [MODIFIED] main.go
- [DELETED] deprecated.go
[safebox] diff computation             ok  62.112µs
```

#### Example 2.2: Line-by-Line Unified Diff (`-p`)
View the exact code changes staged inside the sandbox:
```bash
safebox diff -p
```
**Output**:
```diff
[safebox] session discovery            ok  945.112µs
diff --git a/healthcheck.go b/healthcheck.go
new file mode 100644
--- /dev/null
+++ b/healthcheck.go
@@ -0,0 +1,7 @@
+package main
+
+import "net/http"
+
+func HealthHandler(w http.ResponseWriter, r *http.Request) {
+    w.WriteHeader(http.StatusOK)
+}
[safebox] diff computation             ok  185.221µs
```

#### Example 2.3: Path-Filtered Unified Diff
Inspect modifications restricted to a specific file or subdirectory:
```bash
safebox diff -p healthcheck.go
```
**Output**:
```diff
[safebox] session discovery            ok  820.119µs
diff --git a/healthcheck.go b/healthcheck.go
new file mode 100644
--- /dev/null
+++ b/healthcheck.go
@@ -0,0 +1,7 @@
+package main
+
+import "net/http"
+
+func HealthHandler(w http.ResponseWriter, r *http.Request) {
+    w.WriteHeader(http.StatusOK)
+}
[safebox] diff computation             ok  95.412µs
```

#### Example 2.4: Clean Working Tree Notification
When no uncommitted modifications exist:
```bash
safebox diff
```
**Output**:
```text
[safebox] session discovery            ok  780.112µs
Working tree is clean. No changes detected.
[safebox] diff computation             ok  41.115µs
```

---

## 3. safebox cat

### Synopsis
```bash
safebox cat [options] <file...>
```

### Options
| Flag | Type | Description |
| :--- | :--- | :--- |
| `-q`, `--quiet` | Boolean | Suppresses stderr trace badges for clean piping or redirects. |
| `-h`, `--help` | Boolean | Displays help text and usage examples. |
| `<file...>` | Positional | One or more file paths to stream to standard output. |

---

### Examples

#### Example 3.1: Inspecting a Staged File
Display the raw content of a file created inside the sandbox:
```bash
safebox cat healthcheck.go
```
**Output**:
```go
[safebox] session discovery            ok  812.112µs
package main

import "net/http"

func HealthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}
```

#### Example 3.2: Clean Piping and Redirection (`-q`)
Pipe file contents directly into a pager or redirect to a host file without stderr badges:
```bash
safebox cat -q healthcheck.go > local-healthcheck.go
cat local-healthcheck.go
```
**Output**:
```go
package main

import "net/http"

func HealthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}
```

#### Example 3.3: Inspecting Unmodified Host File
If a file has not been modified in the sandbox, `safebox cat` transparently falls back to the host file:
```bash
safebox cat go.mod
```
**Output**:
```text
[safebox] session discovery            ok  840.112µs
module safebox

go 1.24.0
```

#### Example 3.4: Deleted File Detection
Attempting to inspect a file that was deleted inside the active session reports a warning:
```bash
safebox cat deprecated.go
```
**Output**:
```text
[safebox] session discovery            ok  821.112µs
ERROR safebox cat: file "deprecated.go" was deleted in active session
```

---

## 4. safebox apply

### Synopsis
```bash
safebox apply [options]
```

### Options
| Flag | Type | Description |
| :--- | :--- | :--- |
| `-y`, `--yes` | Boolean | Confirms application automatically without interactive prompt. |
| `-q`, `--quiet` | Boolean | Suppresses trace step timing on standard error. |
| `--force-discard` | Boolean | Commits changes and immediately tears down active session lock. |

---

### Examples

#### Example 4.1: Non-Interactive Application
Atomically commit all staged additions, modifications, and deletions to the host directory:
```bash
safebox apply --yes
```
**Output**:
```text
[safebox] session discovery            ok  1.211892ms
[safebox] apply changes                ok  3.812401ms
[safebox] session cleanup              ok  512.112µs
Shadow changes applied to working directory.
```

#### Example 4.2: Applying While Agent Session is Active
If an agent process is still running, `safebox apply` syncs staged changes while keeping the session active:
```bash
safebox apply --yes
```
**Output**:
```text
[safebox] session discovery            ok  1.112091ms
[safebox] apply changes                ok  3.412891ms
Shadow changes applied to working directory (session kept active).
```

#### Example 4.3: Forced Cleanup with Active Session Lock
Force immediate cleanup even if a background process holds the session lock:
```bash
safebox apply --yes --force-discard
```
**Output**:
```text
[safebox] session discovery            ok  1.012891ms
WARNING: Forced session cleanup while safebox run PID 219832 was active.
[safebox] apply changes                ok  3.912401ms
[safebox] session cleanup              ok  612.112µs
Shadow changes applied to working directory.
```

---

## 5. safebox revert

### Synopsis
```bash
safebox revert [options]
```

### Options
| Flag | Type | Description |
| :--- | :--- | :--- |
| `-y`, `--yes` | Boolean | Confirms discard automatically without interactive prompt. |
| `-q`, `--quiet` | Boolean | Suppresses trace step timing on standard error. |
| `--force-discard` | Boolean | Forces session discard even if a running process holds the lock. |

---

### Examples

#### Example 5.1: Clean Discard of Staged Changes
Discard all uncommitted files and leave the host filesystem untouched:
```bash
safebox revert --yes
```
**Output**:
```text
[safebox] session discovery            ok  1.121891ms
[safebox] discard session              ok  412.339µs
Overlay session discarded. Working directory remains unchanged.
```

#### Example 5.2: Active Session Safety Refusal (Exit Code 3)
Safebox refuses to discard a session while a sandboxed process is actively running:
```bash
safebox revert --yes
```
**Output**:
```text
[safebox] session discovery            DENIED  1.211892ms
[safebox] active lock check            DENIED  420.112µs
ERROR safebox revert: cannot revert active session (safebox run PID 219832 is running). Terminate the running process before reverting, or use 'safebox apply' to capture changes.
```

#### Example 5.3: Forced Discard Overriding Lockfile
Override active session protection when a previous session was abandoned or hung:
```bash
safebox revert --yes --force-discard
```
**Output**:
```text
[safebox] session discovery            ok  1.012401ms
WARNING: Forced session revert while safebox run PID 219832 was active.
[safebox] discard session              ok  512.441µs
Overlay session discarded. Working directory remains unchanged.
```

---

## 6. safebox profile

### Synopsis
```bash
safebox profile [list | show <name>]
```

### Options & Subcommands
| Subcommand | Description |
| :--- | :--- |
| `list` | Lists all 16 built-in profiles and discovered custom user profiles. |
| `show <name>` | Displays TOML permission configuration for agent `<name>`. |

---

### Examples

#### Example 6.1: Listing Registered Profiles
```bash
safebox profile list
```
**Output**:
```text
BUILT-IN PROFILES:
  agy        rw=/root/.gemini
  aider      rw=/root/.config/aider,/root/.local/share/aider,/root/.cache/aider
  amp        rw=/root/.config/amp,/root/.local/share/amp,/root/.cache/amp
  claude     rw=/root/.claude  rwf=/root/.claude.json
  cline      rw=/root/.config/cline,/root/.local/share/cline,/root/.cache/cline
  codex      rw=/root/.codex
  continue   rw=/root/.config/continue,/root/.local/share/continue,/root/.cache/continue
  cursor     rw=/root/.cursor,/root/.config/Cursor
  gemini     rw=/root/.gemini
  goose      rw=/root/.config/goose,/root/.local/share/goose,/root/.cache/goose
  kilo       rw=/root/.config/kilo,/root/.local/share/kilo,/root/.cache/kilo
  mentat     rw=/root/.config/mentat,/root/.local/share/mentat,/root/.cache/mentat
  opencode   rw=/root/.config/opencode,/root/.local/share/opencode,/root/.cache/opencode
  pi         rw=/root/.config/pi,/root/.local/share/pi,/root/.cache/pi
  plandex    rw=/root/.config/plandex,/root/.local/share/plandex,/root/.cache/plandex
  puku       rw=/root/.puku-cli

USER PROFILES:
  (none)
```

#### Example 6.2: Inspecting an Agent Profile
```bash
safebox profile show agy
```
**Output**:
```toml
[binary]
name = "agy"

[paths]
allow_rw = ["$HOME/.gemini"]

[persistent_state]
host_dir = "$XDG_STATE_HOME/safebox/agents/agy"
mount_at = "$HOME/.gemini"

[network]
allow_net = true
```

#### Example 6.3: Unknown Profile Error
```bash
safebox profile show nonexistent-tool
```
**Output**:
```text
ERROR safebox: unknown profile "nonexistent-tool"
```

---

## 7. safebox help

### Synopsis
```bash
safebox help
safebox -h
safebox --help
```

Displays global command usage, flag definitions, exit codes, and agent security notes.

---

## 8. Exit Code Reference

Safebox uses distinct, deterministic exit status codes:

| Exit Code | Meaning | Remediation |
| :---: | :--- | :--- |
| `0` | Success | Operation completed cleanly. |
| `1` | Command execution or general error | Check error message on standard error. |
| `2` | Usage or argument parsing error | Verify command syntax and ensure `--` delimiter was used for `run`. |
| `3` | Active session safety refusal | Wait for running process to finish, or pass `--force-discard` to override. |
| `4` | Network backend unavailable | Install `slirp4netns` or `pasta` on your host. |
| `5` | Persistent state mount failure | Ensure directory permissions under `$XDG_STATE_HOME` are accessible (`0700`). |
| `6` | Active session collision in directory | Another session is running in this directory; wait or apply changes. |
| `7` | Network namespace isolation failure | Kernel failed to verify loopback isolation in child namespace. |
