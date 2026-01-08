// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package common

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getBinaryPath gets the binary path for a process on macOS
func getBinaryPath(pid int32) (string, error) {
	// Try using sysctl with the full MIB path
	// CTL_KERN=1, KERN_PROCARGS2=49
	mib := []int32{1, 49, pid}

	// First, get the size
	var size uintptr
	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		0,
		uintptr(unsafe.Pointer(&size)),
		0,
		0,
	)

	if errno != 0 {
		return "", fmt.Errorf("failed to get process args for pid %d: %w", pid, errno)
	}
	if size == 0 {
		return "", fmt.Errorf("failed to get process args size for pid %d: sysctl returned size 0", pid)
	}

	// Sanity cap on the argsize to avoid dangerous allocations / OOM.
	const maxSysctlArgsSize = 1 << 20 // 1 MiB
	if size > maxSysctlArgsSize {
		return "", fmt.Errorf("sysctl size %d too large for pid %d", size, pid)
	}

	// Get the actual data
	buf := make([]byte, size)
	_, _, errno = syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&buf[0])), // oldp
		uintptr(unsafe.Pointer(&size)),   // oldlenp (in/out)
		0,                                // newp
		0,                                // newlen
	)

	if errno != 0 {
		return "", fmt.Errorf("failed to get process args for pid %d: %w", pid, errno)
	}

	// Parse KERN_PROCARGS2 format:
	// int argc
	// executable path (null-terminated)
	// ...
	if len(buf) < 4 {
		return "", fmt.Errorf("buffer too small for pid %d", pid)
	}

	// Skip the argc (first 4 bytes)
	// Then find the executable path (null-terminated string)
	start := 4
	// Skip any leading nulls
	for start < len(buf) && buf[start] == 0 {
		start++
	}

	if start >= len(buf) {
		return "", fmt.Errorf("no executable path found for pid %d", pid)
	}

	// Find the end (null terminator)
	end := start
	for end < len(buf) && buf[end] != 0 {
		end++
	}

	if end == start {
		return "", fmt.Errorf("empty executable path for pid %d", pid)
	}

	return string(buf[start:end]), nil
}
