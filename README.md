# Safebox

Safebox is an unprivileged, high-performance Linux sandbox and change-management tool designed to safely run untrusted developer scripts, third-party CLI tools, and AI agent workloads.

---

## Key Features

- **Unprivileged Operation**: Requires zero root privileges (`sudo`) and zero setuid binaries. Operates entirely in user space.
- **Microsecond Latency**: Adds negligible startup overhead (well under 250ms latency budget).
- **Multi-Dimensional Kernel Isolation**:
  - **User & Mount Namespaces (`CLONE_NEWUSER`, `CLONE_NEWNS`)**: Remaps UID/GID so inside the sandbox you appear as root (`uid=0`), while on the host you remain an unprivileged user.
  - **Network Isolation (`CLONE_NEWNET`)**: Places execution in an isolated network namespace with unconfigured loopback, blocking external traffic and data exfiltration.
  - **Landlock LSM Sandboxing**: Enforces strict kernel-level access controls, restricting filesystem write permissions strictly to the current working directory.
  - **PID, IPC, & UTS Namespaces**: Isolates processes, inter-process communication, and hostname.
- **Change Visibility & Revert**:
  - **Git Mode**: Formats working tree modifications with colored Lipgloss tokens and provides one-command instant rollback (`safebox revert`).
  - **Shadow Mode (OverlayFS)**: Provides unprivileged copy-on-write filesystem redirection for non-git workspaces, supporting change inspection (`safebox diff --shadow=<dir>`) and host synchronization (`safebox apply --shadow=<dir>`).

---

## System Requirements & Ubuntu VM Setup

### Supported Environments
- **Operating System**: Ubuntu Linux 22.04 LTS, 24.04 LTS, or newer.
- **Kernel Requirements**:
  - **Linux 5.13+**: Required for Landlock LSM ABI v1 (default in Ubuntu 22.04+).
  - **Linux 5.11+**: Required for unprivileged OverlayFS mounts.
- **Architecture**: x86_64, aarch64 / arm64.

### Checking Kernel Capabilities
Run the following commands in your Ubuntu terminal:

```bash
# 1. Verify kernel version (must be >= 5.13)
uname -r

# 2. Verify Landlock LSM is enabled in kernel
cat /sys/kernel/security/lsm
# Expected output contains: landlock

# 3. Verify unprivileged user namespace support
cat /proc/sys/kernel/unprivileged_userns_clone
# Should output: 1
```

### Ubuntu 24.04 LTS User Namespace Configuration (If Applicable)
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
Safebox has zero C-library dependencies and compiles to a single standalone binary:

```bash
# Clone the repository
git clone https://github.com/your-org/safebox.git
cd safebox

# Compile the standalone binary
go build -o safebox .

# Verify the build
./safebox help
```

### System-Wide Installation
To make `safebox` globally accessible across your system:

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
| `run` | `[--] <cmd...>` | Execute command inside namespace and Landlock sandbox |
| `diff` | `[--shadow=<dir>]` | Display colored status of modified, added, and deleted files |
| `revert` | `[--yes \| -y \| --yes=true]` | Discard all working tree changes and restore clean state |
| `apply` | `--shadow=<dir> [--yes \| -y]` | Synchronize shadow OverlayFS changes to working directory |
| `help` | `-h`, `--help` | Display CLI usage documentation |

---

### Command Breakdown

#### 1. `safebox run [--] <cmd...>`
Executes any Linux binary or script in an unprivileged, Landlock-restricted sandbox.

- **Double-Dash (`--`)**: Optional argument separator to ensure flags intended for the target command are not intercepted by safebox.
- **Filesystem Permissions**:
  - Current Working Directory (`CWD`): Read and write allowed.
  - System Paths (`/usr`, `/lib`, `/bin`, etc.): Read-only allowed.
  - Sensitive Paths (`/root`, `/home/other`, `/etc` writes): Denied by Landlock.
- **Network Access**: Denied (outbound TCP/UDP drops).
- **Exit Code Propagation**: Propagates the exact integer exit code of the target program ($0, 1, 2, 127$, etc.).

**Examples**:
```bash
# Run a Python script inside sandbox
safebox run python3 test_script.py

# Run a shell command with arguments using --
safebox run -- sh -c "echo 'Hello Sandbox' > test.txt"

# Run a build tool
safebox run make build
```

---

#### 2. `safebox diff [--shadow=<dir>]`
Inspects and formats changes in human-readable colored output.

