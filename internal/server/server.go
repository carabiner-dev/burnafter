// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
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

// StoredSecret represents a secret stored in memory
type StoredSecret struct {
	Name              string        // Name of the secret
	EncryptedData     []byte        // Encrypted secret data
	Salt              []byte        // Salt used for key derivation
	ClientBinaryHash  string        // Hash of the client binary that stored it
	ClientNonce       string        // Compile-time nonce from client
	InactivityTTL     time.Duration // TTL for inactivity-based expiration
	AbsoluteExpiresAt *time.Time    // Optional absolute expiration time (nil = no absolute expiration)
	LastAccessed      time.Time     // Last time this secret was accessed
}

// Server implements the BurnAfter gRPC service
type Server struct {
	pb.UnimplementedBurnAfterServer

	// Server options
	options *options.Server

	// secrets is the key-value map that stores th encrypted secrets
	secrets   map[string]*StoredSecret
	secretsMu sync.RWMutex

	// Session ID is the server's secret nonce generated as part of the
	// data required to derive the encryption key.
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
		secrets:      map[string]*StoredSecret{},
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

	// Start cleanup goroutine
	go s.cleanupExpiredSecrets()

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

// Ping implements the Ping RPC
func (s *Server) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	s.updateActivity()
	return &pb.PingResponse{Alive: true}, nil
}

// cleanupExpiredSecrets runs as a go routine and it periodically removes
// any expired secrets from memory.
func (s *Server) cleanupExpiredSecrets() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.secretsMu.Lock()
			now := time.Now()
			for name, secret := range s.secrets {
				expired := false
				var reason string

				// Check the secret's inactivity expiration time
				if time.Since(secret.LastAccessed) > secret.InactivityTTL {
					expired = true
					reason = "inactivity timeout"
				}

				// Check the absolute expiration date, this will wipe
				// the secret regardless if it has been accesses or not
				// (this absolute date is optional)
				if secret.AbsoluteExpiresAt != nil && now.After(*secret.AbsoluteExpiresAt) {
					expired = true
					reason = "absolute deadline reached"
				}

				// Remove the secret if it's expired.
				if expired {
					if s.options.Debug {
						log.Printf("Removing expired secret '%s' (reason: %s)", name, reason)
					}
					delete(s.secrets, name)
				}
			}
			s.secretsMu.Unlock()
		case <-s.shutdownChan:
			return
		}
	}
}
