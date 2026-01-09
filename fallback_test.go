// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carabiner-dev/burnafter/options"
)

func TestFallbackStore(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-fallback-store"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	secretName := "test-secret-ksldjhf"
	secretValue := "my-secret-value"

	err := client.Store(ctx, secretName, secretValue, options.WithTTL(300))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify file was created
	filePath, err := client.getFallbackFilePath(secretName)
	if err != nil {
		t.Fatalf("getFallbackFilePath failed: %v", err)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Expected fallback file to exist at %s", filePath)
	}

	// Cleanup
	defer os.Remove(filePath) //nolint:errcheck
}

func TestFallbackGet(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-fallback-get"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	secretName := "test-secret-get"
	secretValue := "my-secret-value-get"

	// Store secret
	err := client.Store(ctx, secretName, secretValue, options.WithTTL(300))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Get secret
	retrieved, err := client.Get(ctx, secretName)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved != secretValue {
		t.Errorf("Expected %q, got %q", secretValue, retrieved)
	}

	// Cleanup
	filePath, _ := client.getFallbackFilePath(secretName) //nolint:errcheck
	defer os.Remove(filePath)                             //nolint:errcheck
}

func TestFallbackDelete(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-fallback-delete"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	secretName := "test-secret-delete"
	secretValue := "my-secret-value-delete"

	// Store secret
	err := client.Store(ctx, secretName, secretValue, options.WithTTL(300))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	filePath, _ := client.getFallbackFilePath(secretName) //nolint:errcheck

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Expected file to exist before delete")
	}

	// Delete secret
	err = client.Delete(ctx, secretName)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("Expected file to be deleted")
	}
}

func TestFallbackExpiry(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-fallback-expiry"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	secretName := "test-secret-expiry"
	secretValue := "expiring-value" //nolint:gosec

	// Store with 2 second TTL
	err := client.Store(ctx, secretName, secretValue, options.WithTTL(2))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Should work immediately
	retrieved, err := client.Get(ctx, secretName)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved != secretValue {
		t.Errorf("Expected %q, got %q", secretValue, retrieved)
	}

	// Wait for expiry
	time.Sleep(3 * time.Second)

	// Should fail now
	_, err = client.Get(ctx, secretName)
	if err == nil {
		t.Errorf("Expected error for expired secret, got none")
	}

	// Cleanup (file should be auto-deleted, but just in case)
	filePath, _ := client.getFallbackFilePath(secretName) //nolint:errcheck
	defer os.Remove(filePath)                             //nolint:errcheck
}

func TestFallbackMultipleSecrets(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-fallback-multiple"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	secrets := map[string]string{
		"secret1": "value1",
		"secret2": "value2",
		"secret3": "value3",
	}

	// Store multiple secrets
	for name, value := range secrets {
		err := client.Store(ctx, name, value, options.WithTTL(300))
		if err != nil {
			t.Fatalf("Store failed for %s: %v", name, err)
		}
	}

	// Retrieve and verify all
	for name, expectedValue := range secrets {
		retrieved, err := client.Get(ctx, name)
		if err != nil {
			t.Fatalf("Get failed for %s: %v", name, err)
		}
		if retrieved != expectedValue {
			t.Errorf("For %s: expected %q, got %q", name, expectedValue, retrieved)
		}
	}

	// Cleanup
	for name := range secrets {
		filePath, _ := client.getFallbackFilePath(name) //nolint:errcheck
		defer os.Remove(filePath)                       //nolint:errcheck
	}
}

func TestFallbackDeterministicPath(t *testing.T) {
	opts1 := options.DefaultClient
	opts1.NoServer = true
	opts1.Nonce = "test-nonce-deterministic"

	opts2 := options.DefaultClient
	opts2.NoServer = true
	opts2.Nonce = "test-nonce-deterministic"

	client1 := NewClient(opts1)
	client2 := NewClient(opts2)

	secretName := "test-secret-path"

	path1, err1 := client1.getFallbackFilePath(secretName)
	path2, err2 := client2.getFallbackFilePath(secretName)

	if err1 != nil || err2 != nil {
		t.Fatalf("getFallbackFilePath failed: %v, %v", err1, err2)
	}

	if path1 != path2 {
		t.Errorf("Expected same path, got %s and %s", path1, path2)
	}
}

func TestFallbackDifferentNonceSamePath(t *testing.T) {
	// File paths should be the same for same secret name, regardless of nonce
	// Nonce affects encryption key, not file path
	opts1 := &options.Client{
		Nonce:    "test-nonce-1",
		NoServer: true,
		Common:   options.DefaultClient.Common,
	}

	opts2 := &options.Client{
		Nonce:    "test-nonce-2",
		NoServer: true,
		Common:   options.DefaultClient.Common,
	}

	client1 := NewClient(opts1)
	client2 := NewClient(opts2)

	secretName := "test-secret-9834jhksdjk21"

	path1, err1 := client1.getFallbackFilePath(secretName)
	path2, err2 := client2.getFallbackFilePath(secretName)

	if err1 != nil || err2 != nil {
		t.Fatalf("getFallbackFilePath failed: %v, %v", err1, err2)
	}

	// Paths should be the same (based on binary hash + secret name)
	if path1 != path2 {
		t.Errorf("Expected same paths for same secret name, got %s and %s", path1, path2)
	}
}

