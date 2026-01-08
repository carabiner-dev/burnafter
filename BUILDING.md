# Building Burnafter

## Overview

Burnafter uses embedded server binaries for cross-platform support. When used as
a library, the server binary for the target platform is embedded directly into
the client, eliminating the need for separate installations or build steps by
library consumers.

## Build Process

### Quick Start

```bash
# Build everything (protos, server binaries, and client)
make all

# Or build just the client (requires server binaries to be pre-built)
make build
```

### Server Binaries

Server binaries are pre-compiled for multiple platforms and embedded into the
client library:

- **Platforms**: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
- **Compression**: gzip -9 (3.5-3.9 MB per platform)
- **Location**: `internal/client/embedded/{os}/{arch}/burnafter-server.gz`
- **Build Tags**: Each platform uses build tags to embed only the required binary

These .gz files are committed to the repository. Note that windows is not
supported yet.

To rebuild server binaries:

```bash
make build-server

# or run the script directly:
./hack/build-server-all-platforms.sh
```

This will:

1. Build `cmd/burnafter-server` for each platform
2. Strip debug symbols (`-ldflags="-s -w"`)
3. Compress with gzip -9
4. Save to `internal/client/embedded/{os}/{arch}/burnafter-server.gz`

**Directory Structure**:

```text
internal/client/embedded/
├── darwin/
│   ├── amd64/burnafter-server.gz
│   └── arm64/burnafter-server.gz
└── linux/
    ├── amd64/burnafter-server.gz
    └── arm64/burnafter-server.gz
```

### How Embedding Works

1. **Build time**: Server binaries are compiled and compressed for all platforms
2. **Embed time**: Platform-specific files with build tags embed only the required binary:
   - `embedded_linux_amd64.go` - embeds `embedded/linux/amd64/burnafter-server.gz`
   - `embedded_linux_arm64.go` - embeds `embedded/linux/arm64/burnafter-server.gz`
   - `embedded_darwin_amd64.go` - embeds `embedded/darwin/amd64/burnafter-server.gz`
   - `embedded_darwin_arm64.go` - embeds `embedded/darwin/arm64/burnafter-server.gz`
3. **Compile time**: Go build tags select the correct embedded file for the target platform
4. **Runtime**: Client decompresses the embedded binary
5. **Execution**:
   - **Linux**: Uses `memfd_create()` to execute from memory (zero disk writes)
   - **macOS/Other**: Extracts to `~/.cache/burnafter/` and executes from there

### Final Binary Size

The client binary includes only the server binary for its target platform:

- Compressed server binary: ~3.5-3.9 MB (platform-specific)
- Client code: ~15 MB
- **Total**: ~19 MB

## Development Workflow

### Making Changes to Server Code

If you modify code that alters the server binary, especially files in
`internal/server/` or `cmd/burnafter-server/`, rebuild the server binaries
or CI checks will fail:

```bash
# Rebuild server binaries for all platforms
make build-server

# Rebuild client with new embedded servers
make build

# Test
./burnafter store test "value" 300
```

### Adding New Platforms

Edit `scripts/build-server-all-platforms.sh` and add to the `PLATFORMS` array:

```bash
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "freebsd/amd64"  # new platform
)
```

Then rebuild:

```bash
make build-server && make build
```

### Cross-Compilation Notes

- Server binaries are cross-compiled using `GOOS` and `GOARCH` environment variables
- Platform-specific code uses build tags (e.g., `//go:build linux`)
- Peer credentials work differently on Linux (`SO_PEERCRED`) vs macOS (`LOCAL_PEERCRED`)
- `memfd_create()` is Linux-only; other platforms use cache extraction

## Library Usage

When someone does `go get github.com/carabiner-dev/burnafter`, they get:

- Source code
- Pre-built, compressed server binaries (`.gz` files in repo)
- Everything needed to build without extra dependencies

The user just needs to:

```go
import "github.com/carabiner-dev/burnafter/pkg/burnafter"

client := burnafter.NewClient(opts)
client.Connect() // Auto-extracts and starts server
```
