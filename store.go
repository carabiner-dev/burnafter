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

// Store stores a secret on the server
func (c *Client) Store(ctx context.Context, name, secret string, funcs ...options.StoreOptsFn) error {
	if c.client == nil {
		return fmt.Errorf("not connected to server")
	}

	opts := &options.Store{}
	for _, f := range funcs {
		if err := f(opts); err != nil {
			return err
		}
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
