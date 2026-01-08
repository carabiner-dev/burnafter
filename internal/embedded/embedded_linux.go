// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build linux
// +build linux

package embedded

import (
	"context"
	"fmt"

	"github.com/chainguard-dev/clog"
	"golang.org/x/sys/unix"
)

// createMemfdServer creates an anonymous in-memory file containing the server binary
// and returns the file descriptor. This is Linux-specific.
func CreateMemfdServer(ctx context.Context) (int, error) {
	// Get the decompressed server binary for this platform
	serverBinary, err := getServerBinary()
	if err != nil {
		return -1, err
	}

	// Create an anonymous in-memory file using memfd_create
	// Use MFD_ALLOW_SEALING to allow sealing operations
	// We don't use MFD_CLOEXEC because we need the fd to be inherited by the child
	// Try with MFD_EXEC first (kernel 6.3+), fall back to 0 for older kernels
	fd, err := unix.MemfdCreate("burnafter-server", unix.MFD_ALLOW_SEALING|0x10)
	if err != nil {
		// Fall back to no flags if MFD_EXEC is not supported
		fd, err = unix.MemfdCreate("burnafter-server", unix.MFD_ALLOW_SEALING)
		if err != nil {
			return -1, fmt.Errorf("memfd_create failed: %w", err)
		}
	}

	// Write the embedded binary to the memfd
	n, err := unix.Write(fd, serverBinary)
	if err != nil {
		unix.Close(fd) //nolint:errcheck,gosec
		return -1, fmt.Errorf("failed to write binary to memfd: %w", err)
	}

	if n != len(serverBinary) {
		unix.Close(fd) //nolint:errcheck,gosec
		return -1, fmt.Errorf("incomplete write to memfd: wrote %d of %d bytes", n, len(serverBinary))
	}

	// Attempt to seal the memfd to prevent further modifications
	// This is optional - if it fails (e.g., due to SELinux), we continue anyway
	// as the sealing is a security enhancement but not required for functionality
	seals := unix.F_SEAL_SEAL | unix.F_SEAL_SHRINK | unix.F_SEAL_GROW | unix.F_SEAL_WRITE
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_ADD_SEALS, seals); err != nil {
		// Sealing failed, but we can continue - it's optional
		// In production code, you might want to log this
		clog.FromContext(ctx).WarnContext(ctx, "failed to seal server memory")
	}

	return fd, nil
}
