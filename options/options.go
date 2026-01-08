// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package options

import "time"

// Common options for client and server options
type Common struct {
	SocketPath        string        `json:"socket_path"`
	DefaultTTL        time.Duration `json:"default_ttl"`
	InactivityTimeout time.Duration `json:"inactivity_timeout"`
	Debug             bool          `json:"debug"`
	EnvVarSocket      string        `json:"envar_socket"`
	EnvVarDebug       string        `json:"envar_debug"`
	MaxSecrets        int           `json:"max_secrets"`     // Maximum number of secrets that can be stored
	MaxSecretSize     int64         `json:"max_secret_size"` // Maximum size of a single secret in bytes
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
	SocketPath:        "", // Empty = auto-generate based on client binary hash
	DefaultTTL:        4 * time.Hour,
	InactivityTimeout: 0, // Inactivity time to shutdown the server when no more connections are detected
	Debug:             false,
	EnvVarSocket:      "BURNAFTER_SOCKET_PATH",
	EnvVarDebug:       "BURNAFTER_DEBUG",
	MaxSecrets:        100,         // Maximum 100 secrets
	MaxSecretSize:     1024 * 1024, // 1 MB per secret
}

// DefaultClient default client options
var DefaultClient = &Client{
	Common: defaultCommon,
}

// DefaultServer default server options
var DefaultServer = &Server{
	Common: defaultCommon,
}
