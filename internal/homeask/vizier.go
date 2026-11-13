package homeask

import (
	"context"
	"fmt"
	"strings"
)

// CheckVizierOwnership uses the same pinned, unarchived title-prefix selection as
// vizier-tell. On lookup failure only a caller identifiable as a vizier candidate,
// or one claiming --vizier-owned, is refused; ordinary callers remain unaffected.
func (f *CardFiler) CheckVizierOwnership(ctx context.Context, thread, owner string, owned bool) error {
	refusal := func(detail string) error {
		return fmt.Errorf("%w: %s; use --owner-thread for another thread's follow-through or --vizier-owned for the vizier's own ruling, root run or scope", ErrInvalid, detail)
	}
	if owned && owner != "" {
		return refusal("give exactly one ownership flag")
	}
	type row struct {
		ID, Title            string
		PinnedAt, ArchivedAt any
	}
	var rows []row
	err := f.call(ctx, "", &rows, "thread", "list", "--json")
	if err != nil {
		var current struct{ Thread row }
		_ = f.call(ctx, "", &current, "thread", "show", thread, "--json")
		if owned || strings.HasPrefix(current.Thread.Title, "Masaq' | vizier") {
			return refusal("cannot resolve the vizier")
		}
		return nil
	}
	hits := []string{}
	candidate := false
	for _, r := range rows {
		if r.ID == thread && strings.HasPrefix(r.Title, "Masaq' | vizier") {
			candidate = true
		}
		if strings.HasPrefix(r.Title, "Masaq' | vizier") && r.PinnedAt != nil && r.ArchivedAt == nil {
			hits = append(hits, r.ID)
		}
	}
	if len(hits) != 1 {
		if owned || candidate {
			return refusal("cannot resolve one pinned, unarchived vizier")
		}
		return nil
	}
	if hits[0] == thread {
		if !owned && owner == "" {
			return refusal("vizier filing requires an owner")
		}
		return nil
	}
	if owned {
		return refusal("--vizier-owned is only for the vizier")
	}
	return nil
}
