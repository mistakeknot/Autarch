package clavain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Prepare is deliberately separate from Review's implementation operation.
func (c *Client) Prepare(ctx context.Context, operation string, request json.RawMessage) (json.RawMessage, error) {
	if operation != "ratify" && operation != "submit" && operation != "status" {
		return nil, fmt.Errorf("unknown preparation operation")
	}
	var binding struct {
		Project string `json:"project"`
	}
	if err := json.Unmarshal(request, &binding); err != nil {
		return nil, err
	}
	if c.projectDir == "" || binding.Project != c.projectDir {
		return nil, fmt.Errorf("preparation project binding required")
	}
	cmd := exec.CommandContext(ctx, c.binPath, "prepare", operation)
	cmd.Dir = c.projectDir
	cmd.Stdin = bytes.NewReader(request)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("Clavain preparation: %w: %s", err, stderr.String())
	}
	if !json.Valid(out.Bytes()) {
		return nil, fmt.Errorf("invalid Clavain preparation receipt")
	}
	return out.Bytes(), nil
}
