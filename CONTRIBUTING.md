# Contributing to Safebox

Thank you for your interest in contributing to Safebox. Safebox is an unprivileged Linux sandbox and change-management engine built for autonomous AI agents and developer workflows.

We welcome contributions from fellow open-source developers. To maintain safety guarantees and architectural clarity, we adhere to strict engineering standards.

---

## 1. Core Principles & Philosophy

- **Fail-Safe over Fail-Open**: Security failures (such as Landlock activation errors, missing namespaces, or permission faults) are fatal errors. We never downgrade security failures to warnings.
- **Microsecond Latency**: The sandbox must add negligible startup latency (<30ms). Every millisecond counts.
- **Zero Daemon, Zero Root**: Safebox runs entirely in user space. No daemons, no background services, no setuid binaries, and no `sudo` requirements.
- **Strictly No AI Documentation Dumps in Core Repository**: The core `safebox` repository is reserved strictly for Go source code, unit/integration tests, and human-crafted documentation. Do not commit raw AI conversational transcripts, verbose AI-generated progress logs, or unedited prompt outputs into the core repository. Contributors are welcome to maintain their own AI logs in personal workspaces or external repositories.

---

## 2. Development Setup

### Prerequisites
- **Linux Kernel**: Linux 5.13 or newer (required for Landlock LSM ABI v1+).
- **Go**: Version 1.22 or newer.
- **Optional Network Backends**: `passt` or `slirp4netns` for testing userspace NAT egress (`--allow-net`).

### Cloning and Building
```bash
# Clone the repository
git clone https://github.com/Mahboob-A/agent-safebox.git
cd agent-safebox

# Build the binary locally
go build -o safebox .

# Verify the executable
./safebox help
```

---

## 3. Four-Stage Verification Battery

Before opening a pull request, your code must pass the complete four-stage test battery:

### Stage 1: Unit Tests with Race Detection
```bash
go test -race -count=1 -v ./...
```
All unit tests across all nine packages must pass with zero race conditions.

### Stage 2: Integration E2E Tests
```bash
RUN_INTEGRATION=1 go test -v ./internal/cli/...
```
Validates userspace NAT networking, DNS resolution, and non-mutation of host configuration.

### Stage 3: Sandbox Boundary Verification
```bash
bash scripts/test-sandbox-boundaries.sh
```
Runs the 10-step security boundary suite, confirming Landlock denial of `/root`, `$HOME`, `/etc/shadow`, OverlayFS whiteouts, and host `/etc` hash preservation.

### Stage 4: Static Analysis & Code Hygiene
```bash
go vet ./...
```

---

## 4. Git Branch & Commit Guidelines

### Contributor Identity
Configure your own real name and email address in Git:
```bash
git config user.name "Your Real Name"
git config user.email "your.email@example.com"
```

### Branching Discipline
- Create a dedicated branch off `main` for your work:
  ```bash
  git checkout -b feat/your-feature-name
  # or
  git checkout -b fix/issue-description
  ```
- Do not commit directly to `main`.

### Commit Message Standards
We follow Conventional Commits. Commit messages must be informative and self-documentary so that anyone reading `git log` understands the problem, the solution, and the rationale:

```git
feat(cli): add support for custom profile timeout flag

Adds a --timeout flag to the run subcommand allowing users to specify a maximum
execution duration for sandboxed child processes. If the timer expires, the PID 1
supervisor sends a SIGTERM followed by a SIGKILL to PID 2.

Fixes #42
```

Common commit prefixes:
- `feat`: New capability or command
- `fix`: Bug fix or security hardening
- `test`: Adding or refactoring tests
- `docs`: Documentation updates
- `refactor`: Internal code cleanup with zero behavioral change

---

## 5. Pull Request (PR) Requirements

When opening a Pull Request, ensure your description is thorough:
1. **Summary of Changes**: Clear explanation of what was changed and why.
2. **Problem Addressed**: Link to the relevant issue or describe the bug/gap.
3. **Reproduction & Verification**: Exact commands executed and terminal output demonstrating that the fix works and that all existing tests pass.
4. **Design Rationale**: Explain non-obvious engineering decisions or kernel tradeoffs.

---

## 6. Code Style & Invariant Rules

Please adhere to these invariants across all Go code and markdown files:

1. **Zero Emoji**: No emoji characters anywhere in source code, comments, test logs, commit messages, or documentation.
2. **Zero Em Dashes**: Do not use Unicode em dashes (`\u2014`). Use standard ASCII hyphens (`-`) or rephrase the sentence.
3. **No Landlock `.BestEffort()`**: Landlock rulesets must be applied without fallback. Never call `.BestEffort()` in Landlock setup; security initialization must fail loud.
4. **No Unflagged TODOs**: Do not commit vague `// TODO` or `// HACK` comments. If work is deferred, link it to a concrete tracked issue.
5. **Fail-Safe Error Handling**: Return structured errors that bubble up to `cli.Dispatch`. Do not call `os.Exit()` or `log.Fatal()` from inside library packages (`internal/isolation`, `internal/revert`, `internal/profiles`, etc.).
