// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build !linux
// +build !linux

package client

import "fmt"

// createMemfdServer is not available on non-Linux platforms.
// The client will automatically fall back to extract the binary
// to the user's .cache directory
func createMemfdServer() (int, error) {
	return -1, fmt.Errorf("memfd_create not supported on this platform")
}
