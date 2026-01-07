// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/carabiner-dev/burnafter/internal/common"
	pb "github.com/carabiner-dev/burnafter/internal/common"
)

// Get implements the Get RPC by handling the full get lifecycle:
// getting the client fingerprint, deriving the secret's encryption
// key, decrpypting the secret and sending it back./
func (s *Server) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	s.updateActivity()

	if s.options.Debug {
		log.Printf("Get request for secret: %s", req.Name)
	}

	// Get client PID and verify binary
	authInfo, err := GetPeerAuthInfo(ctx)
	if err != nil {
		return &pb.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to get client credentials: %v", err),
		}, nil
	}

	_, clientHash, err := common.GetClientBinaryInfo(authInfo.PID)
	if err != nil {
		return &pb.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to verify client binary: %v", err),
		}, nil
	}

	// Retrieve the secret
	s.secretsMu.Lock()
	stored, exists := s.secrets[req.Name]
	if !exists {
		s.secretsMu.Unlock()
		return &pb.GetResponse{
			Success: false,
			Error:   "secret not found",
		}, nil
	}

	now := time.Now()

	// Check if secret expired due to inactivity
	if time.Since(stored.LastAccessed) > stored.InactivityTTL {
		delete(s.secrets, req.Name)
		s.secretsMu.Unlock()
		return &pb.GetResponse{
			Success: false,
			Error:   "secret has expired due to inactivity",
		}, nil
	}

	// Check if secret has expired due to absolute expiration
	if stored.AbsoluteExpiresAt != nil && now.After(*stored.AbsoluteExpiresAt) {
		delete(s.secrets, req.Name)
		s.secretsMu.Unlock()
		return &pb.GetResponse{
			Success: false,
			Error:   "secret has expired (absolute deadline reached)",
		}, nil
	}

	// Verify tjat client binary hash matches
	if stored.ClientBinaryHash != clientHash {
		s.secretsMu.Unlock()
		return &pb.GetResponse{
			Success: false,
			Error:   "client binary hash mismatch - unauthorized",
		}, nil
	}

	// Verify client nonce matches
	if stored.ClientNonce != req.ClientNonce {
		s.secretsMu.Unlock()
		return &pb.GetResponse{
			Success: false,
			Error:   "client nonce mismatch - unauthorized",
		}, nil
	}

	// Update last accessed time
	stored.LastAccessed = time.Now()
	s.secretsMu.Unlock()

	// Derive the key again
	key, err := common.DeriveKey(clientHash, req.ClientNonce, s.sessionID, req.Name, stored.Salt)
	if err != nil {
		return &pb.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to derive key: %v", err),
		}, nil
	}
	defer common.ZeroBytes(key)

	// Decrypt the secret
	plaintext, err := common.Decrypt(stored.EncryptedData, key)
	if err != nil {
		return &pb.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to decrypt secret: %v", err),
		}, nil
	}

	if s.options.Debug {
		log.Printf("Retrieved secret '%s'", req.Name)
	}

	return &pb.GetResponse{
		Success: true,
		Secret:  plaintext,
	}, nil
}
