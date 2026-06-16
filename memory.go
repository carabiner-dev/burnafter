// SPDX-FileCopyrightText: Copyright 2026 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/carabiner-dev/burnafter/secrets"
)

// memSecret is an encrypted secret held in an ephemeral backend. The plaintext
// is never stored: it is sealed with the same per-secret AES-256-GCM key used by
// the file fallback, so neither the keyring payload nor the heap map exposes it.
type memSecret struct {
	nonce      []byte
	ciphertext []byte
	expiry     int64 // unix seconds
}

// secretStore is an ephemeral backend for in-memory mode. Implementations keep
// secrets only for the life of the process / session: the OS secure store
// (kernel keyring) where available, otherwise an in-process map.
type secretStore interface {
	put(ctx context.Context, name string, s memSecret) error
	get(ctx context.Context, name string) (memSecret, bool, error)
	del(ctx context.Context, name string)
}

// heapStore is an in-process, mutex-guarded map of encrypted secrets. It is the
// fallback when no OS secure store is available (e.g. a sandbox without keyctl,
// or a non-Linux host where the keyring isn't ephemeral).
type heapStore struct {
	mu sync.RWMutex
	m  map[string]memSecret
}

func newHeapStore() *heapStore { return &heapStore{m: make(map[string]memSecret)} }

func (h *heapStore) put(_ context.Context, name string, s memSecret) error {
	h.mu.Lock()
	h.m[name] = s
	h.mu.Unlock()
	return nil
}

func (h *heapStore) get(_ context.Context, name string) (memSecret, bool, error) {
	h.mu.RLock()
	s, ok := h.m[name]
	h.mu.RUnlock()
	return s, ok, nil
}

func (h *heapStore) del(_ context.Context, name string) {
	h.mu.Lock()
	delete(h.m, name)
	h.mu.Unlock()
}

// keyringStore stores the (already encrypted) secret in an OS secure store via a
// secrets.Storage backend — on Linux this is the kernel keyring, so the bytes
// live in kernel memory rather than the process heap.
type keyringStore struct {
	storage secrets.Storage
}

func (k *keyringStore) put(ctx context.Context, name string, s memSecret) error {
	return k.storage.Store(ctx, name, &secrets.Payload{EncryptedData: marshalMemSecret(s)})
}

func (k *keyringStore) get(ctx context.Context, name string) (memSecret, bool, error) {
	payload, err := k.storage.Get(ctx, name)
	if err != nil || payload == nil {
		// Any retrieval failure (including "not found") is treated as a miss; a
		// cache miss just triggers a fresh exchange.
		return memSecret{}, false, nil
	}
	s, err := unmarshalMemSecret(payload.EncryptedData)
	if err != nil {
		return memSecret{}, false, err
	}
	return s, true, nil
}

func (k *keyringStore) del(ctx context.Context, name string) {
	_ = k.storage.Delete(ctx, name) //nolint:errcheck // best-effort
}

// marshalMemSecret encodes [nonce | expiry | ciphertext] as opaque bytes for
// storage backends that hold a single blob per secret.
func marshalMemSecret(s memSecret) []byte {
	expiry := s.expiry
	if expiry < 0 {
		expiry = 0
	}
	buf := make([]byte, gcmNonceSize+8+len(s.ciphertext))
	copy(buf, s.nonce)
	binary.BigEndian.PutUint64(buf[gcmNonceSize:], uint64(expiry))
	copy(buf[gcmNonceSize+8:], s.ciphertext)
	return buf
}

func unmarshalMemSecret(b []byte) (memSecret, error) {
	if len(b) < gcmNonceSize+8 {
		return memSecret{}, fmt.Errorf("invalid in-memory secret blob: too small")
	}
	expiry := binary.BigEndian.Uint64(b[gcmNonceSize : gcmNonceSize+8])
	if expiry > math.MaxInt64 {
		return memSecret{}, fmt.Errorf("invalid expiry in in-memory secret blob")
	}
	nonce := make([]byte, gcmNonceSize)
	copy(nonce, b[:gcmNonceSize])
	ciphertext := make([]byte, len(b)-gcmNonceSize-8)
	copy(ciphertext, b[gcmNonceSize+8:])
	return memSecret{nonce: nonce, ciphertext: ciphertext, expiry: int64(expiry)}, nil
}

// useMemory reports whether the client stores secrets only ephemerally.
func (c *Client) useMemory() bool { return c.options.InMemory }

// storeInMemory seals secret and writes it to the ephemeral backend.
func (c *Client) storeInMemory(ctx context.Context, name string, secret []byte, expiry time.Time) error {
	nonce, ciphertext, err := c.seal(name, secret)
	if err != nil {
		return err
	}
	return c.mem.put(ctx, name, memSecret{nonce: nonce, ciphertext: ciphertext, expiry: expiry.Unix()})
}

// getFromMemory reads, expiry-checks, and decrypts a secret from the ephemeral
// backend.
func (c *Client) getFromMemory(ctx context.Context, name string) ([]byte, error) {
	s, ok, err := c.mem.get(ctx, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("secret not found")
	}
	if time.Now().Unix() > s.expiry {
		c.mem.del(ctx, name)
		return nil, fmt.Errorf("secret expired")
	}
	return c.open(name, s.nonce, s.ciphertext)
}

// deleteFromMemory removes a secret from the ephemeral backend.
func (c *Client) deleteFromMemory(ctx context.Context, name string) {
	c.mem.del(ctx, name)
}
