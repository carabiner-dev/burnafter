// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/carabiner-dev/burnafter"
	"github.com/carabiner-dev/burnafter/options"
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
	var err error
	// Handle commands
	switch command {
	case "store":
		err = runStore(context.Background(), clientOpts, args[1:])
	case "get":
		err = runGet(context.Background(), clientOpts, args[1:])
	case "ping":
		err = runPing(context.Background(), clientOpts)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		flag.Usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runStore(ctx context.Context, opts *options.Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: burnafter store <name> <secret> [ttl_seconds] [absolute_expiration_seconds]")
	}

	name := args[0]
	secret := args[1]
	ttl := int64(0)                // Use default TTL
	absoluteExpiration := int64(0) // No absolute expiration by default

	if len(args) >= 3 {
		var err error
		ttl, err = strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid TTL: %w", err)
		}
	}

	if len(args) >= 4 {
		var err error
		absoluteExpiration, err = strconv.ParseInt(args[3], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid absolute expiration: %w", err)
		}
	}

	c := burnafter.NewClient(opts)
	if err := c.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer c.Close() //nolint:errcheck

	if err := c.Store(ctx, name, secret, ttl, absoluteExpiration); err != nil {
		return fmt.Errorf("failed to store secret: %w", err)
	}

	fmt.Printf("Secret %q stored successfully\n", name)
	return nil
}

func runGet(ctx context.Context, opts *options.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: burnafter get <name>")
	}

	name := args[0]

	c := burnafter.NewClient(opts)
	if err := c.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer c.Close() //nolint:errcheck

	secret, err := c.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	// Output only the secret value (for easy piping)
	fmt.Println(secret)
	return nil
}

func runPing(ctx context.Context, opts *options.Client) error {
	// Create the new client, but don't connect
	c := burnafter.NewClient(opts)

	// If the server is not running, stop here to avoid
	// starting the daemon when just checking.
	if !c.IsServerRunning(ctx) {
		return fmt.Errorf("server is not running")
	}

	if err := c.Connect(ctx); err != nil {
		return fmt.Errorf("error connecting to server: %w", err)
	}
	defer c.Close() //nolint:errcheck

	if err := c.Ping(ctx); err != nil {
		return fmt.Errorf("server ping failed: %w", err)
	}

	fmt.Println("server is alive")
	return nil
}
