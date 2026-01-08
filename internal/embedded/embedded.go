// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package embedded

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

// embeddedServerBinary is defined in platform-specific files with build tags:
// - embedded_linux_amd64.go
// - embedded_linux_arm64.go
// - embedded_darwin_amd64.go
// - embedded_darwin_arm64.go
//
// Each file embeds only the server binary for its specific platform,
// reducing the final binary size significantly.

// getServerBinary reads and decompresses the embedded server binary for the current platform
func getServerBinary() ([]byte, error) {
	// embeddedServerBinary is the compressed .gz data
	compressedData := embeddedServerBinary
	if len(compressedData) == 0 {
		return nil, fmt.Errorf("no embedded server binary found for this platform")
	}

	// Decompress using gzip
	gzReader, err := gzip.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Read decompressed data
	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress server binary: %w", err)
	}

	return decompressed, nil
}

// ExtractServerBinaryToTemp writes the server binary to a temporary file with a
// randomized name. Returns the path to the extracted binary.
//
// On MacOS it will attempt to remove the quarantine bit from the extracted file.
func ExtractServerBinaryToTemp() (string, error) {
	// Get the decompressed server binary for this platform
	serverBinary, err := getServerBinary()
	if err != nil {
		return "", err
	}

	// Create a temporary file with a unique name
	tmpFile, err := os.CreateTemp("", "burnafter-server-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write the server binary
	if _, err := tmpFile.Write(serverBinary); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write server binary: %w", err)
	}

	// Make it executable
	if err := tmpFile.Chmod(0700); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}

	tmpFile.Close()

	// On macOS, remove quarantine attribute to allow execution
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("xattr", "-d", "com.apple.quarantine", tmpPath)
		_ = cmd.Run() // Ignore errors - attribute might not exist
	}

	return tmpPath, nil
}
