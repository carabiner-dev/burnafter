// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

//go:build linux && arm64
// +build linux,arm64

package embedded

import (
	_ "embed"
)

// Embed the server binary for linux/arm64
//
//go:embed servers/linux/arm64/burnafter-server.gz
var embeddedServerBinary []byte
