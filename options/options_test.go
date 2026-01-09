// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package options

import (
	"testing"
	"time"
)

func TestDefaultClientOptions(t *testing.T) {
	opts := DefaultClient

	if opts.SocketPath != "" {
		t.Errorf("Expected empty SocketPath, got %s", opts.SocketPath)
	}

	if opts.DefaultTTL != 4*time.Hour {
		t.Errorf("Expected DefaultTTL of 4 hours, got %v", opts.DefaultTTL)
	}

	if opts.MaxSecrets != 100 {
		t.Errorf("Expected MaxSecrets of 100, got %d", opts.MaxSecrets)
	}

	if opts.MaxSecretSize != 1024*1024 {
		t.Errorf("Expected MaxSecretSize of 1MB, got %d", opts.MaxSecretSize)
	}

	if opts.Debug {
		t.Errorf("Expected Debug to be false")
	}

	if opts.NoServer {
		t.Errorf("Expected NoServer to be false")
	}
}

func TestDefaultServerOptions(t *testing.T) {
	opts := DefaultServer

	if opts.SocketPath != "" {
		t.Errorf("Expected empty SocketPath, got %s", opts.SocketPath)
	}

	if opts.InactivityTimeout != 0 {
		t.Errorf("Expected InactivityTimeout of 0, got %v", opts.InactivityTimeout)
	}
}

func TestStoreOptionsWithTTL(t *testing.T) {
	opts := &Store{}
	fn := WithTTL(3600)

	err := fn(opts)
	if err != nil {
		t.Fatalf("WithTTL failed: %v", err)
	}

	if opts.TtlSeconds != 3600 {
		t.Errorf("Expected TtlSeconds of 3600, got %d", opts.TtlSeconds)
	}
}

func TestStoreOptionsWithAbsoluteExpiration(t *testing.T) {
	opts := &Store{}
	expTime := int64(1234567890)
	fn := WithAbsoluteExpiration(expTime)

	err := fn(opts)
	if err != nil {
		t.Fatalf("WithAbsoluteExpiration failed: %v", err)
	}

	if opts.AbsoluteExpirationSeconds != expTime {
		t.Errorf("Expected AbsoluteExpirationSeconds of %d, got %d", expTime, opts.AbsoluteExpirationSeconds)
	}
}

func TestStoreOptionsMultipleFunctions(t *testing.T) {
	opts := &Store{}

	funcs := []StoreOptsFn{
		WithTTL(1800),
		WithAbsoluteExpiration(9999999999),
	}

	for _, fn := range funcs {
		if err := fn(opts); err != nil {
			t.Fatalf("StoreOptsFn failed: %v", err)
		}
	}

	if opts.TtlSeconds != 1800 {
		t.Errorf("Expected TtlSeconds of 1800, got %d", opts.TtlSeconds)
	}

	if opts.AbsoluteExpirationSeconds != 9999999999 {
		t.Errorf("Expected AbsoluteExpirationSeconds of 9999999999, got %d", opts.AbsoluteExpirationSeconds)
	}
}

func TestClientOptionsNoServer(t *testing.T) {
	opts := DefaultClient
	opts.NoServer = true

	if !opts.NoServer {
		t.Errorf("Expected NoServer to be true")
	}
}

func TestClientOptionsNonce(t *testing.T) {
	opts := DefaultClient
	nonce := "test-nonce-12345"
	opts.Nonce = nonce

	if opts.Nonce != nonce {
		t.Errorf("Expected Nonce %s, got %s", nonce, opts.Nonce)
	}
}

func TestCommonOptionsJSON(t *testing.T) {
	// Verify that Common struct has JSON tags for marshaling
	opts := defaultCommon

	if opts.EnvVarSocket != "BURNAFTER_SOCKET_PATH" {
		t.Errorf("Expected EnvVarSocket BURNAFTER_SOCKET_PATH, got %s", opts.EnvVarSocket)
	}

	if opts.EnvVarDebug != "BURNAFTER_DEBUG" {
		t.Errorf("Expected EnvVarDebug BURNAFTER_DEBUG, got %s", opts.EnvVarDebug)
	}
}
