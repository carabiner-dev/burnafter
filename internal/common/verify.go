// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// GetClientBinaryInfo extracts the binary path and hash from the client's PID
func GetClientBinaryInfo(pid int32) (binaryPath string, binaryHash string, err error) {
	// Read the /proc/[pid]/exe symlink to get the binary path
	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	binaryPath, err = os.Readlink(exePath)
	if err != nil {
		return "", "", fmt.Errorf("reading binary path: %w", err)
	}

	// Compute SHA256 hash of the binary
	binaryHash, err = HashFile(binaryPath)
	if err != nil {
		return "", "", fmt.Errorf("hashing client binary: %w", err)
	}

	return binaryPath, binaryHash, nil
}

// HashFile computes the SHA256 hash of a file
func HashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// GetCurrentBinaryHash returns the hash of the currently running binary
func GetCurrentBinaryHash() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("getting the executable path: %w", err)
	}

	// Resolve symlinks
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks: %w", err)
	}

	return HashFile(exePath)
}
