// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build linux && amd64
// +build linux,amd64

package client

import (
	_ "embed"
)

// Embed the server binary for linux/amd64
//
//go:embed embedded/linux/amd64/burnafter-server.gz
var embeddedServerBinary []byte
