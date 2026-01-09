// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package secrets

import (
	"bytes"
	"context"
	"testing"

	"github.com/carabiner-dev/burnafter/secrets"
)

func TestKeyringStorageStoreAndGet(t *testing.T) {
	storage, err := NewKeyringStorage()
	if err != nil {
		t.Skipf("Skipping keyring test: %v", err)
	}

	ctx := context.Background()

	// Create test payload
	payload := &secrets.Payload{
		EncryptedData:    []byte("encrypted-test-data"),
		Salt:             []byte("test-salt"),
		ClientBinaryHash: "test-hash",
	}

	// Store the secret
	err = storage.Store(ctx, "test-secret", payload)
	if err != nil {
		t.Fatalf("Failed to store secret: %v", err)
	}

	// Retrieve the secret
	retrieved, err := storage.Get(ctx, "test-secret")
	if err != nil {
		t.Fatalf("Failed to get secret: %v", err)
	}

	// Verify the data matches
	if !bytes.Equal(retrieved.EncryptedData, payload.EncryptedData) {
		t.Errorf("EncryptedData mismatch: got %s, want %s", retrieved.EncryptedData, payload.EncryptedData)
	}
	if !bytes.Equal(retrieved.Salt, payload.Salt) {
		t.Errorf("Salt mismatch: got %s, want %s", retrieved.Salt, payload.Salt)
	}
	if retrieved.ClientBinaryHash != payload.ClientBinaryHash {
		t.Errorf("ClientBinaryHash mismatch: got %s, want %s", retrieved.ClientBinaryHash, payload.ClientBinaryHash)
	}

	// Clean up
	err = storage.Delete(ctx, "test-secret")
	if err != nil {
		t.Fatalf("Failed to delete secret: %v", err)
	}
}

func TestKeyringStorageDelete(t *testing.T) {
	storage, err := NewKeyringStorage()
	if err != nil {
		t.Skipf("Skipping keyring test: %v", err)
	}

	ctx := context.Background()

	// Create and store test payload
	payload := &secrets.Payload{
		EncryptedData:    []byte("encrypted-test-data"),
		Salt:             []byte("test-salt"),
		ClientBinaryHash: "test-hash",
	}

	err = storage.Store(ctx, "test-delete", payload)
	if err != nil {
		t.Fatalf("Failed to store secret: %v", err)
	}

	// Delete the secret
	err = storage.Delete(ctx, "test-delete")
	if err != nil {
		t.Fatalf("Failed to delete secret: %v", err)
	}

	// Verify it's gone
	_, err = storage.Get(ctx, "test-delete")
	if err == nil {
		t.Error("Expected error when getting deleted secret, got nil")
	}
}

func TestKeyringStorageOverwrite(t *testing.T) {
	storage, err := NewKeyringStorage()
	if err != nil {
		t.Skipf("Skipping keyring test: %v", err)
	}

	ctx := context.Background()

	// Store first version
	payload1 := &secrets.Payload{
		EncryptedData:    []byte("version-1"),
		Salt:             []byte("salt-1"),
		ClientBinaryHash: "hash-1",
	}

	err = storage.Store(ctx, "test-overwrite", payload1)
	if err != nil {
		t.Fatalf("Failed to store secret v1: %v", err)
	}

	// Overwrite with second version
	payload2 := &secrets.Payload{
		EncryptedData:    []byte("version-2"),
		Salt:             []byte("salt-2"),
		ClientBinaryHash: "hash-2",
	}

	err = storage.Store(ctx, "test-overwrite", payload2)
	if err != nil {
		t.Fatalf("Failed to store secret v2: %v", err)
	}

	// Retrieve and verify it's the second version
	retrieved, err := storage.Get(ctx, "test-overwrite")
	if err != nil {
		t.Fatalf("Failed to get secret: %v", err)
	}

	if string(retrieved.EncryptedData) != "version-2" {
		t.Errorf("Expected version-2, got %s", retrieved.EncryptedData)
	}

	// Clean up
	_ = storage.Delete(ctx, "test-overwrite") //nolint:errcheck
}

func TestKeyringStorageGetNonExistent(t *testing.T) {
	storage, err := NewKeyringStorage()
	if err != nil {
		t.Skipf("Skipping keyring test: %v", err)
	}

	ctx := context.Background()

	// Try to get a non-existent key
	_, err = storage.Get(ctx, "non-existent-key")
	if err == nil {
		t.Error("Expected error when getting non-existent key, got nil")
	}
}
