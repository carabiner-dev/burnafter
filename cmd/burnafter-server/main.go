// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

// Package main provides a minimal server-only entry point for the burnafter
// dæmon. This binary is designed to be embedded in the client library and
// spawned as an independent daemon process when needed by applications
// using the client.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/carabiner-dev/burnafter/internal/server"
	"github.com/carabiner-dev/burnafter/options"
	"github.com/chainguard-dev/clog"
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

	log := clog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Totally mute the log unless debugging
	if !serverOpts.Debug {
		log = clog.New(&noopHandler{})
	}

	// Initialize the context with the loaded logger
	ctx := clog.WithLogger(context.Background(), log)

	// Create the new server
	srv, err := server.NewServer(ctx, serverOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	clog.FromContext(ctx).Info("Starting burnafter server...")

	if err := srv.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

type noopHandler struct{}

func (h *noopHandler) Enabled(_ context.Context, level slog.Level) bool   { return false }
func (h *noopHandler) Handle(_ context.Context, record slog.Record) error { return nil }
func (h *noopHandler) WithAttrs(attrs []slog.Attr) slog.Handler           { return h }
func (h *noopHandler) WithGroup(name string) slog.Handler                 { return h }
