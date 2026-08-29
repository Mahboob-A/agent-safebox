# Safebox

Safebox is an unprivileged, high-performance Linux sandbox and change-management tool designed to safely run untrusted developer scripts, third-party CLI tools, and autonomous AI coding agents.

---

## Key Features

- **Unprivileged Operation**: Requires zero root privileges (`sudo`) and zero setuid binaries. Operates entirely in user space via standard Linux namespaces and Landlock LSM.
- **Microsecond Latency**: Adds negligible startup overhead (typically under 40ms, well within the 50ms NFR3 budget).
- **Multi-Dimensional Kernel Isolation**:
  - **User & Mount Namespaces (`CLONE_NEWUSER`, `CLONE_NEWNS`)**: Remaps UID/GID so inside the sandbox you appear as root (`uid=0`), while on the host you remain an unprivileged user.
  - **Network Isolation (`CLONE_NEWNET`)**: Places execution in an isolated network namespace with an unconfigured loopback, blocking all outbound network access and data exfiltration.
  - **PID Namespace with Init Shim (`CLONE_NEWPID`)**: Safebox acts as a minimal PID 1 supervisor inside the container namespace, forwarding signals (such as SIGINT / Ctrl+C, SIGTERM, SIGHUP, SIGQUIT) to the wrapped process (PID 2) and reaping reparented zombie children with a `wait4(-1, ...)` loop.
  - **IPC & UTS Namespaces (`CLONE_NEWIPC`, `CLONE_NEWUTS`)**: Isolates inter-process communication resources, message queues, shared memory, and hostname.
- **Landlock LSM Filesystem Containment**:
  - **Deny-by-Default Security Policy**: Restricts filesystem access at the kernel level.
  - **Deliberately Narrow Default Allow List**: Read-write access to the working directory; read-only access to `/usr`, `/usr/local`, `/lib`, `/lib64`, and `/etc/ld.so.conf.d`; and strictly required configuration files from `/etc` (`/etc/passwd`, `/etc/group`, `/etc/localtime`, `/etc/ld.so.cache`, `/etc/ld.so.conf`, `/etc/nsswitch.conf`).
  - **Protected Paths**: Sensitive paths such as `/etc/shadow`, `/etc/ssh/*`, and user home directories (`~/.ssh`, `~/.aws`, `~/.gnupg`) are strictly denied by default.
  - **Repeatable `--allow-path=<dir>` Flag**: Explicitly grants read and execute access to specific directories outside the default allow list (for example, `/root/.local/bin` for custom agent tools).
  - **Repeatable `--allow-path-rw=<dir>` Flag**: Explicitly grants read and write access to specific directories (for example, agent state directories like `$HOME/.gemini` or `$HOME/.claude`).
  - **Actionable Remediation Hints**: Emits clear copy-pasteable hints on denial (for example, `-> hint: rerun with --allow-path=/path/to/dir`).
- **Policy Inspection Mode (`--probe`)**:
  - Inspects effective Landlock read-only and read-write allow-lists and resolves target binaries (`exec.LookPath`) without creating namespaces or executing commands, exiting cleanly with code 0.
- **In-Namespace OverlayFS & Automatic Session Tracking**:
  - **Zero-Footprint Host Protection**: Sandboxed writes are directed to an unprivileged OverlayFS upper layer mounted inside the private mount namespace. The host filesystem remains pristine until explicitly applied.
  - **Atomic Apply via Staging Directories**: `safebox apply` stages all additions and modifications in an isolated temporary staging directory before committing changes to the host, handling cross-filesystem copies (`EXDEV`) gracefully.
  - **Recursive Directory Whiteout Removal**: Deletes entire directory subtrees cleanly when OverlayFS emits directory whiteout character device nodes.
  - **Automatic Stateless Session Discovery**: `safebox diff`, `safebox apply`, and `safebox revert` automatically resolve the latest session for the current working directory.
  - **Positional Path Filtering**: `safebox diff [paths...]` filters change reports to specific files or subdirectories.
  - **Strict Directory Isolation**: Prevents destructive operations (`apply` and `revert`) from acting on parent sessions when invoked from subdirectories.
  - **Automatic Session Pruning**: Abandoned sessions older than 24 hours are automatically purged on new runs to prevent disk bloat.
- **Default-On Execution Tracing**:
  - Emits colored, timestamped setup and teardown step badges to `stderr` (`[safebox]` for parent steps, `[safebox:child]` for child namespace steps).
  - Stdout remains completely clean for pipes, redirects, and automated tools.
  - Suppressed with zero overhead using `--quiet` (`-q`).
- **Fail-Safe over Fail-Open**: Refuses to run if Landlock or required unprivileged namespace syscalls are unsupported on the host kernel.

---

## Architecture & Codebase Map

Safebox is structured into modular, single-responsibility Go packages:

| Package | Path | Responsibility |
| :--- | :--- | :--- |
| `main` | `safebox/main.go` | Thin entrypoint (<25 lines) checking arguments and dispatching to `internal/cli` |
| `cli` | `safebox/internal/cli/` | Subcommand runners (`run.go`, `child.go`, `diff.go`, `apply.go`, `revert.go`, `probe.go`, `help.go`, `hint.go`, `dispatch.go`) and flag parsing (`parser.go`) |
| `isolation` | `safebox/internal/isolation/` | Namespace clone setup (`namespace.go`), Landlock LSM ruleset (`landlock.go`), OverlayFS mounting (`overlay.go`), PID 1 init supervisor shim (`shim.go`), structured error types (`errors.go`) |
| `revert` | `safebox/internal/revert/` | OverlayFS session management (`session.go`), diff inspection (`diff.go`), atomic staging apply (`shadow_apply.go`), and git fallback (`git.go`) |
| `trace` | `safebox/internal/trace/` | Real-time step timing and formatted `stderr` trace emitters (`trace.go`) |
| `ui` | `safebox/internal/ui/` | Lipgloss color styling and visual token formatting (`styles.go`) |

