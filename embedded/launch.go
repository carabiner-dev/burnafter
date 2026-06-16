// SPDX-FileCopyrightText: Copyright 2026 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

// Package embedded provides the embedded burnafter server and the launcher that
// starts it. Importing this package is what pulls the (multi-megabyte) embedded
// server binary into your program, so wire it explicitly:
//
//	client := burnafter.NewClient(opts, burnafter.WithServerLauncher(embedded.Launch))
//
// Programs that only use the in-memory or encrypted-file modes should not import
// this package, so the linker leaves the embedded binary out of their build.
package embedded

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	iembedded "github.com/carabiner-dev/burnafter/internal/embedded"
	"github.com/carabiner-dev/burnafter/options"
)

// Launch starts the embedded burnafter server as a detached subprocess
// configured with opts. It executes the embedded binary from memory
// (memfd_create) when possible, falling back to extracting it to a temp file
// (e.g. on macOS or when memfd is blocked). It matches burnafter's ServerLauncher
// signature, so pass it via burnafter.WithServerLauncher(embedded.Launch).
func Launch(ctx context.Context, opts *options.Client) error {
	var cmd *exec.Cmd
	var memFile *os.File
	var tempServerPath string // Track temp file for cleanup

	// Try memfd approach first (better security, no disk writes).
	memfd, err := iembedded.CreateMemfdServer(ctx)
	if err == nil {
		if memfd < 0 {
			return fmt.Errorf("invalid memfd file descriptor: %d", memfd)
		}
		memFile = os.NewFile(uintptr(memfd), "burnafter-server")
		if memFile != nil {
			defer memFile.Close() //nolint:errcheck

			// Execute via /proc/self/fd/3 (ExtraFiles map starting at fd 3).
			cmd = exec.CommandContext(ctx, "/proc/self/fd/3")
			cmd.ExtraFiles = []*os.File{memFile}

			if opts.Debug {
				fmt.Fprintf(os.Stderr, "Attempting to start server from memfd...\n")
			}
		}
	}

	// Fall back to writing the binary to a temp file if memfd failed, is blocked,
	// or is unsupported (e.g. macOS).
	if cmd == nil {
		if opts.Debug {
			fmt.Fprintf(os.Stderr, "memfd unavailable, falling back to temp file...\n")
		}

		serverPath, err := iembedded.ExtractServerBinaryToTemp(ctx)
		if err != nil {
			return fmt.Errorf("failed to extract server binary: %w", err)
		}
		tempServerPath = serverPath

		cmd = exec.CommandContext(ctx, serverPath) //nolint:gosec // Path is controlled
	}

	// Pass the client options to the server as the first argument.
	optionsJSON, err := json.Marshal(opts.Common)
	if err != nil {
		return fmt.Errorf("failed to marshal options: %w", err)
	}
	cmd.Args = append([]string{cmd.Path, string(optionsJSON)}, cmd.Args[1:]...)
	cmd.Env = os.Environ()

	// Detach from the parent process.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if opts.Debug {
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

	if err := cmd.Start(); err != nil {
		if tempServerPath != "" {
			os.Remove(tempServerPath) //nolint:errcheck,gosec
		}

		// If memfd execution was blocked (likely SELinux), retry via temp file.
		if memFile != nil && os.IsPermission(err) {
			if opts.Debug {
				fmt.Fprintf(os.Stderr, "memfd execution blocked, retrying with temp file...\n")
			}

			memFile.Close() //nolint:errcheck,gosec

			serverPath, extractErr := iembedded.ExtractServerBinaryToTemp(ctx)
			if extractErr != nil {
				return fmt.Errorf("failed to extract server binary: %w", extractErr)
			}
			tempServerPath = serverPath

			cmd = exec.CommandContext(ctx, serverPath) //nolint:gosec // Path is controlled
			cmd.Args = append([]string{cmd.Path, string(optionsJSON)}, cmd.Args[1:]...)
			cmd.Env = os.Environ()
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid: true,
			}

			if opts.Debug {
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

			if err := cmd.Start(); err != nil {
				if tempServerPath != "" {
					os.Remove(tempServerPath) //nolint:errcheck,gosec
				}
				return fmt.Errorf("failed to start server process: %w", err)
			}
		} else {
			return fmt.Errorf("failed to start server process: %w", err)
		}
	}

	// Delay temp-file cleanup so the OS can finish loading the binary.
	if tempServerPath != "" {
		go func(path string) {
			time.Sleep(2 * time.Second)
			if opts.Debug {
				fmt.Fprintf(os.Stderr, "Removing temp server file: %s\n", path)
			}
			os.Remove(path) //nolint:errcheck,gosec
		}(tempServerPath)
	}

	// Reap the process without blocking so it doesn't become a zombie.
	go cmd.Wait() //nolint:errcheck

	return nil
}
