# Contributing to Safebox

Thank you for your interest in Safebox. Safebox is an unprivileged Linux sandbox and change-management engine built for autonomous AI agents and developer workflows.

This document is organized into two dedicated sections:
- **Part I: AI Coding Agent Instructions & Workspace Bootstrap** (instructions for AI assistants like Claude Code, agy, Cursor, Aider)
- **Part II: Human Developer Contribution Guide** (instructions for open-source human contributors)

---

# Part I: AI Coding Agent Instructions & Workspace Bootstrap

If you are an AI coding assistant working on Safebox, read and follow this section first.

## 1. Workspace Layout & Non-Git Rule

Safebox is developed within a multi-root workspace. The core Go codebase lives in `agent-safebox/` (the Git repository), while all agent-facing notes, specifications, and scratch files live in `project-knowledge/` (a separate, non-git directory):

```
workspace/
├── agent-safebox.code-workspace
├── agent-safebox/              (the core git repository: Go code & tests)
└── project-knowledge/           (NON-GIT directory for AI agent artifacts)
    └── AGENTS.md               (auto-progressive agent rules & index)
```

### Critical Workspace Invariants:
1. **`project-knowledge/` is Non-Git by Default**: Never track `project-knowledge/` inside the `agent-safebox/` git repository. All AI session scratch pads, prompt notes, and progress logs must stay inside `project-knowledge/` and must never pollute `agent-safebox/`.
2. **Git Commands Run Strictly in `agent-safebox/`**: Always execute git commands (`status`, `diff`, `commit`, `branch`, `push`) from within `/root/go-safebox/workspace/agent-safebox` (or the equivalent `agent-safebox/` root).
3. **No AI Documentation in Core**: Never commit raw conversational logs, agent prompt histories, or verbose AI scratch notes to `agent-safebox/`.

---

## 2. Bootstrap Protocol: Creating `AGENTS.md`

When pointed to this document, check if `../project-knowledge/AGENTS.md` exists:
- If `../project-knowledge/AGENTS.md` **does not exist**, create the directory `../project-knowledge/` and write the file `../project-knowledge/AGENTS.md` using the specification below.
- If it already exists, read it completely before modifying any code.

### Auto-Progressive Indexing Rule
At the start of every session, scan `../project-knowledge/` for any new directories or documentation files added by the developer (such as architecture notes, specifications, manuals, or review docs). Dynamically update the **Documentation Index** section of `project-knowledge/AGENTS.md` to list them. This ensures `AGENTS.md` automatically reflects the developer's active workflow and context.

---

## 3. Embedded `AGENTS.md` Specification

Below is the complete, authoritative specification to be placed in `../project-knowledge/AGENTS.md`:

```markdown
# AGENTS.md

**Project:** safebox
**TDD_MODE:** strict (all changes must include tests)
**REPO_TYPE:** monolith

---

## 0. Workspace & Git Boundaries

1. **agent-safebox/** is the git repository. All code, unit tests, and integration scripts live here. Every git command runs inside `agent-safebox/`.
2. **project-knowledge/** is non-git tracked by default. All agent notes, task tracking, and specifications live here.
3. Git branch discipline: Always develop on a feature or fix branch (e.g. `feat/...`). Never commit directly to `main`.

---

## 1. Absolute Rules

These apply everywhere, always, without exception:

- **Mandatory Tests**: Every code change, bug fix, or new capability MUST include corresponding tests. All tests must pass cleanly before declaring work complete.
- **Zero Emoji**: No emoji in code, comments, docs, commit messages, console output, or UI strings. Plain text only.
- **Zero Em Dashes**: Use regular ASCII hyphens (-) or rewrite the sentence. Unicode em dashes are strictly forbidden.
- **Fail-Safe Over Fail-Open (NFR1)**: No code path executes the wrapped command without namespace, Landlock, and init-shim setup succeeding. A setup failure is a hard stop, never a warning.
- **No Landlock .BestEffort()**: Never use .BestEffort() in Landlock calls. It silently succeeds on unsupported kernels, which is forbidden. Treat Landlock setup errors as fatal.
- **Strict Default Allow-List**: The default Landlock allow-list never includes any path under a user's home directory ($HOME, ~/.ssh, ~/.aws). Extending access beyond the default four system paths requires an explicit --allow-path flag.
- **KISS**: Every document and function is the simplest version that communicates the fact or does the job.
- **No Quick Hacks**: Deferred work must be explicitly flagged with tracked issues, not silently skipped.
- **No Silent Decisions**: Any non-obvious engineering choice must be documented in code comments or commit messages.

---

## 2. Documentation Index (Auto-Progressive)

Scan ../project-knowledge/ and list discovered documents here:
- AGENTS.md: This governance and rulebook file.
(Auto-updated by agent as developer adds new files in project-knowledge/)
```

---

# Part II: Human Developer Contribution Guide

If you are a human open-source developer contributing to Safebox, welcome! We adhere to high engineering rigor and test-driven development.

## 1. Development Prerequisites & Setup

### Prerequisites
- **Linux Kernel**: Linux 5.13 or newer (required for Landlock LSM ABI v1+).
- **Go**: Version 1.22 or newer.
- **Optional Network Backends**: `passt` or `slirp4netns` for testing userspace NAT egress (`--allow-net`).

### Cloning and Building
```bash
git clone https://github.com/Mahboob-A/agent-safebox.git
cd agent-safebox
go build -o safebox .
./safebox help
```

---

## 2. Mandatory Four-Stage Verification Battery

Every contribution must include tests, and your branch must pass the full four-stage test battery before opening a pull request:

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

## 3. Git Branch & Commit Standards

- **Branch Discipline**: Fork the repository and create a descriptive branch off `main` (e.g., `feat/custom-profile-flag` or `fix/whiteout-cleanup`). Do not commit directly to `main`.
- **Commit Messages**: Follow Conventional Commits (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`). Commit messages must be informative and self-documentary, providing full context so anyone reading `git log` understands what changed and why.

---

## 4. Pull Request (PR) Requirements

When submitting a PR, provide a complete, detailed description:
1. **Summary**: What changed and why.
2. **Problem Addressed**: Link to the issue or describe the bug/gap.
3. **Reproduction & Verification**: Exact commands executed and terminal output demonstrating that all existing and new tests pass.
4. **Design Decisions**: Rationale for non-obvious choices or kernel tradeoffs.

---

## 5. Core Repository Hygiene

The core `safebox` repository is reserved strictly for Go source code, tests, and human-crafted documentation.
- If you use AI coding assistants in your workflow, keep all agent notes, progress journals, and conversation logs in a separate directory outside the repository.
- Do not commit AI prompt logs, scratch files, or conversational transcripts to `safebox`.
