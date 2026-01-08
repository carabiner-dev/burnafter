// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/carabiner-dev/burnafter/internal/client"
	"github.com/carabiner-dev/burnafter/internal/options"
)

const usage = `burnafter - Ephemeral secret storage with binary verification

Usage:
  burnafter store <name> <secret> [ttl_seconds] [absolute_expiration_secs]  Store a secret
  burnafter get <name>                                                      Retrieve a secret
  burnafter ping                                                            Check if server is running
  
Options:
  -socket string    Socket path (defaults to random tmp path)
  -debug            Enable debug output

Secret Expiration:
  Secrets expire when EITHER condition is met:
  1. Inactivity timeout: Not accessed for ttl_seconds (resets on each read)
  2. Absolute deadline: absolute_expiration_secs elapsed since storage (optional)

Secrets are zero-wiped after expiring. Once a server is no longer guarding any
secrets, it shuts down.

Examples:
  # Store with inactivity timeout only (default: 4 hours)
  burnafter store api-key "my-secret-key"

  # Store with 1 hour inactivity timeout
  burnafter store api-key "my-secret-key" 3600

  # Store with 1 hour inactivity timeout AND 8 hour absolute deadline
  burnafter store api-key "my-secret-key" 3600 28800

  # Retrieve a secret (resets inactivity timer)
  burnafter get api-key

  # Check server status
  burnafter ping
`

func main() {
	// Define flags
	socketPath := flag.String("socket", "", "Unix custom socket path")
	debug := flag.Bool("debug", false, "Enable debug output")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}
	flag.Parse()

	// Suppress default log output unless debug mode
	if !*debug {
		log.SetOutput(os.NewFile(0, os.DevNull))
	}

	// Get subcommand
	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]

	clientOpts := options.DefaultClient

	// Only set socket path if user provided one (empty means auto-generate)
	if *socketPath != "" {
		clientOpts.SocketPath = *socketPath
	}
	clientOpts.Debug = *debug

	// Handle commands
	switch command {
	case "store":
		runStore(clientOpts, args[1:])
	case "get":
		runGet(clientOpts, args[1:])
	case "ping":
		runPing(clientOpts)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

func runStore(opts *options.Client, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: burnafter store <name> <secret> [ttl_seconds] [absolute_expiration_seconds]")
		os.Exit(1)
	}

	name := args[0]
	secret := args[1]
	ttl := int64(0)                // Use default TTL
	absoluteExpiration := int64(0) // No absolute expiration by default

	if len(args) >= 3 {
		var err error
		ttl, err = strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid TTL: %v\n", err)
			os.Exit(1)
		}
	}

	if len(args) >= 4 {
		var err error
		absoluteExpiration, err = strconv.ParseInt(args[3], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid absolute expiration: %v\n", err)
			os.Exit(1)
		}
	}

	c := client.NewClient(opts)
	if err := c.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	if err := c.Store(name, secret, ttl, absoluteExpiration); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to store secret: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Secret '%s' stored successfully\n", name)
}

func runGet(opts *options.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: burnafter get <name>")
		os.Exit(1)
	}

	name := args[0]

	c := client.NewClient(opts)
	if err := c.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	secret, err := c.Get(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get secret: %v\n", err)
		os.Exit(1)
	}

	// Output only the secret value (for easy piping)
	fmt.Println(secret)
}

func runPing(opts *options.Client) {
	// Create the new client, but don't connect
	c := client.NewClient(opts)

	// If the server is not running, stop here to avoid
	// starting the daemon when just checking.
	if !c.IsServerRunning() {
		fmt.Fprintf(os.Stderr, "Server is not running\n")
		os.Exit(1)
	}
	defer c.Close()

	if err := c.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to server: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	if err := c.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "Server ping failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Server is alive")
}
