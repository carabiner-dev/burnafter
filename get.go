// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"fmt"
	"time"

	pb "github.com/carabiner-dev/burnafter/internal/common"
)

// Get retrieves a secret from the server or fallback encrypted file storage
func (c *Client) Get(ctx context.Context, name string) (string, error) {
	// Use fallback storage if server is not available
	if c.useFallback() {
		// Decrypt from file
		secret, err := c.decryptSecret(name)
		if err != nil {
			return "", err
		}

		// Cleanup expired files
		_ = c.cleanupExpiredFallbackFiles() //nolint:errcheck

		return string(secret), nil
	}

	// Server mode
	if c.client == nil {
		return "", fmt.Errorf("not connected to server")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.client.Get(ctx, &pb.GetRequest{
		Name:        name,
		ClientNonce: c.options.Nonce,
	})
	if err != nil {
		return "", fmt.Errorf("getting secret: %w", err)
	}

	if !resp.Success {
		return "", fmt.Errorf("server error: %s", resp.Error)
	}

	return resp.Secret, nil
}
