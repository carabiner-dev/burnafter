#!/bin/bash
# Build burnafter-server for multiple platforms and compress

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
EMBED_DIR="$PROJECT_DIR/internal/embedded/servers"

# Pin the Go toolchain to the version in .go-version so the server binaries are
# byte-reproducible regardless of the developer's ambient Go. CI verifies that
# .go-version tracks the latest stable Go release, so local builds and CI build
# with the same toolchain.
GO_VERSION="$(tr -d '[:space:]' < "$PROJECT_DIR/.go-version")"
export GOTOOLCHAIN="go${GO_VERSION}"
echo "Using Go toolchain: ${GOTOOLCHAIN}"

# Platforms to build for
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
)

echo "Building burnafter-server for multiple platforms..."

sha1sum internal/embedded/servers/*/*/*

for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r GOOS GOARCH <<< "$platform"

    # Create platform-specific directory
    platform_dir="$EMBED_DIR/$GOOS/$GOARCH"
    mkdir -p "$platform_dir"

    output_path="$platform_dir/burnafter-server"

    echo "Building for $GOOS/$GOARCH..."

    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
        -o "$output_path" \
        -trimpath -buildvcs=false \
        -ldflags="-s -w" \
        "$PROJECT_DIR/cmd/burnafter-server"

    # Compress with Go's compress/gzip (via hack/gzip) rather than the system
    # gzip: its DEFLATE output is identical on every host for a given Go version,
    # whereas the system gzip differs (e.g. BSD/Apple vs GNU), which broke the
    # reproducibility check.
    echo "Compressing burnafter-server for ${GOOS}/${GOARCH}..."
    go run "$PROJECT_DIR/hack/gzip" "$output_path"

    # Show size
    size=$(du -h "${output_path}.gz" | cut -f1)
    echo "  -> ${GOOS}/${GOARCH}/burnafter-server.gz (${size})"
done

sha1sum internal/embedded/servers/*/*/*

echo ""
echo "All server binaries built and compressed in $EMBED_DIR"
find "$EMBED_DIR" -name "*.gz" -exec ls -lh {} \;