---

## System Requirements & Ubuntu VM Setup

### Supported Environments
- **Operating System**: Ubuntu Linux 22.04 LTS, 24.04 LTS, or newer.
- **Kernel Requirements**:
  - **Linux 5.13+**: Required for Landlock LSM ABI v1+ (Ubuntu 22.04+ default kernels are 5.15+, 6.5+, or 6.8+).
  - **Linux 5.11+**: Required for unprivileged OverlayFS mounts inside user namespaces.
- **Architecture**: x86_64, aarch64 / arm64.

### Checking Kernel Capabilities
Verify kernel capabilities on your Ubuntu system:

```bash
# 1. Verify kernel version (must be >= 5.13)
uname -r

# 2. Verify Landlock LSM is enabled
cat /sys/kernel/security/lsm
# Expected output contains: landlock

# 3. Verify unprivileged user namespace support
cat /proc/sys/kernel/unprivileged_userns_clone
# Should output: 1
```

### Ubuntu 24.04 LTS User Namespace Configuration
Ubuntu 24.04 introduced AppArmor restrictions on unprivileged user namespaces. If unprivileged user namespace creation is restricted on your system, enable it via sysctl:

```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0

# To persist across system reboots:
echo "kernel.apparmor_restrict_unprivileged_userns = 0" | sudo tee /etc/sysctl.d/60-apparmor-userns.conf
```

---

## Installation & Build

### Prerequisites
- Go 1.22 or newer:
  ```bash
  sudo apt-get update && sudo apt-get install -y golang-go
  ```

### Build from Source
Safebox compiles into a single standalone static binary with zero external runtime dependencies:

```bash
# Clone the repository
git clone https://github.com/mahboob-a/agent-safebox.git
cd agent-safebox

# Compile the standalone binary
go build -o safebox .

# Verify the build
./safebox help
```

### System-Wide Installation
To install `safebox` globally:

```bash
sudo install -m 0755 safebox /usr/local/bin/safebox
safebox help
```

---

## Command-Line Interface (CLI) Reference

### Global Syntax
```bash
safebox <command> [arguments]
```

### Subcommand Matrix

| Command | Arguments / Flags | Description |
| :--- | :--- | :--- |
| `run` | `[--quiet \| -q] [--allow-path=<dir> ...] [--allow-path-rw=<dir> ...] [--probe] -- <cmd...>` | Execute a command inside unprivileged namespaces with OverlayFS and Landlock containment. The `--` delimiter is mandatory. |
| `diff` | `[--quiet \| -q] [paths...]` | Display colored status (`+ [ADDED]`, `~ [MODIFIED]`, `- [DELETED]`) for active session or git workspace (optionally filtered by positional paths). |
| `revert` | `[--quiet \| -q] [--yes \| -y]` | Discard active OverlayFS session changes or restore git working tree. Prompts for confirmation unless `--yes` is passed. |
| `apply` | `[--quiet \| -q] [--yes \| -y]` | Atomically apply sandbox session modifications back to the host working directory. Prompts for confirmation unless `--yes` is passed. |
| `help` | `-h`, `--help` | Display CLI usage documentation. |

---

## Common Use Cases

### 1. Running Untrusted Scripts Safely
Execute an untrusted build or test script inside the sandbox. Network calls and writes outside the working directory are blocked automatically:

```bash
safebox run -- python3 generate_assets.py
```

### 2. Autonomous AI Coding Agents (`agy`, Claude Code, Codex)
Run an autonomous coding agent with tool binary directories granted read+execute and agent state directories granted read+write:

```bash
# Execution without required permissions produces an actionable hint:
safebox run -- /root/.local/bin/agy --version
# ERROR safebox: exec failed: permission denied
#   -> hint: rerun with --allow-path=/root/.local/bin

# Grant read/exec access to the tool path and read/write to its state directory:
safebox run --allow-path=$HOME/.local/bin --allow-path-rw=$HOME/.gemini -- agy "implement user login"
```

### 3. Reviewing & Applying AI Agent Mutations
Inspect every file created or modified by an agent session before committing changes to the host:

```bash
# 1. Run agent task in sandbox (host remains untouched)
safebox run -- npm run build

# 2. Inspect session diff (optionally filtering by directory)
safebox diff src/

# 3. Apply changes atomically if verified, or discard cleanly
safebox apply --yes
# Or discard:
safebox revert --yes
```

### 4. Headless CI/CD & Automated Pipelines
Use `--quiet` (`-q`) to suppress setup traces and cleanly consume command stdout:

```bash
# Output contains only the wrapped command's stdout
VERSION=$(safebox run --quiet --allow-path=/root/.local/bin -- /root/.local/bin/agy --version)
echo "Agent version: $VERSION"
```

### 5. Pre-Flight Security Policy Probing (`--probe`)
Inspect effective Landlock allow-lists and resolved binaries without starting namespaces or executing commands:

```bash
safebox run --probe --allow-path=$HOME/.local/bin --allow-path-rw=$HOME/.gemini -- agy
```

---

## Running Automated Tests

Run the complete test suite and boundary verification scripts:

```bash
# 1. Run all unit and integration tests with race detector
go test -race -count=1 -v ./...

# 2. Run security boundary check suite
bash /root/go-safebox/workspace/.agents/skills/code-review/scripts/test-sandbox-boundaries.sh

# 3. Run repository rule validation
bash /root/go-safebox/workspace/.agents/skills/code-review/scripts/check-rules.sh
```

---

## License

This project is open source and available under the MIT License.
