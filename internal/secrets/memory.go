// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"fmt"
	"sync"

	"github.com/carabiner-dev/burnafter/secrets"
)

// MemoryStorage is an in-memory implementation of the secrets.Storage interface.
// It stores encrypted secrets in a map protected by a mutex for thread safety.
type MemoryStorage struct {
	data map[string]*secrets.Payload
	mu   sync.RWMutex
}

// NewMemoryStorage creates a new in-memory storage backend.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data: make(map[string]*secrets.Payload),
	}
}

// Store persists a secret in memory.
func (m *MemoryStorage) Store(ctx context.Context, id string, secret *secrets.Payload) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[id] = secret
	return nil
}

// Get retrieves a secret from memory by its ID.
func (m *MemoryStorage) Get(ctx context.Context, id string) (*secrets.Payload, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	secret, exists := m.data[id]
	if !exists {
		return nil, fmt.Errorf("secret not found")
	}

	return secret, nil
}

// Delete removes a secret from memory by its id.
func (m *MemoryStorage) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, id)
	return nil
}
