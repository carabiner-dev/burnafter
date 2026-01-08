// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"fmt"
	"time"

	pb "github.com/carabiner-dev/burnafter/internal/common"
)

// Store stores a secret on the server
func (c *Client) Store(ctx context.Context, name, secret string, ttlSeconds, absoluteExpirationSeconds int64) error {
	if c.client == nil {
		return fmt.Errorf("not connected to server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.client.Store(ctx, &pb.StoreRequest{
		Name:                      name,
		Secret:                    secret,
		TtlSeconds:                ttlSeconds,
		ClientNonce:               c.options.Nonce,
		AbsoluteExpirationSeconds: absoluteExpirationSeconds,
	})

	if err != nil {
		return fmt.Errorf("failed to store secret: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("server error: %s", resp.Error)
	}

	return nil
}
