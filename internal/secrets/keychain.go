// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build darwin && cgo

package secrets

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"github.com/chainguard-dev/clog"
	"github.com/keybase/go-keychain"

	"github.com/carabiner-dev/burnafter/secrets"
)

// Ensure the driver implements the storage interface
var _ secrets.Storage = &KeychainStorage{}

const (
	// keychainService is the service name used for all burnafter keychain items
	keychainService = "dev.carabiner.burnafter"
)

// KeychainStorage is a macOS Keychain implementation of the secrets.Storage
// interface. It stores encrypted secrets in the user's default keychain.
//
// Unlike the Linux keyring which requires a worker goroutine, macOS Keychain
// is thread-safe and can be accessed directly from any thread.
type KeychainStorage struct{}

// NewKeychainStorage creates a new macOS Keychain storage backend.
// The macOS Keychain is always available on macOS systems, so this function
// simply returns a new instance. Individual operations will fail if the
// keychain is locked or inaccessible.
func NewKeychainStorage(ctx context.Context) (*KeychainStorage, error) {
	clog.FromContext(ctx).Debug("macOS Keychain storage initialized")
	return &KeychainStorage{}, nil
}

// Store persists a secret in the macOS Keychain.
func (k *KeychainStorage) Store(ctx context.Context, id string, secret *secrets.Payload) error {
	clog.FromContext(ctx).Debugf("Storing secret %s in keychain", id)

	// Serialize the payload to bytes using gob encoding
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(secret); err != nil {
		return fmt.Errorf("failed to encode the secret payload: %w", err)
	}

	// Delete existing item if it exists (keychain will error if we try to add duplicate)
	_ = k.Delete(ctx, id) // Ignore error - item might not exist

	// Create a new keychain item
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(keychainService)
	item.SetAccount(id)
	item.SetData(buf.Bytes())
	item.SetLabel(fmt.Sprintf("burnafter secret: %s", id))
	item.SetDescription("Encrypted ephemeral secret managed by burnafter")
	item.SetAccessible(keychain.AccessibleWhenUnlocked)

	// Add to keychain
	if err := keychain.AddItem(item); err != nil {
		return fmt.Errorf("failed to add item to keychain: %w", err)
	}

	clog.FromContext(ctx).Debugf("Successfully stored secret %s in keychain", id)
	return nil
}

// Get retrieves a secret from the macOS Keychain by its ID.
func (k *KeychainStorage) Get(ctx context.Context, id string) (*secrets.Payload, error) {
	clog.FromContext(ctx).Debugf("Getting secret %s from keychain", id)

	// Query the keychain
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(keychainService)
	query.SetAccount(id)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)

	results, err := keychain.QueryItem(query)
	if err == keychain.ErrorItemNotFound {
		return nil, fmt.Errorf("secret %s not found in keychain", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query keychain: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("secret %s not found in keychain", id)
	}

	// Get the data from the first (and only) result
	data := results[0].Data

	// Deserialize the payload
	var payload secrets.Payload
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding secret payload: %w", err)
	}

	clog.FromContext(ctx).Debugf("Successfully retrieved secret %s from keychain", id)
	return &payload, nil
}

// Delete removes a secret from the macOS Keychain by its ID.
func (k *KeychainStorage) Delete(ctx context.Context, id string) error {
	clog.FromContext(ctx).Debugf("Deleting secret %s from keychain", id)

	// Create delete query
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(keychainService)
	item.SetAccount(id)

	// Delete the item
	err := keychain.DeleteItem(item)
	if err == keychain.ErrorItemNotFound {
		// Item not found is not an error - it's already deleted
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete item from keychain: %w", err)
	}

	clog.FromContext(ctx).Debugf("Successfully deleted secret %s from keychain", id)
	return nil
}
