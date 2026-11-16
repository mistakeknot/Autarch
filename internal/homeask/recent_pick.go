package homeask

import (
	"maps"
	"strings"
	"time"
)

// sameDecision ignores filing keys and delivery-only metadata. Changing a title
// or owner is not a new ask; pinned artifact digests remain part of the identity.
func sameDecision(r CardRequest, c Card) bool {
	identity := func(ask map[string]any, root *RootRun, move map[string]any) string {
		ask = maps.Clone(ask)
		for _, k := range []string{"ask_key", "evidence", "group"} {
			delete(ask, k)
		}
		id, err := RequestIdentity(CardRequest{Title: "decision", Ask: ask, RootRun: root, Move: move})
		if err != nil {
			return ""
		}
		return id
	}
	var move map[string]any
	if c.Move != nil {
		move = c.Move.Raw
	}
	a, b := identity(r.Ask, r.RootRun, r.Move), identity(c.Ask, c.RootRun, move)
	return a != "" && a == b
}
func (f *CardFiler) recentPick(comments []Comment) (recent, unreadable bool) {
	now := time.Now()
	if f.Now != nil {
		now = f.Now()
	}
	for _, c := range comments {
		if c.Kind != "user" || !strings.HasPrefix(c.Body, "Ruled:") {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, c.CreatedAt)
		if err != nil {
			return true, true
		}
		if !at.Before(now.Add(-24 * time.Hour)) {
			recent = true
		}
	}
	return recent, false
}
