#!/bin/bash
set -euo pipefail

# Safebox Manual Release Script
# Builds release tarballs for linux/amd64 and linux/arm64 with SHA256 checksums.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VERSION="${1:-v0.5.0}"

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+.*$ ]]; then
  echo "Error: Version must follow semantic versioning format (e.g., v0.5.0), got '$VERSION'" >&2
  exit 1
fi

cd "$REPO_ROOT"

GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "========================================================"
echo " Building Safebox Release: $VERSION ($GIT_COMMIT)"
echo "========================================================"

# 1. Run rule verification
echo "[1/4] Running code and rule checks..."
if [ -f "$REPO_ROOT/.agents/skills/code-review/scripts/check-rules.sh" ]; then
  bash "$REPO_ROOT/.agents/skills/code-review/scripts/check-rules.sh"
fi

# 2. Run unit tests
echo "[2/4] Running test suite..."
go test ./...

# 3. Clean and prepare dist directory
echo "[3/4] Cross-compiling static Linux binaries..."
DIST_DIR="$REPO_ROOT/dist"
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

LDFLAGS="-s -w -X 'safebox/internal/cli.Version=${VERSION}' -X 'safebox/internal/cli.GitCommit=${GIT_COMMIT}' -X 'safebox/internal/cli.BuildDate=${BUILD_DATE}'"

# Target 1: linux/amd64
echo "  -> Compiling linux/amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -ldflags="$LDFLAGS" \
  -o "$DIST_DIR/safebox-linux-amd64" .

# Target 2: linux/arm64
echo "  -> Compiling linux/arm64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
  -trimpath \
  -ldflags="$LDFLAGS" \
  -o "$DIST_DIR/safebox-linux-arm64" .

# 4. Package tarballs and generate checksums
echo "[4/4] Packaging tarballs and generating SHA256 checksums..."
TARBALL_AMD64="safebox-${VERSION}-linux-amd64.tar.gz"
TARBALL_ARM64="safebox-${VERSION}-linux-arm64.tar.gz"

tar -czf "$DIST_DIR/$TARBALL_AMD64" -C "$DIST_DIR" safebox-linux-amd64
tar -czf "$DIST_DIR/$TARBALL_ARM64" -C "$DIST_DIR" safebox-linux-arm64

(
  cd "$DIST_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$TARBALL_AMD64" "$TARBALL_ARM64" > checksums.txt
  else
    shasum -a 256 "$TARBALL_AMD64" "$TARBALL_ARM64" > checksums.txt
  fi
)

echo "========================================================"
echo " Release Build Complete!"
echo " Artifacts in: $DIST_DIR"
cat "$DIST_DIR/checksums.txt"
echo "========================================================"
echo
echo "NEXT STEPS TO PUBLISH GITHUB RELEASE:"
echo "  1. Merge your PR and push changes to main branch:"
echo "       git checkout main && git merge <feature-branch> && git push origin main"
echo "  2. Tag the release on main and push the tag:"
echo "       git tag $VERSION"
echo "       git push origin $VERSION"
echo "  3. Create GitHub release and upload assets:"
echo "       gh release create $VERSION $DIST_DIR/$TARBALL_AMD64 $DIST_DIR/$TARBALL_ARM64 $DIST_DIR/checksums.txt --title \"Safebox $VERSION\" --notes \"Release $VERSION\""
echo "     Or drag-and-drop the tarballs in $DIST_DIR to: https://github.com/Mahboob-A/agent-safebox/releases/new"
echo "========================================================"
