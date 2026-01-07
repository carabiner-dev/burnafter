// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	pb "github.com/carabiner-dev/burnafter/internal/common"
	"github.com/carabiner-dev/burnafter/internal/options"
	"github.com/carabiner-dev/burnafter/internal/server"
	"google.golang.org/grpc"
)

// Client is the burnafter client.
// This is the library that applications embed to spin up a server
// and store and retrieve secrets.
type Client struct {
	options *options.Client
	conn    *grpc.ClientConn
	client  pb.BurnAfterClient
}

// NewClient creates a new client instance
func NewClient(opts *options.Client) *Client {
	return &Client{
		options: opts,
	}
}

// Connect establishes the connection to the server.
// If the server is not running, this function spawns the
// forked process to start listening.
func (c *Client) Connect() error {
	// Check if server is already running
	if c.isServerRunning() {
		return c.dial()
	}

	// Server is not running, start it
	if err := c.startServer(); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	// Wait for server to be ready
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if c.isServerRunning() {
			return c.dial()
		}
	}

	return fmt.Errorf("server failed to start within timeout")
}

// isServerRunning checks if the server is responding
func (c *Client) isServerRunning() bool {
	conn, err := net.DialTimeout("unix", c.options.SocketPath, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// dial connects to the gRPC server through the unix socket
func (c *Client) dial() error {
	// Custom dialer for Unix domain sockets
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return net.Dial("unix", c.options.SocketPath)
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

// startServer launches the server as a daemon forked from the client process
func (c *Client) startServer() error {
	// Get the path to the current executable
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Prepare command to run server in daemon mode
	cmd := exec.Command(exePath, "server")

	// Detach from parent process
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}

	// Set up stdin/stdout/stderr
	if c.options.Debug {
		// In th client is in debug mode, inherit stdout/stderr
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		// In production, redirect to /dev/null
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
		return fmt.Errorf("failed to start server process: %w", err)
	}

	// Don't wait for the process and release it so it doesn't become a zombie
	go cmd.Wait()

	return nil
}

// Ping checks if the server is alive
func (c *Client) Ping() error {
	if c.client == nil {
		return fmt.Errorf("not connected to server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