func TestFallbackCrossClientRetrieval(t *testing.T) {
	nonce := "test-nonce-cross-client"

	// Client 1 stores secret
	opts1 := options.DefaultClient
	opts1.NoServer = true
	opts1.Nonce = nonce

	client1 := NewClient(opts1)
	ctx := context.Background()

	if err := client1.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	secretName := "cross-client-secret"
	secretValue := "cross-client-value"

	err := client1.Store(ctx, secretName, secretValue, options.WithTTL(300))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Client 2 (same nonce) retrieves secret
	opts2 := options.DefaultClient
	opts2.NoServer = true
	opts2.Nonce = nonce

	client2 := NewClient(opts2)

	if err := client2.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	retrieved, err := client2.Get(ctx, secretName)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved != secretValue {
		t.Errorf("Expected %q, got %q", secretValue, retrieved)
	}

	// Cleanup
	filePath, _ := client1.getFallbackFilePath(secretName) //nolint:errcheck
	defer os.Remove(filePath)                              //nolint:errcheck
}

func TestFallbackPing(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-ping"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Ping should always succeed in fallback mode
	err := client.Ping(ctx)
	if err != nil {
		t.Errorf("Ping failed in fallback mode: %v", err)
	}
}

func TestFallbackCleanupExpiredFiles(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-cleanup"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Store secret with 1 second TTL
	secretName := "cleanup-test"
	err := client.Store(ctx, secretName, "value", options.WithTTL(1))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	filePath, _ := client.getFallbackFilePath(secretName) //nolint:errcheck

	// File should exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Expected file to exist")
	}

	// Wait for expiry
	time.Sleep(2 * time.Second)

	// Trigger cleanup by storing another secret
	_ = client.Store(ctx, "trigger-cleanup", "value", options.WithTTL(60)) //nolint:errcheck
	defer func() {
		path, _ := client.getFallbackFilePath("trigger-cleanup") //nolint:errcheck
		os.Remove(path)                                          //nolint:errcheck,gosec
	}()

	// Original file should be cleaned up
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("Expected expired file to be cleaned up")
	}
}

func TestFallbackGetNonExistent(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-nonexistent"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	_, err := client.Get(ctx, "does-not-exist")
	if err == nil {
		t.Errorf("Expected error for non-existent secret")
	}
}

func TestFallbackDeleteNonExistent(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-delete-nonexistent"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	err := client.Delete(ctx, "does-not-exist")
	if err == nil {
		t.Errorf("Expected error when deleting non-existent secret")
	}
}

func TestFallbackAbsoluteExpiration(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-absolute-exp"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	secretName := "absolute-exp-test" //nolint:gosec
	// Set absolute expiration 2 seconds from now
	absoluteExp := time.Now().Add(2 * time.Second).Unix()

	err := client.Store(ctx, secretName, "value", options.WithAbsoluteExpiration(absoluteExp))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Should work immediately
	_, err = client.Get(ctx, secretName)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(3 * time.Second)

	// Should fail now
	_, err = client.Get(ctx, secretName)
	if err == nil {
		t.Errorf("Expected error for expired secret")
	}

	// Cleanup
	filePath, _ := client.getFallbackFilePath(secretName) //nolint:errcheck
	defer os.Remove(filePath)                             //nolint:errcheck
}

func TestFallbackFilePermissions(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-permissions"

	client := NewClient(opts)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	secretName := "permissions-test"
	err := client.Store(ctx, secretName, "value", options.WithTTL(300))
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	filePath, _ := client.getFallbackFilePath(secretName) //nolint:errcheck
	defer os.Remove(filePath)                             //nolint:errcheck

	// Check file permissions
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	mode := info.Mode().Perm()
	expected := os.FileMode(0o600)

	if mode != expected {
		t.Errorf("Expected permissions %o, got %o", expected, mode)
	}
}

func TestFallbackFilePathFormat(t *testing.T) {
	opts := options.DefaultClient
	opts.NoServer = true
	opts.Nonce = "test-nonce-otro"

	client := NewClient(opts)

	secretName := "test-secret"
	filePath, err := client.getFallbackFilePath(secretName)
	if err != nil {
		t.Fatalf("getFallbackFilePath failed: %v", err)
	}

	// Verify path is in tmp directory
	tmpDir := os.TempDir()
	if filepath.Dir(filePath) != tmpDir {
		t.Errorf("Expected path in %s, got %s", tmpDir, filePath)
	}

	// Verify filename has correct format (burnafter-{hash}-{hash})
	filename := filepath.Base(filePath)
	if len(filename) < len("burnafter--") {
		t.Errorf("Filename too short: %s", filename)
	}

	// Should not have extension
	ext := filepath.Ext(filename)
	if ext != "" {
		t.Errorf("Expected no file extension, got %s", ext)
	}
}
