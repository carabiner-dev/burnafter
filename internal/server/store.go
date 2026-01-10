// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/chainguard-dev/clog"

	"github.com/carabiner-dev/burnafter/internal/common"
	"github.com/carabiner-dev/burnafter/secrets"
)

// Store implements the Store RPC. Takes a storage request to save aaa secret
// in the server's secret map. It handles getting the client finger print,
// deriving the key, encrypting the secret and storing it along with the
// required metadaata.
func (s *Server) Store(ctx context.Context, req *common.StoreRequest) (*common.StoreResponse, error) {
	s.updateActivity()

	clog.FromContext(ctx).Debugf("Store request for secret: %s", req.Name)

	// Get client PID and verify binary
	authInfo, err := GetPeerAuthInfo(ctx)
	if err != nil {
		return &common.StoreResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to get client credentials: %v", err),
		}, nil
	}

	// Read the client binary information. This includes the hash which will
	// be used to derive the encryption key.
	_, clientHash, err := common.GetClientBinaryInfo(authInfo.PID)
	if err != nil {
		return &common.StoreResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to verify client binary: %v", err),
		}, nil
	}

	// Debug the hash value
	clog.FromContext(ctx).Debugf("Client binary hash: %s", clientHash)

	// Check secret size limit
	secretSize := int64(len(req.Secret))
	if secretSize > s.options.MaxSecretSize {
		return &common.StoreResponse{
			Success: false,
			Error:   fmt.Sprintf("secret size (%d bytes) exceeds maximum allowed size (%d bytes)", secretSize, s.options.MaxSecretSize),
		}, nil
	}

	// Check maximum number of secrets limit (only if storing a new secret)
	s.secretsMu.RLock()
	_, exists := s.secrets[req.Name]
	currentCount := len(s.secrets)
	s.secretsMu.RUnlock()

	if !exists && currentCount >= s.options.MaxSecrets {
		return &common.StoreResponse{
			Success: false,
			Error:   fmt.Sprintf("maximum number of secrets (%d) reached", s.options.MaxSecrets),
		}, nil
	}

	// Generate salt for this secret
	salt, err := common.GenerateSalt()
	if err != nil {
		return &common.StoreResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to generate salt: %v", err),
		}, nil
	}

	// Derive the encryption key from the client hash, the client nonce, our
	// session ID and the secret name (plus the salt)
	key, err := common.DeriveKey(clientHash, req.ClientNonce, s.sessionID, req.Name, salt)
	if err != nil {
		return &common.StoreResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to derive key: %v", err),
		}, nil
	}
	// Wipe out the key from memory when we are done
	defer common.ZeroBytes(key)

	// Encrypt the secret
	encrypted, err := common.Encrypt(req.Secret, key)
	if err != nil {
		return &common.StoreResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to encrypt secret: %v", err),
		}, nil
	}

	// Calculate inactivity TTL
	ttl := time.Duration(req.TtlSeconds) * time.Second
	if ttl == 0 {
		ttl = s.options.DefaultTTL
	}

	// Calculate optional absolute expiration
	var absoluteExpiresAt *time.Time
	if req.AbsoluteExpirationSeconds > 0 {
		t := time.Now().Add(time.Duration(req.AbsoluteExpirationSeconds) * time.Second)
		absoluteExpiresAt = &t
	}

	// Create the stored secret with encrypted data
	stored := &secrets.Payload{
		EncryptedData:    encrypted,
		Salt:             salt,
		ClientBinaryHash: clientHash,
	}

	// Store the encrypted secret in the storage backend
	if err := s.storage.Store(ctx, req.Name, stored); err != nil {
		return &common.StoreResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to store secret in backend: %v", err),
		}, nil
	}

	// Store only metadata in server memory for lifecycle management
	now := time.Now()
	s.secretsMu.Lock()
	s.secrets[req.Name] = &secrets.Metadata{
		Name:              req.Name,
		InactivityTTL:     ttl,
		AbsoluteExpiresAt: absoluteExpiresAt,
		LastAccessed:      now,
	}
	s.secretsMu.Unlock()

	if absoluteExpiresAt != nil {
		clog.FromContext(ctx).Debugf("Stored secret '%s', inactivity TTL: %v, absolute expiration: %s",
			req.Name, ttl, absoluteExpiresAt.Format(time.RFC3339))
	} else {
		clog.FromContext(ctx).Debugf("Stored secret '%s', inactivity TTL: %v (no absolute expiration)",
			req.Name, ttl)
	}

	return &common.StoreResponse{Success: true}, nil
}
