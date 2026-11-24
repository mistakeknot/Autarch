package homeask

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Preflight runs the existing wire parser and project checks without submitting.
// Independent failures are joined so callers can fix them in one pass.
func (f *CardFiler) Preflight(ctx context.Context, r CardRequest) error {
	var failures []error
	add := func(err error) {
		if err != nil {
			failures = append(failures, err)
		}
	}
	add(CheckOptionIDs(r.Ask))
	add(f.checkCujgelFiling(ctx, r))
	// Exact-request recovery must remain possible after temporary proof is gone.
	existing := false
	if CheckRequestKey(r.Key) == nil {
		if identity, err := RequestIdentity(r); err == nil {
			candidate, err := f.locate(ctx, r.Key, identity)
			existing = err == nil && candidate != nil
		}
	}
	if !existing {
		add(f.CheckUI(r))
	}
	if _, _, err := r.Description(); err != nil {
		add(fmt.Errorf("%w; fix the named field (option labels allow at most %d characters)", err, maxLabel))
	}
	_, _, err := f.PrepareAsk(ctx, r.Project, fmt.Sprint(r.Ask["project"]), fmt.Sprint(r.Ask["project_root"]))
	if err != nil {
		add(fmt.Errorf("%w; use the registered project and root pair", err))
	}
	// Missing ownership flags are valid for ordinary callers. The CLI checks
	// the vizier-only requirement through CheckVizierOwnership.
	if r.Owner != "" && r.VizierOwned {
		add(fmt.Errorf("%w: give exactly one ownership flag", ErrInvalid))
	}
	switch r.Type {
	case "", "other", "play", "merge":
	case "product", "design":
		q, _ := r.Ask["question"].(string)
		lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(q, "\r\n", "\n")), "\n")
		if len(lines) < 5 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" || strings.TrimSpace(lines[2]) == "" || strings.TrimSpace(lines[3]) != "" || strings.TrimSpace(strings.Join(lines[4:], "\n")) == "" {
			add(fmt.Errorf("%w: add a three-line opener: what this is / what the reader would see or get / why now, then a blank line and the question", ErrInvalid))
		}
	default:
		add(fmt.Errorf("%w: --card-type must be product, design, play, merge or other", ErrInvalid))
	}
	return errors.Join(failures...)
}
