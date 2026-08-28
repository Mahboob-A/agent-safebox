#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFEBOX_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BINARY="$SAFEBOX_DIR/safebox"

if [[ ! -f "$BINARY" ]]; then
    echo "Notice: safebox binary not yet built. Building now..."
    (cd "$SAFEBOX_DIR" && go build -o safebox .)
fi

echo "=== Boundary Check 1: Argument validation (no args) ==="
set +e
"$BINARY" run > /dev/null 2>&1
EXIT_CODE=$?
set -e
if [[ $EXIT_CODE -eq 0 ]]; then
    echo "FAIL: 'safebox run' without command returned 0, expected non-zero error." >&2
    exit 1
else
    echo "OK: 'safebox run' without command exited non-zero ($EXIT_CODE)."
fi

echo "=== Boundary Check 2: Working directory write access (overlay write & apply) ==="
TEST_FILE=".safebox_test_probe_$$"
(cd "$SAFEBOX_DIR" && "$BINARY" run -- touch "$TEST_FILE")
if [[ -f "$SAFEBOX_DIR/$TEST_FILE" ]]; then
    echo "FAIL: File leaked directly to host before apply! Overlay isolation breached!" >&2
    exit 1
fi
(cd "$SAFEBOX_DIR" && "$BINARY" apply --yes > /dev/null)
if [[ -f "$SAFEBOX_DIR/$TEST_FILE" ]]; then
    rm -f "$SAFEBOX_DIR/$TEST_FILE"
    echo "OK: Working directory write allowed via overlay and applied."
else
    echo "FAIL: Could not write file in working directory inside sandbox." >&2
    exit 1
fi

echo "=== Boundary Check 3: Restricted filesystem access (/root) ==="
set +e
OUTPUT=$("$BINARY" run -- ls /root 2>&1)
EXIT_CODE=$?
set -e
if [[ $EXIT_CODE -eq 0 ]]; then
    echo "FAIL: Access to /root succeeded inside sandbox. Landlock policy breached!" >&2
    exit 1
else
    echo "OK: Access to /root correctly denied inside sandbox (exit $EXIT_CODE)."
fi

echo "=== Boundary Check 4: Network isolation ==="
set +e
"$BINARY" run -- ping -c 1 -W 1 8.8.8.8 > /dev/null 2>&1
EXIT_CODE=$?
set -e
if [[ $EXIT_CODE -eq 0 ]]; then
    echo "FAIL: Network ping succeeded inside sandbox. Network namespace isolation breached!" >&2
    exit 1
else
    echo "OK: Outbound network traffic blocked (exit $EXIT_CODE)."
fi

echo "=== Boundary Check 5: Read-only system path write denial (/etc) ==="
set +e
OUTPUT=$("$BINARY" run -- touch /etc/.safebox_rw_probe_$$ 2>&1)
EXIT_CODE=$?
set -e
if [[ $EXIT_CODE -eq 0 ]]; then
    rm -f /etc/.safebox_rw_probe_$$ 2>/dev/null || true
    echo "FAIL: Write to /etc succeeded inside sandbox. Landlock read-only policy breached!" >&2
    exit 1
else
    echo "OK: Write to /etc correctly denied inside sandbox (exit $EXIT_CODE)."
fi

echo "=== Boundary Check 6: Partial /etc access (shadow denied, passwd allowed) ==="
set +e
OUTPUT_SHADOW=$("$BINARY" run -- cat /etc/shadow 2>&1)
EXIT_SHADOW=$?
set -e
if [[ $EXIT_SHADOW -eq 0 ]]; then
    echo "FAIL: Read of /etc/shadow succeeded inside sandbox. Landlock sensitive file policy breached!" >&2
    exit 1
else
    echo "OK: Read of /etc/shadow correctly denied inside sandbox (exit $EXIT_SHADOW)."
fi

set +e
OUTPUT_PASSWD=$("$BINARY" run -- cat /etc/passwd 2>&1)
EXIT_PASSWD=$?
set -e
if [[ $EXIT_PASSWD -ne 0 ]]; then
    echo "FAIL: Read of /etc/passwd failed inside sandbox (exit $EXIT_PASSWD)." >&2
    exit 1
else
    echo "OK: Read of /etc/passwd allowed inside sandbox."
fi

echo "=== Boundary Check 7: Real CLI Agent execution with --allow-path, trace, and remediation ==="
if [[ -x "/root/.local/bin/agy" ]]; then
    set +e
    OUTPUT=$("$BINARY" run -- /root/.local/bin/agy --version 2>&1)
    EXIT_CODE=$?
    set -e
    if [[ $EXIT_CODE -eq 0 ]]; then
        echo "FAIL: /root/.local/bin/agy ran without --allow-path! Landlock policy breached!" >&2
        exit 1
    fi
    if [[ "$OUTPUT" != *"--allow-path=/root/.local/bin"* ]]; then
        echo "FAIL: Remediation hint missing for /root/.local/bin/agy denial." >&2
        exit 1
    fi
    echo "OK: Real tool denial and remediation hint verified."

    OUTPUT=$("$BINARY" run --allow-path=/root/.local/bin -- /root/.local/bin/agy --version 2>&1)
    if [[ "$OUTPUT" != *"1.1.22"* ]]; then
        echo "FAIL: Real agy execution with --allow-path failed." >&2
        exit 1
    fi
    if [[ "$OUTPUT" != *"[safebox]"* ]]; then
        echo "FAIL: Execution trace missing from real agy execution." >&2
        exit 1
    fi
    echo "OK: Real tool execution with --allow-path and trace verified."
else
    echo "SKIP: /root/.local/bin/agy not present on host."
fi

echo "All sandbox boundary checks PASSED."
exit 0
