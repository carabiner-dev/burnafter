// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/carabiner-dev/burnafter/internal/common"
	isecrets "github.com/carabiner-dev/burnafter/internal/secrets"
	"github.com/carabiner-dev/burnafter/options"
	"github.com/carabiner-dev/burnafter/secrets"
	"github.com/chainguard-dev/clog"
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

	// ctx holds the server's root context with logger
	ctx context.Context
}

// NewServer creates a new BurnAfter server with the supplied options
func NewServer(ctx context.Context, opts *options.Server) (*Server, error) {
	// Generat the random session ID
	sessionID, err := common.GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Initialize the storage driver
	var storage secrets.Storage

	// In Linux, try to use the kernel keyring driver to store the encrypted secrets.
	keyringStorage, err := isecrets.NewKeyringStorage(ctx)
	if err == nil {
		clog.FromContext(ctx).Debug("Using kernel keyring storage for secrets")
		storage = keyringStorage
	}

	// .. but fall back to memory storage if not available
	if storage == nil {
		if runtime.GOOS == "linux" {
			clog.FromContext(ctx).Debugf("Kernel keyring not available, using memory storage: %v", err)
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
		ctx:          ctx,
	}

	return s, nil
}

// loggerInterceptor ius a grpx unary interceptor that injects the server's
// logger into each request context
func (s *Server) loggerInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	// Inject the logger from the server's context into the request context
	return handler(clog.WithLogger(ctx, clog.FromContext(s.ctx)), req)
}

// Run starts the server and blocks until shutdown
func (s *Server) Run(ctx context.Context) error {
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

	clog.FromContext(ctx).Debugf("Server listening on %s", s.options.SocketPath)
	clog.FromContext(ctx).Debugf("Session ID: %s", s.sessionID)

	// Create gRPC server with custom credentials to extract peer info and logger interceptor
	s.grpcServer = grpc.NewServer(
		grpc.Creds(NewPeerCredentials()),
		grpc.UnaryInterceptor(s.loggerInterceptor),
	)
	common.RegisterBurnAfterServer(s.grpcServer, s)

	// Start cleanup goroutine
	go s.cleanupExpiredSecrets()

	// Start inactivity monitor
	if s.options.InactivityTimeout > 0 {
		s.inactivityTimer = time.AfterFunc(s.options.InactivityTimeout, func() {
			clog.FromContext(ctx).Info("Inactivity timeout reached, shutting down")
			s.grpcServer.GracefulStop()
			close(s.shutdownChan)
		})
	}

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
					clog.FromContext(s.ctx).Debugf("Removing expired secret '%s' (reason: %s)", name, reason)
					delete(s.secrets, name)
					// Also delete from the storage backend
					_ = s.storage.Delete(s.ctx, name) //nolint:errcheck
				}
			}

			// Check if all secrets have been removed
			secretCount := len(s.secrets)
			s.secretsMu.Unlock()

			// If no secrets remain, shutdown the server
			if secretCount == 0 && s.grpcServer != nil {
				clog.FromContext(s.ctx).Debug("No secrets remaining, shutting down server")
				s.grpcServer.GracefulStop()
				close(s.shutdownChan)
				return
			}
		case <-s.shutdownChan:
			return
		}
	}
}
