// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build darwin && amd64
// +build darwin,amd64

package embedded

import (
	_ "embed"
)

// Embed the server binary for darwin/amd64
//
//go:embed servers/darwin/amd64/burnafter-server.gz
var embeddedServerBinary []byte
