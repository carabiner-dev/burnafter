// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build !linux
// +build !linux

package embedded

import (
	"context"
	"fmt"
)

// CreateMemfdServer is not available on non-Linux platforms.
// The client will automatically fall back to extract the binary
// to an ephemeral tmp file.
func CreateMemfdServer(_ context.Context) (int, error) {
	return -1, fmt.Errorf("memfd_create not supported on this platform")
}
