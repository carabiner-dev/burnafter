// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package common

import (
	"fmt"
	"os"
)

// getBinaryPath gets the binary path for a process on Linux
func getBinaryPath(pid int32) (string, error) {
	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	binaryPath, err := os.Readlink(exePath)
	if err != nil {
		return "", err
	}
	return binaryPath, nil
}
