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

echo "=== Boundary Check 2: Working directory write access ==="
TEST_FILE=".safebox_test_probe_$$"
(cd "$SAFEBOX_DIR" && "$BINARY" run -- touch "$TEST_FILE")
if [[ -f "$SAFEBOX_DIR/$TEST_FILE" ]]; then
    rm -f "$SAFEBOX_DIR/$TEST_FILE"
    echo "OK: Working directory write allowed."
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

echo "All sandbox boundary checks PASSED."
exit 0
