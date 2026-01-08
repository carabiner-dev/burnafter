// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"fmt"
	"time"

	pb "github.com/carabiner-dev/burnafter/internal/common"
)

// Get retrieves a secret from the server
func (c *Client) Get(name string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("not connected to server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
