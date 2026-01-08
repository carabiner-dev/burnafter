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
	// Use sysctl kern.proc.pathname to get the executable path
	mib := []int32{1, 14, 12, pid} // CTL_KERN, KERN_PROC, KERN_PROC_PATHNAME, pid

	// Query the size first
	n := uintptr(0)
	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		0,
		uintptr(unsafe.Pointer(&n)),
		0,
		0,
	)
	if errno != 0 {
		return "", errno
	}

	if n == 0 {
		return "", fmt.Errorf("no path returned for pid %d", pid)
	}

	// Now get the actual path
	buf := make([]byte, n)
	_, _, errno = syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&n)),
		0,
		0,
	)
	if errno != 0 {
		return "", errno
	}

	// Remove null terminator if present
	if n > 0 && buf[n-1] == 0 {
		n--
	}

	return string(buf[:n]), nil
}
