package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPickedCardAlwaysRefusesRefile(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, state, createdAt         string
		changed, supersedes, noComment bool
	}{
		{name: "recent ruled", state: "ruled", createdAt: now.Add(-time.Hour).Format(time.RFC3339Nano)},
		{name: "old ruled", state: "ruled", createdAt: now.Add(-25 * time.Hour).Format(time.RFC3339Nano)},
		{name: "changed ask", state: "picked", createdAt: now.Add(-time.Hour).Format(time.RFC3339Nano), changed: true},
		{name: "supersedes changed ask", state: "picked", createdAt: now.Add(-time.Hour).Format(time.RFC3339Nano), changed: true, supersedes: true},
		{name: "no comment", state: "picked", noComment: true},
		{name: "empty time", state: "picked"},
		{name: "unparseable time", state: "picked", createdAt: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bb, f, state := updateRig(t)
			f.Now = func() time.Time { return now }
			r := testReq()
			r.Ask["ask_key"] = "stable"
			first, err := f.FileOrUpdateCard(context.Background(), r, "")
			if err != nil {
				t.Fatal(err)
			}
			*state = tc.state
			if !tc.noComment {
				bb.tasks[0].Comments = append(bb.tasks[0].Comments, Comment{Kind: "user", Body: "Ruled: chosen", CreatedAt: tc.createdAt})
			}
			r.Key = "retry"
			if tc.changed {
				r.Ask["question"] = "A different decision?"
			}
			supersedes := ""
			if tc.supersedes {
				supersedes = first.ID
			}
			before := bb.tasks[0].Desc
			_, err = f.FileOrUpdateCard(context.Background(), r, supersedes)
			if !errors.Is(err, ErrAlreadyRuled) || !errors.Is(err, errNoCardWritten) {
				t.Fatalf("picked card must refuse without writing: %v", err)
			}
			if bb.count("tasks", "create") != 1 || bb.count("tasks", "update") != 0 || bb.tasks[0].Desc != before {
				t.Fatal("picked card was created or edited")
			}
		})
	}
}

func TestUnreadablePickTimeDiagnostic(t *testing.T) {
	for _, timestamp := range []string{"", "unknown"} {
		t.Run(timestamp, func(t *testing.T) {
			bb, f, state := updateRig(t)
			r := testReq()
			r.Ask["ask_key"] = "stable"
			if _, err := f.FileOrUpdateCard(context.Background(), r, ""); err != nil {
				t.Fatal(err)
			}
			*state = "closed"
			bb.tasks[0].Comments = append(bb.tasks[0].Comments, Comment{Kind: "user", Body: "Ruled: chosen", CreatedAt: timestamp})
			r.Key = "retry"
			_, err := f.FileOrUpdateCard(context.Background(), r, "")
			if !errors.Is(err, ErrAlreadyRuled) || !strings.Contains(err.Error(), "pick time is unreadable") {
				t.Fatalf("expected unreadable pick time refusal, got %v", err)
			}
		})
	}
}

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
