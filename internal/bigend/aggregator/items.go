package aggregator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/discovery"
)

// DefaultJamThreshold is how long an item may go untouched before it is
// flagged as a jam. Seven days: a bead nobody has updated for a week is stuck
// or neglected rather than merely slow. Override with SetJamThreshold.
const DefaultJamThreshold = 7 * 24 * time.Hour

// beadListTimeout bounds one `bd list` call so a hung bd cannot stall Refresh.
const beadListTimeout = 20 * time.Second

// Item is one unit of work (a bead) on a project's belt in the colony view.
type Item struct {
	ID      string `json:"id"`
	Project string `json:"project"` // project path, the key used by State.Projects
	Status  string `json:"status"`  // the stage the item sits in
	Owner   string `json:"owner,omitempty"`
	// Age is the time since the item last changed (updated time, or created
	// time when no update is recorded), in nanoseconds when marshalled.
	Age time.Duration `json:"age"`
	// Jam is true when Age exceeds the jam threshold.
	Jam bool `json:"jam,omitempty"`
}

// Clone returns a copy of the item.
func (i Item) Clone() Item { return i }

// FlowCount is the number of items that entered and left one stage of one
// project between two consecutive snapshots.
type FlowCount struct {
	Project string `json:"project"`
	Stage   string `json:"stage"`
	Entered int    `json:"entered"`
	Left    int    `json:"left"`
}

// Clone returns a copy of the flow count.
func (f FlowCount) Clone() FlowCount { return f }

// BeadRecord is the subset of `bd list --json` output the colony view reads.
type BeadRecord struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeadLister reads the beads of one project. Implementations must be
// read-only.
type BeadLister interface {
	ListBeads(ctx context.Context, projectPath string) ([]BeadRecord, error)
}

// Output caps for one `bd list` call. Variables so tests can lower them.
var (
	maxBDStdout int64 = 64 << 20
	maxBDStderr int64 = 64 << 10
)

// cappedBuffer collects up to limit bytes. The write that crosses the limit
// fails and cancels the command, so a runaway bd is killed instead of filling
// memory or blocking on a full pipe.
type cappedBuffer struct {
	buf      bytes.Buffer
	limit    int64
	cancel   context.CancelFunc
	exceeded bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if int64(c.buf.Len())+int64(len(p)) > c.limit {
		c.exceeded = true
		c.cancel()
		return 0, errors.New("output limit exceeded")
	}
	return c.buf.Write(p)
}

// BDLister lists beads by running `bd --readonly list --json --limit 0` in the
// project directory. --limit 0 lifts bd's default cap of 50, which would hide
// the oldest beads. It never writes to bd, and it fails if bd prints more than
// maxBDStdout bytes to stdout or maxBDStderr bytes to stderr.
type BDLister struct{}

// ListBeads implements BeadLister.
func (BDLister) ListBeads(ctx context.Context, projectPath string) ([]BeadRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, beadListTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "--readonly", "list", "--json", "--limit", "0")
	cmd.Dir = projectPath
	cmd.WaitDelay = 2 * time.Second
	stdout := &cappedBuffer{limit: maxBDStdout, cancel: cancel}
	stderr := &cappedBuffer{limit: maxBDStderr, cancel: cancel} // bd prints configuration warnings here; keep stdout pure JSON
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	switch {
	case stdout.exceeded:
		return nil, fmt.Errorf("bd list in %s: stdout exceeded %d bytes", projectPath, maxBDStdout)
	case stderr.exceeded:
		return nil, fmt.Errorf("bd list in %s: stderr exceeded %d bytes", projectPath, maxBDStderr)
	case err != nil:
		return nil, fmt.Errorf("bd list in %s: %w: %s", projectPath, err, bytes.TrimSpace(stderr.buf.Bytes()))
	}
	var recs []BeadRecord
	if err := json.Unmarshal(stdout.buf.Bytes(), &recs); err != nil {
		return nil, fmt.Errorf("decode bd list output for %s: %w", projectPath, err)
	}
	return recs, nil
}

// buildItems turns bead records into items, computing age and the jam flag
// against now.
func buildItems(project string, recs []BeadRecord, now time.Time, jam time.Duration) []Item {
	items := make([]Item, 0, len(recs))
	for _, r := range recs {
		touched := r.UpdatedAt
		if touched.IsZero() {
			touched = r.CreatedAt
		}
		age := now.Sub(touched)
		if age < 0 {
			age = 0
		}
		items = append(items, Item{
			ID:      r.ID,
			Project: project,
			Status:  r.Status,
			Owner:   r.Owner,
			Age:     age,
			Jam:     age > jam,
		})
	}
	return items
}

