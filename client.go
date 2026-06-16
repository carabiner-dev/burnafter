// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/chainguard-dev/clog"
	"google.golang.org/grpc"

	pb "github.com/carabiner-dev/burnafter/internal/common"
	isecrets "github.com/carabiner-dev/burnafter/internal/secrets"
	"github.com/carabiner-dev/burnafter/internal/server"
	"github.com/carabiner-dev/burnafter/options"
)

var ErrServerStartFailed = errors.New("server failed to start (and fallback mode is disabled)")

// ServerLauncher starts the burnafter server as a detached subprocess
// configured with the given options. The embedded-server implementation lives in
// the github.com/carabiner-dev/burnafter/embedded package (embedded.Launch);
// keeping it out of the core lets programs that only use the in-memory or
// encrypted-file modes avoid linking the multi-megabyte embedded server binary.
type ServerLauncher func(ctx context.Context, opts *options.Client) error

// Option configures a Client.
type Option func(*Client)

// WithServerLauncher enables embedded-server mode by supplying the launcher
// (typically embedded.Launch). Without it, a client falls back to file or
// in-memory storage instead of spawning a server.
func WithServerLauncher(l ServerLauncher) Option {
	return func(c *Client) { c.launcher = l }
}

// Client is the burnafter client.
//
// This is the library that applications use to spin up the embedded server
// used to store and retrieve secrets.
type Client struct {
	options           *options.Client
	conn              *grpc.ClientConn
	client            pb.BurnAfterClient
	serverStartFailed bool // Track if server startup was attempted and failed

	// launcher starts the embedded server when set (see WithServerLauncher).
	// When nil, the client never spawns a server and uses the file/in-memory
	// fallback instead.
	launcher ServerLauncher

	// mem is the ephemeral backend used when options.InMemory is set: the OS
	// secure store (kernel keyring) when available, otherwise an in-process map.
	// Defaults to the heap store and may be upgraded to the keyring in Connect.
	mem secretStore
}

// NewClient creates a new client instance.
func NewClient(opts *options.Client, clientOpts ...Option) *Client {
	// If no socket path is specified, generate one based on the client binary hash
	if opts.SocketPath == "" {
		opts.SocketPath = generateSocketPath()
	}

	c := &Client{
		options: opts,
		// Default ephemeral backend; Connect upgrades to the keyring when the
		// platform supports it.
		mem: newHeapStore(),
	}
	for _, o := range clientOpts {
		o(c)
	}
	return c
}

// generateSocketPath creates a socket path based on the client binary's SHA256 hash
func generateSocketPath() string {
	hash, err := pb.GetCurrentBinaryHash()
	if err != nil {
		// Fallback to a default path if we can't compute the hash
		return "/tmp/burnafter.sock"
	}

	// Use first 16 characters of hash for the socket name
	// This provides uniqueness while keeping the filename reasonable
	return fmt.Sprintf("/tmp/burnafter-%s.sock", hash[:16])
}

// Connect establishes the connection to the server.
// If the server is not running, this function spawns the
// forked process to start listening.
//
// If NoServer option is set or server startup fails, fallback mode is used.
func (c *Client) Connect(ctx context.Context) error {
	// In-memory mode keeps secrets ephemeral: no server, no files. Prefer the OS
	// secure store (kernel keyring) so secret bytes live in kernel memory rather
	// than the process heap; fall back to the encrypted heap map (set in
	// NewClient) when the keyring isn't available — e.g. a sandbox without
	// keyctl, or a non-Linux host where the keyring isn't ephemeral.
	if c.options.InMemory {
		if ks, err := isecrets.NewKeyringStorage(ctx); err == nil {
			c.mem = &keyringStore{storage: ks}
			clog.FromContext(ctx).Debug("in-memory secrets: using OS kernel keyring")
		} else {
			clog.FromContext(ctx).Debugf("in-memory secrets: keyring unavailable, using encrypted heap: %v", err)
		}
		return nil
	}

	// If NoServer option is set, skip server connection
	if c.options.NoServer {
		return nil
	}

	// If server startup already failed, skip trying again
	if c.serverStartFailed {
		if c.options.NoFallbackMode {
			return ErrServerStartFailed
		}
		return nil
	}

	// No launcher wired in: the embedded server isn't available (import the
	// github.com/carabiner-dev/burnafter/embedded package and pass
	// WithServerLauncher(embedded.Launch) to enable it). Use the file fallback
	// unless the caller requires the server.
	if c.launcher == nil {
		c.serverStartFailed = true
		if c.options.NoFallbackMode {
			return ErrServerStartFailed
		}
		return nil
	}

	// Check if server is already running
	if c.IsServerRunning(ctx) {
		return c.dial()
	}

	// Server is not running, start it
	if err := c.startServer(ctx); err != nil {
		// Starting the embedded server can fail in restricted environments
		// (e.g. sandboxes that block memfd_create or exec, such as some
		// containerized runtimes). By default this is not fatal: fall back to
		// encrypted file storage. Callers that require the server (and want a
		// hard failure instead of degrading) set NoFallbackMode.
		c.serverStartFailed = true
		if c.options.NoFallbackMode {
			return fmt.Errorf("%w: %w", ErrServerStartFailed, err)
		}
		clog.WarnContextf(ctx, "could not start burnafter server, using fallback storage: %v", err)
		return nil
	}

	// Wait for the server to be ready
	for range 10 {
		time.Sleep(100 * time.Millisecond)
		if c.IsServerRunning(ctx) {
			return c.dial()
		}
	}

	// Server failed to start, mark as failed and use fallback
	c.serverStartFailed = true
	if c.options.NoFallbackMode {
		return ErrServerStartFailed
	}
	return nil
}

// isServerRunning checks if the server is responding
func (c *Client) IsServerRunning(ctx context.Context) bool {
	d := net.Dialer{Timeout: 1 * time.Second}
	conn, err := d.DialContext(ctx, "unix", c.options.SocketPath)
	if err != nil {
		return false
	}
	conn.Close() //nolint:errcheck,gosec
	return true
}

// dial connects to the gRPC server through the unix socket
func (c *Client) dial() error {
	// Custom dialer for Unix domain sockets
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", c.options.SocketPath)
	}

	// Use "passthrough" as the scheme and a dummy IP address,
	// the actual connection is made by the custom dialer
	conn, err := grpc.NewClient(
		"passthrough:///unix",
		grpc.WithTransportCredentials(server.NewPeerCredentials()),
		grpc.WithContextDialer(dialer),
	)
	if err != nil {
		return fmt.Errorf("failed to dial server: %w", err)
	}

	c.conn = conn
	c.client = pb.NewBurnAfterClient(conn)

	return nil
}

// startServer starts the embedded server through the configured launcher.
// Connect guarantees c.launcher is non-nil before calling this.
func (c *Client) startServer(ctx context.Context) error {
	return c.launcher(ctx, c.options)
}

// Ping checks if the server is alive
func (c *Client) Ping(ctx context.Context) error {
	// In fallback mode, always return success
	if c.useFallback() {
		return nil
	}

	// Server mode
	if c.client == nil {
		return fmt.Errorf("not connected to server")
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resp, err := c.client.Ping(ctx, &pb.PingRequest{})
	if err != nil {
		return fmt.Errorf("pinging server: %w", err)
	}

	if !resp.Alive {
		return fmt.Errorf("server not alive")
	}

	return nil
}

// Close closes the connection to the server.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
