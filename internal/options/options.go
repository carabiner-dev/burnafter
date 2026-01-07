// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package options

import "time"

// Common options for client and server options
type Common struct {
	SocketPath        string
	DefaultTTL        time.Duration
	InactivityTimeout time.Duration
	Debug             bool
	EnvVarSocket      string
	EnvVarDebug       string
}

// Server options set
type Server struct {
	Common
}

// Client options set
type Client struct {
	Nonce string
	Common
}

// defaultCommon default common options shared by default server and client sets
var defaultCommon = Common{
	SocketPath:        "/tmp/burnafter.sock",
	DefaultTTL:        4 * time.Hour,
	InactivityTimeout: 10 * time.Minute,
	Debug:             false,
	EnvVarSocket:      "BURNAFTER_SOCKET_PATH",
	EnvVarDebug:       "BURNAFTER_DEBUG",
}

// DefaultClient default client options
var DefaultClient = &Client{
	Common: defaultCommon,
}

// DefaultServer default server options
var DefaultServer = &Server{
	Common: defaultCommon,
}
