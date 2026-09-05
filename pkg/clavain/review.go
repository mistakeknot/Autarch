package clavain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/mistakeknot/autarch/pkg/review"
)

// Review submits or polls the SAME immutable accepted proposal. The L2
// supervisor survives caller disconnects and retains Beads/Intercore IDs.
func (c *Client) Review(ctx context.Context, p review.Proposal) (review.Execution, error) {
	var result review.Execution
	if c.projectDir == "" || c.projectDir != p.Project {
		return result, fmt.Errorf("Clavain project binding required")
	}
	data, err := json.Marshal(map[string]any{"version": 1, "key": fmt.Sprintf("%s:%d", p.ID, p.Revision), "project": p.Project, "tracker": p.Tracker, "proposal": p})
	if err != nil {
		return result, err
	}
	cmd := exec.CommandContext(ctx, c.binPath, "review", "submit")
	cmd.Dir = c.projectDir
	cmd.Stdin = bytes.NewReader(data)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err = cmd.Run(); err != nil {
		return result, fmt.Errorf("Clavain review: %w: %s", err, stderr.String())
	}
	if err = json.Unmarshal(out.Bytes(), &result); err != nil {
		return result, err
	}
	if result.Project != p.Project || result.ProposalID != p.ID || result.ProposalRevision != p.Revision {
		return result, fmt.Errorf("Clavain returned unrelated execution")
	}
	return result, nil
}
