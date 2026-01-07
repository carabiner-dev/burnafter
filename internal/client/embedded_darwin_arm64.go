// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build darwin && arm64
// +build darwin,arm64

package client

import (
	_ "embed"
)

// Embed the server binary for darwin/arm64
//
//go:embed embedded/darwin/arm64/burnafter-server.gz
var embeddedServerBinary []byte
