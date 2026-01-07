// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build darwin
// +build darwin

package server

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// GetPeerCredentials extracts PID, UID, and GID from the Unix socket connection
// coming in from the client on macOS/Darwin.
func GetPeerCredentials(conn *net.UnixConn) (pid int32, uid uint32, gid uint32, err error) {
	// Get the underlying file descriptor of the incoming connection
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("getting raw connection: %w", err)
	}

	var xucred *unix.Xucred
	var credErr error

	// Use the raw connection to call getsockopt with LOCAL_PEERCRED (macOS)
	err = rawConn.Control(func(fd uintptr) {
		xucred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})

	if err != nil {
		return 0, 0, 0, fmt.Errorf("trying to control raw connection: %w", err)
	}

	if credErr != nil {
		return 0, 0, 0, fmt.Errorf("failed to get peer credentials: %w", credErr)
	}

	// Note: macOS Xucred doesn't include PID, so we return 0 for PID
	// This is a limitation of the macOS API
	return 0, xucred.Uid, xucred.Groups[0], nil
}
