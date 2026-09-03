#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFEBOX_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
ERRORS=0

SEARCH_DIRS=("$SAFEBOX_DIR")
if [ -d "$SAFEBOX_DIR/../project-knowledge/progress" ]; then
    SEARCH_DIRS+=("$(cd "$SAFEBOX_DIR/../project-knowledge/progress" && pwd)")
fi

echo "[1/6] Checking git branch / release context..."
BRANCH=$(git -C "$SAFEBOX_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
if [ "${GITHUB_ACTIONS:-false}" = "true" ]; then
    echo "OK: Running in GitHub Actions CI environment ($BRANCH)."
elif [[ "$BRANCH" == "main" ]]; then
    echo "FAIL: Current branch is 'main'. Direct commits to main are forbidden." >&2
    ERRORS=$((ERRORS + 1))
else
    echo "OK: Branch is '$BRANCH' (non-main)."
fi

echo "[2/6] Checking git author identity..."
GIT_NAME=$(git -C "$SAFEBOX_DIR" log -1 --format='%an' 2>/dev/null || git -C "$SAFEBOX_DIR" config user.name || echo "")
GIT_EMAIL=$(git -C "$SAFEBOX_DIR" log -1 --format='%ae' 2>/dev/null || git -C "$SAFEBOX_DIR" config user.email || echo "")
if [[ "$GIT_NAME" != "Mahboob Alam" || "$GIT_EMAIL" != "connect.mahboobalam@gmail.com" ]]; then
    echo "FAIL: Invalid git identity '$GIT_NAME <$GIT_EMAIL>'. Must be 'Mahboob Alam <connect.mahboobalam@gmail.com>'." >&2
    ERRORS=$((ERRORS + 1))
else
    echo "OK: Git identity matches 'Mahboob Alam <connect.mahboobalam@gmail.com>'."
fi

echo "[3/6] Checking for forbidden .BestEffort() in Landlock calls..."
if grep -rnw "$SAFEBOX_DIR" -e "BestEffort" --include="*.go" 2>/dev/null; then
    echo "FAIL: Found .BestEffort() call. Landlock setup must fail loud, no BestEffort allowed." >&2
    ERRORS=$((ERRORS + 1))
else
    echo "OK: No .BestEffort() found."
fi

echo "[4/6] Checking for em dashes in code, progress, and review files..."
EM_DASH_FOUND=0
if grep -rnIP '\x{2014}' "${SEARCH_DIRS[@]}" --exclude-dir=".agents" --exclude-dir=".git" --exclude-dir="dist" 2>/dev/null; then
    EM_DASH_FOUND=1
fi
if [[ $EM_DASH_FOUND -eq 1 ]]; then
    echo "FAIL: Found em dash. Use regular hyphen (-) per formatting rules." >&2
    ERRORS=$((ERRORS + 1))
else
    echo "OK: No em dashes found in checked directories."
fi

echo "[5/6] Checking for emoji in code, progress, and review files..."
EMOJI_FOUND=0
if grep -rnIP '[\x{1F600}-\x{1F64F}\x{1F300}-\x{1F5FF}\x{1F680}-\x{1F6FF}\x{1F1E0}-\x{1F1FF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}]' "${SEARCH_DIRS[@]}" --exclude-dir=".agents" --exclude-dir=".git" --exclude-dir="dist" 2>/dev/null; then
    EMOJI_FOUND=1
fi
if [[ $EMOJI_FOUND -eq 1 ]]; then
    echo "FAIL: Found emoji characters. Plain text only." >&2
    ERRORS=$((ERRORS + 1))
else
    echo "OK: No emoji found in checked directories."
fi

echo "[6/6] Checking for unflagged TODOs or quick hacks in Go files..."
if grep -rnP '(TODO|FIXME|HACK)' "$SAFEBOX_DIR" --include="*.go" --exclude-dir=".agents" --exclude-dir=".git" --exclude-dir="dist" 2>/dev/null; then
    echo "WARNING: Found TODO/FIXME/HACK comments. Verify these are explicitly documented in phase logs." >&2
else
    echo "OK: No unflagged TODO/FIXME/HACK in code."
fi

if [[ $ERRORS -gt 0 ]]; then
    echo "Rule check FAILED with $ERRORS error(s)." >&2
    exit 1
fi

echo "All rule checks PASSED."
exit 0
