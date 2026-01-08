// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "github.com/carabiner-dev/burnafter/internal/common"
	"github.com/carabiner-dev/burnafter/internal/embedded"
	"github.com/carabiner-dev/burnafter/internal/server"
	"github.com/carabiner-dev/burnafter/options"
)

// Client is the burnafter client.
//
// This is the library that applications use to spin up the embedded server
// used to store and retrieve secrets.
type Client struct {
	options *options.Client
	conn    *grpc.ClientConn
	client  pb.BurnAfterClient
}

// NewClient creates a new client instance
func NewClient(opts *options.Client) *Client {
	// If no socket path is specified, generate one based on the client binary hash
	if opts.SocketPath == "" {
		opts.SocketPath = generateSocketPath()
	}

	return &Client{
		options: opts,
	}
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
func (c *Client) Connect(ctx context.Context) error {
	// Check if server is already running
	if c.IsServerRunning(ctx) {
		return c.dial()
	}

	// Server is not running, start it
	if err := c.startServer(ctx); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	// Wait for the server to be ready
	for range 10 {
		time.Sleep(100 * time.Millisecond)
		if c.IsServerRunning(ctx) {
			return c.dial()
		}
	}

	return fmt.Errorf("server failed to start within timeout")
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

// startServer launches the server as a daemon by executing
// the embedded server binary. It attempts to execute from memory first (memfd_create),
// falling back to cache directory extraction if memory execution is blocked (e.g., by SELinux).
func (c *Client) startServer(ctx context.Context) error {
	var cmd *exec.Cmd
	var memFile *os.File
	var tempServerPath string // Track temp file for cleanup

	// Try memfd approach first (better security, no disk writes)
	memfd, err := embedded.CreateMemfdServer(ctx)
	if err == nil {
		// Convert the raw fd to an *os.File so we can pass it via ExtraFiles
		memFile = os.NewFile(uintptr(memfd), "burnafter-server")
		if memFile != nil {
			defer memFile.Close() //nolint:errcheck

			// Execute the binary via /proc/self/fd/3
			// ExtraFiles are mapped starting at fd 3 in the child process
			cmd = exec.CommandContext(ctx, "/proc/self/fd/3")
			cmd.ExtraFiles = []*os.File{memFile}

			if c.options.Debug {
				fmt.Fprintf(os.Stderr, "Attempting to start server from memfd...\n")
			}
		}
	}

	// Fallback to writing the binary to temp file if memfd failed, blocked
	// or is not supported (ie macos)
	if cmd == nil {
		if c.options.Debug {
			fmt.Fprintf(os.Stderr, "memfd unavailable, falling back to temp file...\n")
		}

		serverPath, err := embedded.ExtractServerBinaryToTemp(ctx)
		if err != nil {
			return fmt.Errorf("failed to extract server binary: %w", err)
		}
		tempServerPath = serverPath

		cmd = exec.CommandContext(ctx, serverPath) //nolint:gosec // Path is controlled
	}

	// Marshal the client options to JSON to pass to the server
	optionsJSON, err := json.Marshal(c.options.Common)
	if err != nil {
		return fmt.Errorf("failed to marshal options: %w", err)
	}

	// Set the JSON options as the first command-line argument
	cmd.Args = append([]string{cmd.Path, string(optionsJSON)}, cmd.Args[1:]...)

	// Inherit the environment (no need to pass individual vars)
	cmd.Env = os.Environ()

	// Detach from parent process
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}

	// Set up stdin/stdout/stderr
	if c.options.Debug {
		// In debug mode, we inherit stdout/stderr so that
		// we can see the output con the calling application
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		// In production, redirect to /dev/null. At some poing this should
		// implement some way to log.
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("failed to open /dev/null: %w", err)
		}
		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	}

	// Start the server process
	if err := cmd.Start(); err != nil {
		// Clean up temp file if it was created
		if tempServerPath != "" {
			os.Remove(tempServerPath) //nolint:errcheck,gosec
		}

		// If memfd execution failed (likely SELinux), try temp file fallback
		if memFile != nil && os.IsPermission(err) {
			if c.options.Debug {
				fmt.Fprintf(os.Stderr, "memfd execution blocked, retrying with temp file...\n")
			}

			memFile.Close() //nolint:errcheck,gosec // Close the memfd, we're not using it

			// Extract the binary to a temp file
			serverPath, extractErr := embedded.ExtractServerBinaryToTemp(ctx)
			if extractErr != nil {
				return fmt.Errorf("failed to extract server binary: %w", extractErr)
			}
			tempServerPath = serverPath

			cmd = exec.CommandContext(ctx, serverPath) //nolint:gosec // Path is controlled

			// Marshal options and pass as first argument
			cmd.Args = append([]string{cmd.Path, string(optionsJSON)}, cmd.Args[1:]...)
			cmd.Env = os.Environ()

			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid: true,
			}

			if c.options.Debug {
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
			} else {
				devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
				if err != nil {
					return fmt.Errorf("failed to open /dev/null: %w", err)
				}
				cmd.Stdin = devNull
				cmd.Stdout = devNull
				cmd.Stderr = devNull
			}

			// Retry starting the server with temp binary
			if err := cmd.Start(); err != nil {
				// Clean up temp file on failure
				if tempServerPath != "" {
					os.Remove(tempServerPath) //nolint:errcheck,gosec
				}
				return fmt.Errorf("failed to start server process: %w", err)
			}
		} else {
			return fmt.Errorf("failed to start server process: %w", err)
		}
	}

	// Server started successfully - schedule cleanup of temp file if it was created
	// We delay the cleanup to give the OS time to load the binary into memory
	if tempServerPath != "" {
		go func(path string) {
			// Wait a bit for the OS to finish loading the binary into memory
			time.Sleep(2 * time.Second)
			if c.options.Debug {
				fmt.Fprintf(os.Stderr, "Removing temp server file: %s\n", path)
			}
			os.Remove(path) //nolint:errcheck,gosec
		}(tempServerPath)
	}

	// Release the process and don't wait for it so it doesn't become a zombie
	go cmd.Wait() //nolint:errcheck

	return nil
}

// Ping checks if the server is alive
func (c *Client) Ping(ctx context.Context) error {
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
