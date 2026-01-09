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

	"google.golang.org/grpc"

	"github.com/carabiner-dev/burnafter/internal/common"
	isecrets "github.com/carabiner-dev/burnafter/internal/secrets"
	"github.com/carabiner-dev/burnafter/options"
	"github.com/carabiner-dev/burnafter/secrets"
)

// Server implements the BurnAfter gRPC service
type Server struct {
	common.UnimplementedBurnAfterServer

	// Server options
	options *options.Server

	// secrets is the key-value map that stores secret metadata
	secrets   map[string]*secrets.Metadata
	secretsMu sync.RWMutex

	// storage is the backend that stores the actual encrypted secret data
	storage secrets.Storage

	// Session ID is the server's secret nonce generated as part of the
	// data required to derive the encryption key.
	sessionID string

	lastActivity time.Time
	activityMu   sync.Mutex

	inactivityTimer *time.Timer
	shutdownChan    chan struct{}
	grpcServer      *grpc.Server
}

// NewServer creates a new BurnAfter server with the supplied options
func NewServer(opts *options.Server) (*Server, error) {
	// Generat the random session ID
	sessionID, err := common.GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Initialize the storage driver
	var storage secrets.Storage

	// In Linux, try to use the kernel keyring driver to store the encrypted secrets.
	keyringStorage, err := isecrets.NewKeyringStorage()
	if err == nil {
		if opts.Debug {
			log.Printf("Using kernel keyring storage for secrets")
		}
		storage = keyringStorage
	}

	// .. but fall back to memory storage if not available
	if storage == nil {
		if opts.Debug {
			log.Printf("Kernel keyring not available, using memory storage: %v", err)
		}
		storage = isecrets.NewMemoryStorage()
	}

	// Create the server
	s := &Server{
		secrets:      map[string]*secrets.Metadata{},
		storage:      storage,
		sessionID:    sessionID,
		lastActivity: time.Now(),
		options:      opts,
		shutdownChan: make(chan struct{}),
	}

	return s, nil
}

// Run starts the server and blocks until shutdown
func (s *Server) Run() error {
	// Remove existing socket file if it already exists
	if err := os.RemoveAll(s.options.SocketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create Unix domain socket listener
	lc := net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "unix", s.options.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	defer listener.Close() //nolint:errcheck

	// Set socket permissions to be restrictive (owner only)
	if err := os.Chmod(s.options.SocketPath, 0o600); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	if s.options.Debug {
		log.Printf("Server listening on %s", s.options.SocketPath)
		log.Printf("Session ID: %s", s.sessionID)
	}

	// Create gRPC server with custom credentials to extract peer info
	s.grpcServer = grpc.NewServer(
		grpc.Creds(NewPeerCredentials()),
	)
	common.RegisterBurnAfterServer(s.grpcServer, s)

	// Start cleanup goroutine
	go s.cleanupExpiredSecrets()

	// Start inactivity monitor
	if s.options.InactivityTimeout > 0 {
		s.inactivityTimer = time.AfterFunc(s.options.InactivityTimeout, func() {
			if s.options.Debug {
				log.Printf("Inactivity timeout reached, shutting down")
			}
			s.grpcServer.GracefulStop()
			close(s.shutdownChan)
		})
	}

	// Serve
	if err := s.grpcServer.Serve(listener); err != nil {
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
func (s *Server) Ping(ctx context.Context, req *common.PingRequest) (*common.PingResponse, error) {
	s.updateActivity()
	return &common.PingResponse{Alive: true}, nil
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
					// Also delete from the storage backend
					_ = s.storage.Delete(context.Background(), name) //nolint:errcheck
				}
			}

			// Check if all secrets have been removed
			secretCount := len(s.secrets)
			s.secretsMu.Unlock()

			// If no secrets remain, shutdown the server
			if secretCount == 0 && s.grpcServer != nil {
				if s.options.Debug {
					log.Printf("No secrets remaining, shutting down server")
				}
				s.grpcServer.GracefulStop()
				close(s.shutdownChan)
				return
			}
		case <-s.shutdownChan:
			return
		}
	}
}
