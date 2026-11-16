package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRecentPickedClosedCardCannotRefile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		age     time.Duration
		changed bool
		refuse  bool
	}{{"recent", time.Hour, false, true}, {"changed-ask", time.Hour, true, false}, {"expired", 25 * time.Hour, false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			bb, f, state := updateRig(t)
			now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
			f.Now = func() time.Time { return now }
			r := testReq()
			r.Ask["ask_key"] = "stable"
			if _, err := f.FileOrUpdateCard(context.Background(), r, ""); err != nil {
				t.Fatal(err)
			}
			bb.tasks[0].Status = "done"
			*state = "closed"
			bb.tasks[0].Comments = append(bb.tasks[0].Comments, Comment{Kind: "user", Body: "Ruled: chosen", CreatedAt: now.Add(-tc.age).Format(time.RFC3339Nano)})
			r.Key = "retry"
			if tc.changed {
				r.Ask["question"] = "A different decision?"
			}
			_, err := f.FileOrUpdateCard(context.Background(), r, "")
			if tc.refuse && (!errors.Is(err, ErrAlreadyRuled) || bb.count("tasks", "create") != 1) {
				t.Fatalf("recent pick refiled: %v", err)
			}
			if !tc.refuse && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecentPickAllowsNewPinnedHead(t *testing.T) {
	bb, f, state := updateRig(t)
	now := time.Now()
	f.Now = func() time.Time { return now }
	r := testReq()
	r.Ask["ask_key"] = "stable"
	r.RootRun = &RootRun{Script: "/sample.sh", SHA256: strings.Repeat("a", 64), Timeout: 30, Set: "sample"}
	if _, err := f.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	bb.tasks[0].Status = "done"
	*state = "closed"
	bb.tasks[0].Comments = append(bb.tasks[0].Comments, Comment{Kind: "user", Body: "Ruled: chosen", CreatedAt: now.Format(time.RFC3339Nano)})
	r.Key = "new-head"
	r.RootRun.SHA256 = strings.Repeat("b", 64)
	if _, err := f.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	if bb.count("tasks", "create") != 2 {
		t.Fatal("new pinned head not filed")
	}
}
