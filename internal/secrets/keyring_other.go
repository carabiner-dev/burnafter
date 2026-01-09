// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package secrets

import (
	"fmt"

	"github.com/carabiner-dev/burnafter/secrets"
)

// NewKeyringStorage always returns an error on non-Linux platforms.
func NewKeyringStorage() (secrets.Storage, error) {
	return nil, fmt.Errorf("kernel keyring storage is only supported on Linux")
}
