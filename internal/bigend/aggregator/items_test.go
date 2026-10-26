package aggregator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
)

// fakeLister serves bead records from a fixture and counts calls per project.
type fakeLister struct {
	records map[string][]BeadRecord
	errs    map[string]error
	calls   map[string]int
}

func (f *fakeLister) ListBeads(_ context.Context, projectPath string) ([]BeadRecord, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[projectPath]++
	if err := f.errs[projectPath]; err != nil {
		return nil, err
	}
	return f.records[projectPath], nil
}

func beadProject(t *testing.T, name string, withBeads bool) discovery.Project {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if withBeads {
		if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return discovery.Project{Path: dir, Name: name}
}

func findItem(items []Item, id string) (Item, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

func findFlow(flow []FlowCount, project, stage string) FlowCount {
	for _, f := range flow {
		if f.Project == project && f.Stage == stage {
			return f
		}
	}
	return FlowCount{}
}

// openBeads builds n open beads a-001.. last touched `idle` before now.
func openBeads(n int, now time.Time, idle time.Duration) []BeadRecord {
	recs := make([]BeadRecord, 0, n)
	for i := 1; i <= n; i++ {
		recs = append(recs, BeadRecord{
			ID:        fmt.Sprintf("a-%03d", i),
			Status:    "open",
			Owner:     "mk",
			CreatedAt: now.Add(-idle - 24*time.Hour),
			UpdatedAt: now.Add(-idle),
		})
	}
	return recs
}

func TestItemsAndFlowOverTwoSnapshots(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	alpha := beadProject(t, "alpha", true)
	plain := beadProject(t, "plain", false) // no .beads: must not be listed

	// Snapshot 1: 60 open beads (more than bd's default limit of 50), one of
	// them untouched for 30 days.
	snap1 := openBeads(60, now, 2*time.Hour)
	snap1[59] = BeadRecord{ID: "a-060", Status: "open", Owner: "mk",
		CreatedAt: now.Add(-45 * 24 * time.Hour), UpdatedAt: now.Add(-30 * 24 * time.Hour)}

	lister := &fakeLister{records: map[string][]BeadRecord{alpha.Path: snap1}}
	agg := New(nil, &config.Config{}, nil)
	agg.SetBeadLister(lister)
	projects := []discovery.Project{alpha, plain}

	agg.refreshItems(context.Background(), projects, now)
	s1 := agg.GetState()

	if got := len(s1.Items); got != 60 {
		t.Fatalf("snapshot 1 items = %d, want 60", got)
	}
	if lister.calls[plain.Path] != 0 {
		t.Fatalf("project without .beads was listed")
	}
	if s1.Flow != nil {
		t.Fatalf("first snapshot has no predecessor, flow = %+v", s1.Flow)
	}
	it, ok := findItem(s1.Items, "a-001")
	if !ok || it.Project != alpha.Path || it.Status != "open" || it.Owner != "mk" {
		t.Fatalf("a-001 = %+v, ok=%v", it, ok)
	}
	if it.Age != 2*time.Hour || it.Jam {
		t.Fatalf("a-001 age=%v jam=%v, want 2h and no jam", it.Age, it.Jam)
	}
	old, _ := findItem(s1.Items, "a-060")
	if old.Age != 30*24*time.Hour || !old.Jam {
		t.Fatalf("a-060 age=%v jam=%v, want 720h and jam", old.Age, old.Jam)
	}

	// Snapshot 2, one minute later: a-001 starts work, a-002 closes (drops out
	// of the open list), a-061 arrives.
	now2 := now.Add(time.Minute)
	snap2 := openBeads(60, now, 2*time.Hour)
	snap2[59] = snap1[59]
	snap2[0].Status = "in_progress"
	snap2 = append(snap2[:1], snap2[2:]...) // drop a-002
	snap2 = append(snap2, BeadRecord{ID: "a-061", Status: "open", Owner: "mk",
		CreatedAt: now2, UpdatedAt: now2})
	lister.records[alpha.Path] = snap2

	agg.refreshItems(context.Background(), projects, now2)
	s2 := agg.GetState()

	if got := len(s2.Items); got != 60 {
		t.Fatalf("snapshot 2 items = %d, want 60", got)
	}
	if _, ok := findItem(s2.Items, "a-002"); ok {
		t.Fatal("a-002 should be gone")
	}
	if it, _ := findItem(s2.Items, "a-001"); it.Status != "in_progress" {
		t.Fatalf("a-001 status = %q", it.Status)
	}
	if it, _ := findItem(s2.Items, "a-061"); it.Age != 0 {
		t.Fatalf("a-061 age = %v, want 0", it.Age)
	}
	if old, _ := findItem(s2.Items, "a-060"); !old.Jam || old.Age != 30*24*time.Hour+time.Minute {
		t.Fatalf("a-060 age=%v jam=%v", old.Age, old.Jam)
	}

	open := findFlow(s2.Flow, alpha.Path, "open")
	if open.Entered != 1 || open.Left != 2 { // a-061 in; a-001 and a-002 out
		t.Fatalf("open flow = %+v, want entered 1 left 2", open)
	}
	prog := findFlow(s2.Flow, alpha.Path, "in_progress")
	if prog.Entered != 1 || prog.Left != 0 {
		t.Fatalf("in_progress flow = %+v, want entered 1 left 0", prog)
	}
	if len(s2.Flow) != 2 {
		t.Fatalf("flow entries = %+v, want 2 stages", s2.Flow)
	}
}

func TestJamThresholdConfigurable(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	p := beadProject(t, "alpha", true)
	lister := &fakeLister{records: map[string][]BeadRecord{p.Path: openBeads(1, now, 2*time.Hour)}}
	agg := New(nil, &config.Config{}, nil)
	agg.SetBeadLister(lister)

	agg.refreshItems(context.Background(), []discovery.Project{p}, now)
	if agg.GetState().Items[0].Jam {
		t.Fatal("2h-old item jammed under the default threshold")
	}
	agg.SetJamThreshold(time.Hour)
	agg.refreshItems(context.Background(), []discovery.Project{p}, now)
	if !agg.GetState().Items[0].Jam {
		t.Fatal("2h-old item not jammed under a 1h threshold")
	}
	if DefaultJamThreshold != 7*24*time.Hour {
		t.Fatalf("DefaultJamThreshold = %v, documented default is 7 days", DefaultJamThreshold)
	}
}

func TestListFailureKeepsPreviousItemsWithoutFakeFlow(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	p := beadProject(t, "alpha", true)
	lister := &fakeLister{records: map[string][]BeadRecord{p.Path: openBeads(3, now, time.Hour)}}
	agg := New(nil, &config.Config{}, nil)
	agg.SetBeadLister(lister)
	projects := []discovery.Project{p}

	agg.refreshItems(context.Background(), projects, now)
	lister.errs = map[string]error{p.Path: errors.New("bd unavailable")}
	agg.refreshItems(context.Background(), projects, now.Add(time.Minute))

	s := agg.GetState()
	if len(s.Items) != 3 {
		t.Fatalf("items after failed listing = %d, want 3 carried forward", len(s.Items))
	}
	if len(s.Flow) != 0 {
		t.Fatalf("failed listing produced flow %+v", s.Flow)
	}
}

func TestGetStateReturnsIndependentItemSnapshot(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	p := beadProject(t, "alpha", true)
	lister := &fakeLister{records: map[string][]BeadRecord{p.Path: openBeads(2, now, time.Hour)}}
	agg := New(nil, &config.Config{}, nil)
	agg.SetBeadLister(lister)
	agg.refreshItems(context.Background(), []discovery.Project{p}, now)

	s := agg.GetState()
	s.Items[0].Status = "mutated"
	s.Items = append(s.Items[:0], s.Items[1:]...)

	again := agg.GetState()
	if len(again.Items) != 2 || again.Items[0].Status != "open" {
		t.Fatalf("mutating a snapshot leaked into the aggregator: %+v", again.Items)
	}
}

func TestBDListerPassesReadOnlyUnlimitedArgsAndIgnoresStderr(t *testing.T) {
	bin := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\necho 'warning: noise' >&2\n" +
		`echo '[{"id":"x-1","status":"open","owner":"mk","created_at":"2026-10-01T00:00:00Z","updated_at":"2026-10-02T00:00:00Z"}]'` + "\n"
	if err := os.WriteFile(filepath.Join(bin, "bd"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	recs, err := BDLister{}.ListBeads(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ID != "x-1" || recs[0].UpdatedAt.IsZero() {
		t.Fatalf("records = %+v", recs)
	}
	args, _ := os.ReadFile(argsFile)
	if string(args) != "--readonly list --json --limit 0\n" {
		t.Fatalf("bd args = %q", args)
	}
}
