// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/carabiner-dev/burnafter/options"
)

func TestDeriveKey(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce"
	opts.NoServer = true

	client := NewClient(opts)

	secretName := "test-secret"

	// Derive key
	key, err := client.deriveKey(secretName)
	if err != nil {
		t.Fatalf("deriveKey failed: %v", err)
	}

	// Key should be 32 bytes (AES-256)
	if len(key) != 32 {
		t.Errorf("Expected key length of 32, got %d", len(key))
	}

	// Deriving again should give same key (deterministic)
	key2, err := client.deriveKey(secretName)
	if err != nil {
		t.Fatalf("deriveKey failed on second call: %v", err)
	}

	if !bytes.Equal(key, key2) {
		t.Errorf("Expected same key on repeated derivation")
	}
}

func TestDeriveKeyDifferentSecrets(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce"
	opts.NoServer = true

	client := NewClient(opts)

	key1, err := client.deriveKey("secret1")
	if err != nil {
		t.Fatalf("deriveKey failed: %v", err)
	}

	key2, err := client.deriveKey("secret2")
	if err != nil {
		t.Fatalf("deriveKey failed: %v", err)
	}

	// Keys should be different for different secrets
	if bytes.Equal(key1, key2) {
		t.Errorf("Expected different keys for different secrets")
	}
}

func TestDeriveKeyDifferentNonces(t *testing.T) {
	opts1 := &options.Client{
		Nonce:    "nonce1",
		NoServer: true,
		Common:   options.DefaultClient.Common,
	}

	opts2 := &options.Client{
		Nonce:    "nonce2",
		NoServer: true,
		Common:   options.DefaultClient.Common,
	}

	client1 := NewClient(opts1)
	client2 := NewClient(opts2)

	secretName := "same-secret"

	key1, err := client1.deriveKey(secretName)
	if err != nil {
		t.Fatalf("deriveKey failed for client1: %v", err)
	}

	key2, err := client2.deriveKey(secretName)
	if err != nil {
		t.Fatalf("deriveKey failed for client2: %v", err)
	}

	// Keys should be different for different nonces
	if bytes.Equal(key1, key2) {
		t.Errorf("Expected different keys for different nonces")
	}
}

func TestEncryptDecryptSecret(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce-crypto"
	opts.NoServer = true

	client := NewClient(opts)

	secretName := "crypto-test"
	secretValue := []byte("my-secret-value")
	expiryTime := time.Now().Add(1 * time.Hour)

	// Encrypt
	err := client.encryptSecret(secretName, secretValue, expiryTime)
	if err != nil {
		t.Fatalf("encryptSecret failed: %v", err)
	}

	// Decrypt
	decrypted, err := client.decryptSecret(secretName)
	if err != nil {
		t.Fatalf("decryptSecret failed: %v", err)
	}

	if !bytes.Equal(secretValue, decrypted) {
		t.Errorf("Expected %s, got %s", secretValue, decrypted)
	}

	// Cleanup
	defer client.deleteFallbackSecret(secretName) //nolint:errcheck
}

