// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

// Package main provides a minimal server-only entry point for the burnafter
// dæmon. This binary is designed to be embedded in the client library and
// spawned as an independent daemon process when needed by applications
// using the client.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/carabiner-dev/burnafter/internal/options"
	"github.com/carabiner-dev/burnafter/internal/server"
)

func main() {
	serverOpts := options.DefaultServer

	if socketPath := os.Getenv(serverOpts.EnvVarSocket); socketPath != "" {
		serverOpts.SocketPath = socketPath
	}

	if os.Getenv(serverOpts.EnvVarDebug) == "1" {
		serverOpts.Debug = true
		log.SetOutput(os.Stderr)
	} else {
		log.SetOutput(os.NewFile(0, os.DevNull))
	}

	srv, err := server.NewServer(serverOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	if serverOpts.Debug {
		log.Println("Starting burnafter server...")
	}

	if err := srv.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
