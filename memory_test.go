// SPDX-FileCopyrightText: Copyright 2026 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"testing"
	"time"

	"github.com/carabiner-dev/burnafter/options"
)

func newInMemoryClient() *Client {
	opts := *options.DefaultClient
	opts.InMemory = true
	opts.Nonce = "test-nonce"
	return NewClient(&opts)
}

func TestInMemory_StoreGetDelete(t *testing.T) {
	ctx := context.Background()
	c := newInMemoryClient()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := c.Store(ctx, "k", "v", options.WithTTL(3600)); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil || got != "v" {
		t.Fatalf("Get = (%q, %v), want (v, nil)", got, err)
	}

	// The value is held encrypted, not as plaintext.
	c.memMu.RLock()
	stored := c.memStore["k"]
	c.memMu.RUnlock()
	if len(stored.ciphertext) == 0 || string(stored.ciphertext) == "v" {
		t.Fatalf("expected ciphertext in memory, got %q", stored.ciphertext)
	}

	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Get(ctx, "k"); err == nil {
		t.Fatalf("expected miss after delete")
	}
}

func TestInMemory_Expiry(t *testing.T) {
	ctx := context.Background()
	c := newInMemoryClient()

	// Store with an absolute expiry in the past: a get must report it gone.
	past := time.Now().Add(-time.Minute).Unix()
	if err := c.Store(ctx, "k", "v", options.WithAbsoluteExpiration(past)); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if _, err := c.Get(ctx, "k"); err == nil {
		t.Fatalf("expected expired secret to be reported as not found")
	}
}
