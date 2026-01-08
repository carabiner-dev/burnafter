// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

// Package main provides a minimal server-only entry point for the burnafter
// dæmon. This binary is designed to be embedded in the client library and
// spawned as an independent daemon process when needed by applications
// using the client.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/carabiner-dev/burnafter/internal/server"
	"github.com/carabiner-dev/burnafter/options"
)

func main() {
	// Start server with default options
	serverOpts := options.DefaultServer

	// If JSON options are passed as the first argument, merge them with defaults
	if len(os.Args) > 1 {
		var clientOpts options.Common
		if err := json.Unmarshal([]byte(os.Args[1]), &clientOpts); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse options: %v\n", err)
			os.Exit(1)
		}
		serverOpts.Common = clientOpts
	}

	// Configure logging based on debug setting
	if serverOpts.Debug {
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
