#!/bin/bash
# Build burnafter-server for multiple platforms and compress

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
EMBED_DIR="$PROJECT_DIR/internal/embedded/servers"

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

    GOOS=$GOOS GOARCH=$GOARCH go build \
        -o "$output_path" \
        -trimpath -buildvcs=false \
        -ldflags="-s -w" \
        "$PROJECT_DIR/cmd/burnafter-server"

    # Compress with gzip. Make sure to use -n as we need to 
    # make the gzip output deterministic.
    echo "Compressing burnafter-server for ${GOOS}/${GOARCH}..."
    gzip -n -9 -f "$output_path"

    # Show size
    size=$(du -h "${output_path}.gz" | cut -f1)
    echo "  -> ${GOOS}/${GOARCH}/burnafter-server.gz (${size})"
done

sha1sum internal/embedded/servers/*/*/*

echo ""
echo "All server binaries built and compressed in $EMBED_DIR"
find "$EMBED_DIR" -name "*.gz" -exec ls -lh {} \;
