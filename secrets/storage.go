// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

// Package secrets exposes the public interface for secret storage implementations
// the most basic of these is the memory driver that burnafter uses when it cannot
// access more secure system managed storage.
package secrets

import (
	"context"
	"time"
)

// Payload represents the actual secret data stored in the backend.
// This contains the encrypted data, cryptographic material, and
// client authentication info.
type Payload struct {
	EncryptedData    []byte // Encrypted secret data
	Salt             []byte // Salt used for key derivation
	ClientBinaryHash string // Hash of the client binary that stored it
}

// Metadata represents the metadata about a secret that the server
// keeps in memory for lifecycle management. This includes expiration information
// and access timestamps.
type Metadata struct {
	Name              string        // Name of the secret
	InactivityTTL     time.Duration // TTL for inactivity-based expiration
	AbsoluteExpiresAt *time.Time    // Optional absolute expiration time (nil = no absolute expiration)
	LastAccessed      time.Time     // Last time this secret was accessed
}

// Storage defines the interface for storing and retrieving encrypted secrets.
// Implementations of this interface handle the actual persistence of secret
// data, while the server manages lifecycle, access control, and expiration.
type Storage interface {
	// Store persists a secret with the given name to the storage backend.
	Store(context.Context, string, *Payload) error

	// Get retrieves a secret
	Get(context.Context, string) (*Payload, error)

	// Delete removes a secret from storage
	Delete(context.Context, string) error
}

// TODO(puerco): Sooner or later we'll need a List()
