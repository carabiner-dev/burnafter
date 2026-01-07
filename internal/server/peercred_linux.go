// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build linux
// +build linux

package server

import (
	"fmt"
	"net"
	"syscall"
)

// GetPeerCredentials extracts PID, UID, and GID from the Unix socket connection
// coming in from the client.
func GetPeerCredentials(conn *net.UnixConn) (pid int32, uid uint32, gid uint32, err error) {
	// Get the underlying file descriptor of the incoming
	// connection.
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("getting raw connection: %w", err)
	}

	var ucred *syscall.Ucred
	var credErr error

	// Use the raw connection to call getsockopt, this returns the Ucred
	// struct from the socket.
	err = rawConn.Control(func(fd uintptr) {
		ucred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})

	if err != nil {
		return 0, 0, 0, fmt.Errorf("trying to control raw connection: %w", err)
	}

	if credErr != nil {
		return 0, 0, 0, fmt.Errorf("failed to get peer credentials: %w", credErr)
	}

	return ucred.Pid, ucred.Uid, ucred.Gid, nil
}
