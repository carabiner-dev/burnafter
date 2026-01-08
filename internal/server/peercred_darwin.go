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
	var peerPID int
	var credErr, pidErr error

	// Use the raw connection to call getsockopt
	err = rawConn.Control(func(fd uintptr) {
		// Get PID using LOCAL_PEEREPID (macOS-specific)
		peerPID, pidErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEEREPID)

		// Get UID/GID using LOCAL_PEERCRED
		xucred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})

	if err != nil {
		return 0, 0, 0, fmt.Errorf("trying to control raw connection: %w", err)
	}

	if pidErr != nil {
		return 0, 0, 0, fmt.Errorf("failed to get peer PID: %w", pidErr)
	}

	if credErr != nil {
		return 0, 0, 0, fmt.Errorf("failed to get peer credentials: %w", credErr)
	}

	return int32(peerPID), xucred.Uid, xucred.Groups[0], nil
}
