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
cp "$DIST_DIR/$TARBALL_AMD64" "$DIST_DIR/safebox-linux-amd64.tar.gz"
cp "$DIST_DIR/$TARBALL_ARM64" "$DIST_DIR/safebox-linux-arm64.tar.gz"

(
  cd "$DIST_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$TARBALL_AMD64" "$TARBALL_ARM64" safebox-linux-amd64.tar.gz safebox-linux-arm64.tar.gz > checksums.txt
  else
    shasum -a 256 "$TARBALL_AMD64" "$TARBALL_ARM64" safebox-linux-amd64.tar.gz safebox-linux-arm64.tar.gz > checksums.txt
  fi
)

echo "========================================================"
echo " Release Build Complete!"
echo " Artifacts in: $DIST_DIR"
cat "$DIST_DIR/checksums.txt"
echo "========================================================"
echo
echo "NEXT STEPS TO PUBLISH GITHUB RELEASE (PROTECTED MAIN):"
echo "  1. Merge your Pull Request into main via the GitHub web interface"
echo "     (direct pushes to main are disabled/protected)."
echo "  2. Switch to main locally and pull the merged commit:"
echo "       git checkout main && git pull origin main"
echo "  3. Create an annotated tag on main and push the tag:"
echo "       git tag -a $VERSION -m \"Safebox $VERSION release\""
echo "       git push origin $VERSION"
echo "  4. GitHub Actions (.github/workflows/release.yml) will automatically"
echo "     cross-compile and publish the release assets to GitHub."
echo "     (Manual fallback: gh release create $VERSION $DIST_DIR/* --title \"Safebox $VERSION\")"
echo "========================================================"
