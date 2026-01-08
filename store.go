// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"fmt"
	"time"

	pb "github.com/carabiner-dev/burnafter/internal/common"
	"github.com/carabiner-dev/burnafter/options"
)

// Store stores a secret on the server or in fallback encrypted file storage
func (c *Client) Store(ctx context.Context, name, secret string, funcs ...options.StoreOptsFn) error {
	opts := &options.Store{}
	for _, f := range funcs {
		if err := f(opts); err != nil {
			return err
		}
	}

	// Use fallback storage if server is not available
	if c.useFallback() {
		// Calculate expiry time
		var expiryTime time.Time
		switch {
		case opts.AbsoluteExpirationSeconds > 0:
			expiryTime = time.Unix(opts.AbsoluteExpirationSeconds, 0)
		case opts.TtlSeconds > 0:
			expiryTime = time.Now().Add(time.Duration(opts.TtlSeconds) * time.Second)
		default:
			// Use default TTL
			expiryTime = time.Now().Add(c.options.DefaultTTL)
		}

		// Encrypt and store to file
		if err := c.encryptSecret(name, []byte(secret), expiryTime); err != nil {
			return fmt.Errorf("failed to store secret in fallback: %w", err)
		}

		// Cleanup expired files
		_ = c.cleanupExpiredFallbackFiles() //nolint:errcheck

		return nil
	}

	// Server mode
	if c.client == nil {
		return fmt.Errorf("not connected to server")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.client.Store(ctx, &pb.StoreRequest{
		Name:                      name,
		Secret:                    secret,
		TtlSeconds:                opts.TtlSeconds,
		ClientNonce:               c.options.Nonce,
		AbsoluteExpirationSeconds: opts.AbsoluteExpirationSeconds,
	})
	if err != nil {
		return fmt.Errorf("failed to store secret: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("server error: %s", resp.Error)
	}

	return nil
}