- **Git Mode (default)**: When executed in a git repository, inspects unstaged and staged modifications relative to `HEAD`.
- **Shadow Mode (`--shadow=<upperdir>`)**: When `--shadow=<dir>` is specified, compares the OverlayFS upper layer with the host working directory.
- **Output Status Tokens**:
  - `+ [ADDED] <path>` (Green): New untracked or newly created file.
  - `~ [MODIFIED] <path>` (Yellow): Existing file whose contents or mode changed.
  - `- [DELETED] <path>` (Red): Deleted file (including OverlayFS whiteouts).
  - `? [UNTRACKED] <path>` (Blue): Untracked files in git mode.

**Examples**:
```bash
# Inspect changes in a git repository
safebox diff

# Inspect changes in a non-git directory from a shadow session
safebox diff --shadow=/tmp/safebox-session-123/upper
```

---

#### 3. `safebox revert [--yes|-y]`
Discards uncommitted changes in a git repository, returning the working tree to pristine state.

- **Interactive Confirmation**: Prompts the user before discarding:
  `[PROMPT] Discard all working tree changes? [y/N]: `
- **Force Flags (`--yes`, `-y`, `--yes=true`)**: Skips the confirmation prompt for automated scripts and CI pipelines.

**Examples**:
```bash
# Revert with interactive confirmation
safebox revert

# Revert immediately without prompt
safebox revert --yes
safebox revert -y
```

---

#### 4. `safebox apply --shadow=<dir> [--yes|-y]`
Applies modifications captured in an OverlayFS shadow directory to the current working directory.

- **Flag `--shadow=<dir>` (Required)**: Path to the OverlayFS `upperdir` containing session changes.
- **Interactive Confirmation**: Prompts before applying:
  `[PROMPT] Apply shadow changes to working directory? [y/N]: `
- **Force Flags (`--yes`, `-y`)**: Bypasses the prompt to apply changes immediately.
- **Behavior**:
  - Copies added and modified files to the host directory (preserving file modes).
  - Creates necessary parent directories.
  - Removes files from the host directory that were deleted (marked with whiteout devices) in the shadow session.

**Examples**:
```bash
# Apply shadow modifications with confirmation
safebox apply --shadow=/tmp/session-42/upper

# Apply shadow modifications non-interactively
safebox apply --shadow=/tmp/session-42/upper --yes
```

---

## Step-by-Step Usage Tutorials

### Tutorial 1: Safe Package Installation & Inspection
When testing an untrusted package or script:

```bash
cd /path/to/my-project

# 1. Run installation inside safebox
safebox run -- npm install untrusted-package

# 2. Inspect what files were modified or created
safebox diff

# 3. If unwanted modifications are detected, rollback immediately
safebox revert --yes
```

---

### Tutorial 2: AI Coding Agent Sandbox & Change Review
When an automated coding agent performs file edits and builds:

```bash
# 1. Agent runs tests and code generators
safebox run -- pytest tests/

# 2. Developer inspects the exact diff
safebox diff

# 3. If approved, keep changes; if rejected, rollback
safebox revert -y
```

---

### Tutorial 3: Non-Git OverlayFS Shadow Workspace
For non-git workspaces or zero-risk ephemeral execution:

```bash
# 1. Create session directories
SESSION_DIR=$(mktemp -d /tmp/safebox-session.XXXXXX)
mkdir -p "$SESSION_DIR/upper" "$SESSION_DIR/work" "$SESSION_DIR/target"

# 2. Inside overlay session, make modifications
# (all writes are captured in $SESSION_DIR/upper without touching lowerdir)

# 3. Inspect shadow changes from host
safebox diff --shadow="$SESSION_DIR/upper"

# 4. Apply changes to host or discard session
safebox apply --shadow="$SESSION_DIR/upper" --yes
# Or discard:
rm -rf "$SESSION_DIR"
```

---

## Troubleshooting & Diagnostics

- **`permission denied` outside working directory**:
  - *Cause*: Command attempted to modify files outside `CWD` (e.g. `/etc`, `/usr`, `/root`).
  - *Behavior*: Intended security confinement enforced by Landlock LSM.
- **`network is unreachable`**:
  - *Cause*: Command attempted outbound network communication.
  - *Behavior*: Intended network containment enforced by `CLONE_NEWNET`. Pre-download required packages before sandboxed execution.
- **`shadow directory ... does not exist`**:
  - *Cause*: The path passed to `--shadow=<dir>` was not found on disk. Verify that the directory path exists.

---

## License

This project is licensed under the Apache License 2.0.
