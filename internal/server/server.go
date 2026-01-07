// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/carabiner-dev/burnafter/internal/common"
	pb "github.com/carabiner-dev/burnafter/internal/common"
	"github.com/carabiner-dev/burnafter/internal/options"
	"google.golang.org/grpc"
)

// Server implements the BurnAfter gRPC service
type Server struct {
	pb.UnimplementedBurnAfterServer

	// Server options
	options *options.Server

	// secrets is the key-value map that stores th encrypted secrets
	secrets   map[string]*common.StoredSecret
	secretsMu sync.RWMutex

	// Session ID is the servers secret nonce generated to derive
	// the server key.
	sessionID string

	lastActivity time.Time
	activityMu   sync.Mutex

	inactivityTimer *time.Timer
	shutdownChan    chan struct{}
}

// NewServer creates a new BurnAfter server with the supplied options
func NewServer(opts *options.Server) (*Server, error) {
	// Generat the random session ID
	sessionID, err := common.GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Create the server
	s := &Server{
		secrets:      map[string]*common.StoredSecret{},
		sessionID:    sessionID,
		lastActivity: time.Now(),
		options:      opts,
		shutdownChan: make(chan struct{}),
	}

	return s, nil
}

// Run starts the server and blocks until shutdown
func (s *Server) Run() error {
	// Remove existing socket file if it exists
	if err := os.RemoveAll(s.options.SocketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", s.options.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	defer listener.Close()

	// Set socket permissions to be restrictive (owner only)
	if err := os.Chmod(s.options.SocketPath, 0600); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	if s.options.Debug {
		log.Printf("Server listening on %s", s.options.SocketPath)
		log.Printf("Session ID: %s", s.sessionID)
	}

	// Create gRPC server with custom credentials to extract peer info
	grpcServer := grpc.NewServer(
		grpc.Creds(NewPeerCredentials()),
	)
	pb.RegisterBurnAfterServer(grpcServer, s)

	// Start inactivity monitor
	s.inactivityTimer = time.AfterFunc(s.options.InactivityTimeout, func() {
		if s.options.Debug {
			log.Printf("Inactivity timeout reached, shutting down")
		}
		grpcServer.GracefulStop()
		close(s.shutdownChan)
	})

	// Serve
	if err := grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}

	return nil
}

// updateActivity updates the last activity timestamp of the server.
func (s *Server) updateActivity() {
	s.activityMu.Lock()
	defer s.activityMu.Unlock()

	s.lastActivity = time.Now()

	// Reset the inactivity timer
	if s.inactivityTimer != nil {
		s.inactivityTimer.Reset(s.options.InactivityTimeout)
	}
}
