// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// extractServerBinaryToCache writes the server binary to a cache directory
// Returns the path to the extracted binary. Used as fallback when memfd is not available.
func extractServerBinaryToCache() (string, error) {
	// Get the decompressed server binary for this platform
	serverBinary, err := getServerBinary()
	if err != nil {
		return "", err
	}

	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".cache")
	}

	burnafterCache := filepath.Join(cacheDir, "burnafter")
	if err := os.MkdirAll(burnafterCache, 0700); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	serverPath := filepath.Join(burnafterCache, "burnafter-server")

	// Check if binary already exists and has matching content
	if existingData, err := os.ReadFile(serverPath); err == nil {
		if len(existingData) == len(serverBinary) {
			return serverPath, nil
		}
	}

	// Write/update the server binary
	if err := os.WriteFile(serverPath, serverBinary, 0700); err != nil {
		return "", fmt.Errorf("failed to write server binary: %w", err)
	}

	return serverPath, nil
}