// computeFlow compares two item snapshots and counts, per project and stage,
// the items that entered (new, or moved in from another stage) and left
// (gone, or moved to another stage). Only stages with movement are returned,
// sorted by project then stage. It returns nil when nothing moved.
func computeFlow(prev, cur []Item) []FlowCount {
	type key struct{ project, id string }
	before := make(map[key]string, len(prev))
	for _, it := range prev {
		before[key{it.Project, it.ID}] = it.Status
	}
	type stageKey struct{ project, stage string }
	counts := map[stageKey]*FlowCount{}
	bump := func(project, stage string) *FlowCount {
		k := stageKey{project, stage}
		fc := counts[k]
		if fc == nil {
			fc = &FlowCount{Project: project, Stage: stage}
			counts[k] = fc
		}
		return fc
	}
	seen := make(map[key]struct{}, len(cur))
	for _, it := range cur {
		k := key{it.Project, it.ID}
		seen[k] = struct{}{}
		old, existed := before[k]
		if existed && old == it.Status {
			continue
		}
		bump(it.Project, it.Status).Entered++
		if existed {
			bump(it.Project, old).Left++
		}
	}
	for k, status := range before {
		if _, ok := seen[k]; !ok {
			bump(k.project, status).Left++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	flow := make([]FlowCount, 0, len(counts))
	for _, fc := range counts {
		flow = append(flow, *fc)
	}
	sort.Slice(flow, func(i, j int) bool {
		if flow[i].Project != flow[j].Project {
			return flow[i].Project < flow[j].Project
		}
		return flow[i].Stage < flow[j].Stage
	})
	return flow
}

// SetBeadLister replaces the source of bead data (tests use a fixture).
func (a *Aggregator) SetBeadLister(l BeadLister) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.beads = l
}

// SetJamThreshold sets the age past which an item is flagged as a jam.
func (a *Aggregator) SetJamThreshold(d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.jamThreshold = d
}

// itemResult is what one collection pass produces; commit it under the lock.
type itemResult struct {
	items []Item
	flow  []FlowCount
	// seen holds each listed project's last valid bead records. A project is
	// baselined (flow may be computed for it) only while it has an entry.
	seen map[string][]BeadRecord
}

// collectItems lists beads for every project that has a .beads directory and
// returns the items plus the flow against the previous snapshot.
//
// A project is only compared once it has two valid observations: a failed
// listing publishes nothing for a project that has no earlier valid listing,
// and the first valid listing sets its baseline without producing flow. When a
// later listing fails, the project's last valid records are rebuilt into items
// against now, so they keep ageing and can still become jams, and they
// produce no flow. Projects that vanish from discovery lose their baseline.
func (a *Aggregator) collectItems(ctx context.Context, projects []discovery.Project, now time.Time) itemResult {
	a.mu.RLock()
	lister, jam := a.beads, a.jamThreshold
	prev := a.state.Items
	prevSeen := a.itemSeen
	a.mu.RUnlock()

	res := itemResult{items: []Item{}, seen: map[string][]BeadRecord{}}
	compare := map[string]bool{} // projects with a valid observation now and before
	for _, p := range projects {
		if info, err := os.Stat(filepath.Join(p.Path, ".beads")); err != nil || !info.IsDir() {
			continue
		}
		recs, err := lister.ListBeads(ctx, p.Path)
		if err != nil {
			if last, ok := prevSeen[p.Path]; ok {
				slog.Warn("bead listing failed; ageing the last valid items", "project", p.Path, "error", err)
				res.seen[p.Path] = last
				res.items = append(res.items, buildItems(p.Path, last, now, jam)...)
			} else {
				slog.Warn("bead listing failed; no earlier listing to fall back on", "project", p.Path, "error", err)
			}
			continue
		}
		recs = append([]BeadRecord(nil), recs...)
		if _, ok := prevSeen[p.Path]; ok {
			compare[p.Path] = true
		}
		res.seen[p.Path] = recs
		res.items = append(res.items, buildItems(p.Path, recs, now, jam)...)
	}

	only := func(items []Item) []Item {
		var out []Item
		for _, it := range items {
			if compare[it.Project] {
				out = append(out, it)
			}
		}
		return out
	}
	res.flow = computeFlow(only(prev), only(res.items))
	return res
}

// commitItemsLocked stores a collection result in the aggregator. Callers hold a.mu
// for writing.
func (a *Aggregator) commitItemsLocked(res itemResult) {
	a.itemSeen = res.seen
	a.state.Items = res.items
	a.state.Flow = res.flow
}

// refreshItems collects items and flow and stores them in the state.
func (a *Aggregator) refreshItems(ctx context.Context, projects []discovery.Project, now time.Time) {
	res := a.collectItems(ctx, projects, now)
	a.mu.Lock()
	a.commitItemsLocked(res)
	a.mu.Unlock()
}
