package clavain

import (
	"context"
	"strings"
)

// SetArtifact registers an artifact path on a sprint bead.
func (c *Client) SetArtifact(ctx context.Context, beadID, artifactType, path string) error {
	_, err := c.execText(ctx, "set-artifact", beadID, artifactType, path)
	return err
}

// GetArtifact retrieves an artifact path for a sprint bead.
// Returns ("", nil) if no artifact of that type exists.
// Returns ("", err) for actual subprocess failures.
func (c *Client) GetArtifact(ctx context.Context, beadID, artifactType string) (string, error) {
	result, err := c.execText(ctx, "get-artifact", beadID, artifactType)
	if err != nil {
		// clavain-cli get-artifact exits 1 with empty stdout when artifact not found.
		// Distinguish "not found" (result is empty) from actual errors.
		if result == "" || strings.Contains(err.Error(), "not found") {
			return "", nil
		}
		return "", err
	}
	return result, nil
}
