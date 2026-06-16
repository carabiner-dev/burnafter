// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin || !cgo

package secrets

import (
	"context"
	"fmt"

	"github.com/carabiner-dev/burnafter/secrets"
)

// NewKeychainStorage always returns an error on non-macOS platforms.
func NewKeychainStorage(context.Context) (secrets.Storage, error) {
	return nil, fmt.Errorf("keychain storage is only supported on macOS")
}
