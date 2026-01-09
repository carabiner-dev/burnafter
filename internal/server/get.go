// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/carabiner-dev/burnafter/internal/common"
)

// Get implements the Get RPC by handling the full get lifecycle:
// getting the client fingerprint, deriving the secret's encryption
// key, decrpypting the secret and sending it back./
func (s *Server) Get(ctx context.Context, req *common.GetRequest) (*common.GetResponse, error) {
	s.updateActivity()

	if s.options.Debug {
		log.Printf("Get request for secret: %s", req.Name)
	}

	// Get client PID and verify binary
	authInfo, err := GetPeerAuthInfo(ctx)
	if err != nil {
		return &common.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to get client credentials: %v", err),
		}, nil
	}

	_, clientHash, err := common.GetClientBinaryInfo(authInfo.PID)
	if err != nil {
		return &common.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to verify client binary: %v", err),
		}, nil
	}

	// Retrieve the secret metadata
	s.secretsMu.Lock()
	metadata, exists := s.secrets[req.Name]
	if !exists {
		s.secretsMu.Unlock()
		return &common.GetResponse{
			Success: false,
			Error:   "secret not found",
		}, nil
	}

	now := time.Now()

	// Check if secret expired due to inactivity
	if time.Since(metadata.LastAccessed) > metadata.InactivityTTL {
		delete(s.secrets, req.Name)
		s.secretsMu.Unlock()
		// Also delete from storage backend
		_ = s.storage.Delete(ctx, req.Name) //nolint:errcheck
		return &common.GetResponse{
			Success: false,
			Error:   "secret has expired due to inactivity",
		}, nil
	}

	// Check if secret has expired due to absolute expiration
	if metadata.AbsoluteExpiresAt != nil && now.After(*metadata.AbsoluteExpiresAt) {
		delete(s.secrets, req.Name)
		s.secretsMu.Unlock()
		// Also delete from storage backend
		_ = s.storage.Delete(ctx, req.Name) //nolint:errcheck
		return &common.GetResponse{
			Success: false,
			Error:   "secret has expired (absolute deadline reached)",
		}, nil
	}

	// Update last accessed time
	metadata.LastAccessed = time.Now()
	s.secretsMu.Unlock()

	// Retrieve the actual secret from storage backend
	stored, err := s.storage.Get(ctx, req.Name)
	if err != nil {
		return &common.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to retrieve secret from storage: %v", err),
		}, nil
	}

	// Verify that client binary hash matches
	if stored.ClientBinaryHash != clientHash {
		return &common.GetResponse{
			Success: false,
			Error:   "client binary hash mismatch - unauthorized",
		}, nil
	}

	// Derive the key again
	key, err := common.DeriveKey(clientHash, req.ClientNonce, s.sessionID, req.Name, stored.Salt)
	if err != nil {
		return &common.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to derive key: %v", err),
		}, nil
	}
	defer common.ZeroBytes(key)

	// Decrypt the secret
	plaintext, err := common.Decrypt(stored.EncryptedData, key)
	if err != nil {
		return &common.GetResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to decrypt secret: %v", err),
		}, nil
	}

	if s.options.Debug {
		log.Printf("Retrieved secret '%s'", req.Name)
	}

	return &common.GetResponse{
		Success: true,
		Secret:  plaintext,
	}, nil
}
