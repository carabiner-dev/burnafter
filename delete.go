// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"context"
	"fmt"
)

// Delete removes a secret from fallback encrypted file storage
// Note: Delete is only supported in fallback mode currently
func (c *Client) Delete(ctx context.Context, name string) error {
	// Use fallback storage if server is not available
	if c.useFallback() {
		// Delete from file
		if err := c.deleteFallbackSecret(name); err != nil {
			return err
		}

		// Cleanup expired files
		_ = c.cleanupExpiredFallbackFiles() //nolint:errcheck

		return nil
	}

	// Server mode - Delete not yet implemented in server
	return fmt.Errorf("delete is only supported in fallback mode")
}
