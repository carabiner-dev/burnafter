// SPDX-FileCopyrightText: Copyright 2026 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"fmt"
	"time"
)

// memSecret is an encrypted secret held only in process memory. The plaintext
// never sits in the heap: it is sealed with the same per-secret AES-256-GCM key
// used by the file fallback, so an accidental core dump or log of the map does
// not expose secrets.
type memSecret struct {
	nonce      []byte
	ciphertext []byte
	expiry     int64 // unix seconds
}

// useMemory reports whether the client stores secrets only in process memory.
func (c *Client) useMemory() bool {
	return c.options.InMemory
}

// storeInMemory encrypts secret and keeps it in the in-process map.
func (c *Client) storeInMemory(name string, secret []byte, expiry time.Time) error {
	nonce, ciphertext, err := c.seal(name, secret)
	if err != nil {
		return err
	}
	c.memMu.Lock()
	c.memStore[name] = memSecret{nonce: nonce, ciphertext: ciphertext, expiry: expiry.Unix()}
	c.memMu.Unlock()
	return nil
}

// getFromMemory returns and decrypts a secret from the in-process map, removing
// and reporting it as not found once expired.
func (c *Client) getFromMemory(name string) ([]byte, error) {
	c.memMu.RLock()
	s, ok := c.memStore[name]
	c.memMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("secret not found")
	}
	if time.Now().Unix() > s.expiry {
		c.memMu.Lock()
		delete(c.memStore, name)
		c.memMu.Unlock()
		return nil, fmt.Errorf("secret expired")
	}
	return c.open(name, s.nonce, s.ciphertext)
}

// deleteFromMemory removes a secret from the in-process map.
func (c *Client) deleteFromMemory(name string) {
	c.memMu.Lock()
	delete(c.memStore, name)
	c.memMu.Unlock()
}
