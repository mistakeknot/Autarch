package homeask

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// TargetEvidence is read from Home's host adapter, never from card metadata.
type TargetEvidence struct {
	Schema      int    `json:"schema"`
	ThreadID    string `json:"thread_id"`
	Title       string `json:"title"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Archived    *bool  `json:"archived"`
	Deleted     *bool  `json:"deleted"`
	ObservedAt  string `json:"observed_at"`
}

func (f *CardFiler) lookupPickTarget(ctx context.Context, thread string) (TargetEvidence, error) {
	var t TargetEvidence
	if err := f.call(ctx, thread, &t, "home", "target", "--thread", thread, "--json"); err != nil {
		return t, err
	}
	_, dateErr := time.Parse(time.RFC3339Nano, t.ObservedAt)
	if t.Schema != 1 || t.ThreadID != thread || strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.ProjectID) == "" || strings.TrimSpace(t.ProjectName) == "" || t.Archived == nil || t.Deleted == nil || *t.Archived || *t.Deleted || dateErr != nil {
		return t, fmt.Errorf("%w: on_pick target metadata unavailable", ErrInvalid)
	}
	return t, nil
}

// PreflightOnPick is read-only. Every filing lane calls it before any Tasks write.
// An isolated canary can call it without ever entering a mutation lane.
func (f *CardFiler) PreflightOnPick(ctx context.Context, req CardRequest) error {
	desc, _, err := req.Description()
	if err != nil {
		return err
	}
	card, err := ParseCard(desc)
	if err != nil {
		return err
	}
	ask, err := card.ToV1(req.Thread)
	if err != nil {
		return err
	}
	participating := false
	for _, o := range ask.Options {
		if o.OnPick != nil {
			participating = true
		}
	}
	if !participating {
		return nil
	}
	var caps struct {
		Schema int   `json:"schema"`
		OnPick []int `json:"on_pick"`
		Target []int `json:"target_lookup"`
	}
	if err = f.call(ctx, req.Thread, &caps, "home", "capabilities", "--json"); err != nil {
		return err
	}
	contains := func(xs []int) bool {
		for _, x := range xs {
			if x == 1 {
				return true
			}
		}
		return false
	}
	if caps.Schema != 1 || !contains(caps.OnPick) || !contains(caps.Target) {
		return fmt.Errorf("%w: on_pick capabilities unavailable", ErrHomeDown)
	}
	b, ok := f.binding(ctx, req.Project)
	if !ok || b.home == "" || b.home != ask.Project {
		return fmt.Errorf("%w: on_pick confirmed binding required", ErrInvalid)
	}
	if req.Thread == "" {
		return fmt.Errorf("%w: on_pick asker required", ErrInvalid)
	}
	if _, err = f.lookupPickTarget(ctx, req.Thread); err != nil {
		return err
	}
	for _, o := range ask.Options {
		if o.OnPick == nil {
			continue
		}
		var declaration map[string]any
		if err := json.Unmarshal(o.OnPick, &declaration); err != nil {
			return err
		}
		target, _ := declaration["target_thread"].(string)
		allowed := target == req.Thread
		for _, b := range card.Blocks {
			if b.Ref == "thread:"+target {
				allowed = true
			}
		}
		if !allowed {
			return fmt.Errorf("%w: on_pick target outside generation scope", ErrInvalid)
		}
		if _, err = f.lookupPickTarget(ctx, target); err != nil {
			return err
		}
		subject, _ := declaration["subject"].(map[string]any)
		if subject["kind"] == "github-pr" {
			if err = checkPickHead(ctx, subject); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkPickHead(ctx context.Context, subject map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	number, _ := subject["number"].(float64)
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", strconv.FormatInt(int64(number), 10), "--repo", subject["repo"].(string), "--json", "state,headRefOid")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("%w: on_pick head unavailable: %v", ErrHomeDown, err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(pipe, 65537))
	if len(raw) > 65536 {
		cancel()
	}
	waitErr := cmd.Wait()
	var head struct{ State, HeadRefOid string }
	if readErr != nil || waitErr != nil || len(raw) > 65536 || json.Unmarshal(raw, &head) != nil || head.State != "OPEN" || head.HeadRefOid != subject["head"] {
		return fmt.Errorf("%w: on_pick head unavailable or changed", ErrInvalid)
	}
	return nil
}