func TestEncryptDecryptEmptySecret(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce-empty"
	opts.NoServer = true

	client := NewClient(opts)

	secretName := "empty-secret"
	secretValue := []byte("")
	expiryTime := time.Now().Add(1 * time.Hour)

	// Encrypt empty secret
	err := client.encryptSecret(secretName, secretValue, expiryTime)
	if err != nil {
		t.Fatalf("encryptSecret failed: %v", err)
	}

	// Decrypt
	decrypted, err := client.decryptSecret(secretName)
	if err != nil {
		t.Fatalf("decryptSecret failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("Expected empty secret, got %d bytes", len(decrypted))
	}

	// Cleanup
	defer client.deleteFallbackSecret(secretName) //nolint:errcheck
}

func TestEncryptDecryptLargeSecret(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce-large"
	opts.NoServer = true

	client := NewClient(opts)

	secretName := "large-secret"
	// Create 10KB secret
	secretValue := make([]byte, 10*1024)
	for i := range secretValue {
		secretValue[i] = byte(i % 256)
	}
	expiryTime := time.Now().Add(1 * time.Hour)

	// Encrypt
	err := client.encryptSecret(secretName, secretValue, expiryTime)
	if err != nil {
		t.Fatalf("encryptSecret failed: %v", err)
	}

	// Decrypt
	decrypted, err := client.decryptSecret(secretName)
	if err != nil {
		t.Fatalf("decryptSecret failed: %v", err)
	}

	if !bytes.Equal(secretValue, decrypted) {
		t.Errorf("Large secret decryption mismatch")
	}

	// Cleanup
	defer client.deleteFallbackSecret(secretName) //nolint:errcheck
}

func TestDecryptExpiredSecret(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce-expired"
	opts.NoServer = true

	client := NewClient(opts)

	secretName := "expired-secret"
	secretValue := []byte("will-expire")
	// Set expiry in the past
	expiryTime := time.Now().Add(-1 * time.Hour)

	// Encrypt
	err := client.encryptSecret(secretName, secretValue, expiryTime)
	if err != nil {
		t.Fatalf("encryptSecret failed: %v", err)
	}

	// Try to decrypt - should fail
	_, err = client.decryptSecret(secretName)
	if err == nil {
		t.Errorf("Expected error when decrypting expired secret")
	}

	// File should be auto-deleted, clean up just in case
	defer client.deleteFallbackSecret(secretName) //nolint:errcheck
}

func TestDecryptNonExistent(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce-nonexistent"
	opts.NoServer = true

	client := NewClient(opts)

	_, err := client.decryptSecret("does-not-exist")
	if err == nil {
		t.Errorf("Expected error when decrypting non-existent secret")
	}
}

func TestEncryptSecretTwiceOverwrites(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce-overwrite"
	opts.NoServer = true

	client := NewClient(opts)

	secretName := "overwrite-test"
	value1 := []byte("first-value")
	value2 := []byte("second-value")
	expiryTime := time.Now().Add(1 * time.Hour)

	// Encrypt first value
	err := client.encryptSecret(secretName, value1, expiryTime)
	if err != nil {
		t.Fatalf("encryptSecret failed: %v", err)
	}

	// Encrypt second value (overwrite)
	err = client.encryptSecret(secretName, value2, expiryTime)
	if err != nil {
		t.Fatalf("encryptSecret failed on overwrite: %v", err)
	}

	// Decrypt should get second value
	decrypted, err := client.decryptSecret(secretName)
	if err != nil {
		t.Fatalf("decryptSecret failed: %v", err)
	}

	if !bytes.Equal(value2, decrypted) {
		t.Errorf("Expected second value after overwrite, got something else")
	}

	// Cleanup
	defer client.deleteFallbackSecret(secretName) //nolint:errcheck
}

func TestEncryptedFileFormat(t *testing.T) {
	opts := options.DefaultClient
	opts.Nonce = "test-nonce-format"
	opts.NoServer = true

	client := NewClient(opts)

	secretName := "format-test"
	secretValue := []byte("test-value")
	expiryTime := time.Now().Add(1 * time.Hour)

	// Encrypt
	err := client.encryptSecret(secretName, secretValue, expiryTime)
	if err != nil {
		t.Fatalf("encryptSecret failed: %v", err)
	}

	// Read raw file
	filePath, _ := client.getFallbackFilePath(secretName) //nolint:errcheck
	defer client.deleteFallbackSecret(secretName)         //nolint:errcheck

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Check minimum size: version(1) + nonce(12) + expiry(8) + ciphertext+tag(>=16)
	minSize := 1 + 12 + 8 + 16
	if len(data) < minSize {
		t.Errorf("Encrypted file too small: %d bytes (expected at least %d)", len(data), minSize)
	}

	// Check version
	version := data[0]
	if version != 1 {
		t.Errorf("Expected version 1, got %d", version)
	}
}

func TestUseFallback(t *testing.T) {
	// Test NoServer option
	opts1 := options.DefaultClient
	opts1.NoServer = true

	client1 := NewClient(opts1)
	if !client1.useFallback() {
		t.Errorf("Expected useFallback to be true when NoServer is set")
	}

	// Test serverStartFailed flag
	opts2 := options.DefaultClient
	opts2.NoServer = false

	client2 := NewClient(opts2)
	client2.serverStartFailed = true

	if !client2.useFallback() {
		t.Errorf("Expected useFallback to be true when serverStartFailed is set")
	}

	// Test neither flag set
	opts3 := options.DefaultClient
	opts3.NoServer = false

	client3 := NewClient(opts3)
	if client3.useFallback() {
		t.Errorf("Expected useFallback to be false when neither flag is set")
	}
}
