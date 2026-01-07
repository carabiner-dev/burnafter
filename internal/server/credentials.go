// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// peerCredentials implements GRPC's credentials.TransportCredentials
// for Unix sockets.
type peerCredentials struct{}

// NewPeerCredentials creates transport credentials that extract peer info
func NewPeerCredentials() credentials.TransportCredentials {
	return &peerCredentials{}
}

func (c *peerCredentials) ClientHandshake(ctx context.Context, authority string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return rawConn, &peerAuthInfo{}, nil
}

// ServerHandshake handles the new connection from the client. It extracts the
// peer information from the socket calling GetsockoptUcred in the GetPeerCredentials
// function.
func (c *peerCredentials) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	// Extract peer credentials from Unix socket
	unixConn, ok := rawConn.(*net.UnixConn)
	if !ok {
		return rawConn, &peerAuthInfo{}, nil
	}

	pid, uid, gid, err := GetPeerCredentials(unixConn)
	if err != nil {
		// Don't fail the handshake, just log and continue
		return rawConn, &peerAuthInfo{}, nil
	}

	return rawConn, &peerAuthInfo{
		PID: pid,
		UID: uid,
		GID: gid,
	}, nil
}

func (c *peerCredentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{
		SecurityProtocol: "unix",
		SecurityVersion:  "1.0",
	}
}

func (c *peerCredentials) Clone() credentials.TransportCredentials {
	return &peerCredentials{}
}

func (c *peerCredentials) OverrideServerName(string) error {
	return nil
}

// peerAuthInfo contains authentication info from peer credentials
type peerAuthInfo struct {
	PID int32
	UID uint32
	GID uint32
}

func (a *peerAuthInfo) AuthType() string {
	return "unix-peercred"
}

// GetPeerAuthInfo extracts peerAuthInfo from context
func GetPeerAuthInfo(ctx context.Context) (*peerAuthInfo, error) {
	// First try to get from peer context
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no peer in context")
	}

	authInfo, ok := p.AuthInfo.(*peerAuthInfo)
	if !ok {
		return nil, fmt.Errorf("auth info is not peerAuthInfo, got %T", p.AuthInfo)
	}

	return authInfo, nil
}
