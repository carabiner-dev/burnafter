// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package secrets

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/carabiner-dev/burnafter/secrets"
)

// Ensure the driver implements the storage interface
var _ secrets.Storage = &KeyringStorage{}

// KeyringStorage is a Linux kernel keyring implementation of the
// secrets.Storage interface. It stores encrypted secrets in the process keyring
type KeyringStorage struct{}

// NewKeyringStorage creates a new kernel keyring storage backend.
// It uses the process keyring (KEY_SPEC_PROCESS_KEYRING) which is
// isolated per-process.
func NewKeyringStorage() (*KeyringStorage, error) {
	// Request the process keyring, creating it if it doesn't exist
	// The second parameter (true) tells the kernel to create it if needed
	_, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_PROCESS_KEYRING, true)
	if err != nil {
		return nil, fmt.Errorf("failed to access/create process keyring: %w", err)
	}

	return &KeyringStorage{}, nil
}

// Store persists a secret in the kernel keyring.
func (k *KeyringStorage) Store(ctx context.Context, id string, secret *secrets.Payload) error {
	// Serialize the payload to bytes using gob encoding
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(secret); err != nil {
		return fmt.Errorf("failed to encode the secret payload: %w", err)
	}

	// Check if key already exists and delete it first
	// This ensures we're always creating a fresh key
	if existingKeyID, err := unix.KeyctlSearch(unix.KEY_SPEC_PROCESS_KEYRING, "user", id, 0); err == nil {
		//nolint:errcheck // Don't err if key can't be removed it will be overwritten anyway.
		_, _ = unix.KeyctlInt(unix.KEYCTL_UNLINK, existingKeyID, unix.KEY_SPEC_PROCESS_KEYRING, 0, 0)
	}

	// Add the key to the process keyring
	keyID, err := unix.AddKey("user", id, buf.Bytes(), unix.KEY_SPEC_PROCESS_KEYRING)
	if err != nil {
		return fmt.Errorf("adding key to keyring: %w", err)
	}

	// Set a permission that only allows the owner to access
	err = unix.KeyctlSetperm(keyID, 0x3f000000) // Owner: all permissions
	if err != nil {
		return fmt.Errorf("setting key permissions: %w", err)
	}

	return nil
}

// Get retrieves a secret from the kernel keyring by its ID.
func (k *KeyringStorage) Get(ctx context.Context, id string) (*secrets.Payload, error) {
	// Search for the key by description
	keyID, err := unix.KeyctlSearch(unix.KEY_SPEC_PROCESS_KEYRING, "user", id, 0)
	if err != nil {
		return nil, fmt.Errorf("looking up secret: %w", err)
	}

	// First, get the size of the key data
	size, err := unix.KeyctlBuffer(unix.KEYCTL_READ, keyID, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("gettting key size: %w", err)
	}

	// Allocate buffer and read the key data
	buf := make([]byte, size)
	_, err = unix.KeyctlBuffer(unix.KEYCTL_READ, keyID, buf, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read key from keyring: %w", err)
	}

	// Deserialize the payload
	var payload secrets.Payload
	dec := gob.NewDecoder(bytes.NewReader(buf))
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding secret payload: %w", err)
	}

	return &payload, nil
}

// Delete removes a secret from the kernel keyring by its ID.
func (k *KeyringStorage) Delete(ctx context.Context, id string) error {
	// Search for the key by its ID
	keyID, err := unix.KeyctlSearch(unix.KEY_SPEC_PROCESS_KEYRING, "user", id, 0)
	if err != nil {
		//nolint:nilerr // Key not found is not an error
		return nil
	}

	// Unlink the key from the keyring
	_, err = unix.KeyctlInt(unix.KEYCTL_UNLINK, keyID, unix.KEY_SPEC_PROCESS_KEYRING, 0, 0)
	if err != nil {
		return fmt.Errorf("unlinking key from keyring: %w", err)
	}

	return nil
}
