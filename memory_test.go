// SPDX-FileCopyrightText: Copyright 2026 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/carabiner-dev/burnafter/options"
	"github.com/carabiner-dev/burnafter/secrets"
)

// fakeStorage is an in-process secrets.Storage used to exercise the keyring
// code path on any platform (the real kernel keyring is Linux-only).
type fakeStorage struct {
	m map[string]*secrets.Payload
}

func newFakeStorage() *fakeStorage { return &fakeStorage{m: map[string]*secrets.Payload{}} }

func (f *fakeStorage) Store(_ context.Context, name string, p *secrets.Payload) error {
	f.m[name] = p
	return nil
}

func (f *fakeStorage) Get(_ context.Context, name string) (*secrets.Payload, error) {
	p, ok := f.m[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

func (f *fakeStorage) Delete(_ context.Context, name string) error {
	delete(f.m, name)
	return nil
}

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

	// The value is held encrypted, not as plaintext. On hosts without a kernel
	// keyring (e.g. macOS) the backend is the heap store, so we can inspect it.
	if hs, ok := c.mem.(*heapStore); ok {
		hs.mu.RLock()
		stored := hs.m["k"]
		hs.mu.RUnlock()
		if len(stored.ciphertext) == 0 || string(stored.ciphertext) == "v" {
			t.Fatalf("expected ciphertext in memory, got %q", stored.ciphertext)
		}
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

func TestMarshalMemSecret_RoundTrip(t *testing.T) {
	in := memSecret{
		nonce:      bytes.Repeat([]byte{0xab}, gcmNonceSize),
		ciphertext: []byte("encrypted-bytes"),
		expiry:     1893456000, // 2030-01-01
	}
	out, err := unmarshalMemSecret(marshalMemSecret(in))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bytes.Equal(out.nonce, in.nonce) || !bytes.Equal(out.ciphertext, in.ciphertext) || out.expiry != in.expiry {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, in)
	}

	if _, err := unmarshalMemSecret([]byte("short")); err == nil {
		t.Fatalf("expected error on truncated blob")
	}
}

// TestInMemory_KeyringBackend exercises the keyring code path cross-platform by
// forcing a keyringStore over a fake secrets.Storage.
func TestInMemory_KeyringBackend(t *testing.T) {
	ctx := context.Background()
	c := newInMemoryClient()
	c.mem = &keyringStore{storage: newFakeStorage()}

	if err := c.Store(ctx, "k", "v", options.WithTTL(3600)); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := c.Get(ctx, "k")
	if err != nil || got != "v" {
		t.Fatalf("Get = (%q, %v), want (v, nil)", got, err)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Get(ctx, "k"); err == nil {
		t.Fatalf("expected miss after delete")
	}
}
