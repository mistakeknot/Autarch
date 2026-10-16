package backlogrun

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Mode is weekly (full triage, Home cross-check, needs-spec and gated messages) or daily (ready dispatch only).
type Mode string

const (
	Weekly Mode = "weekly"
	Daily  Mode = "daily"
)

// Options configure one run.
type Options struct {
	Mode        Mode
	DryRun      bool // write the digest and message only the vizier
	MaxInFlight int  // a coordinator with this many cards in flight is not messaged (default 3)
	BatchSize   int  // cards per triage call (default 25)
	// RepeatAfter is how long a card is left alone after a message about it before the same message is repeated.
	RepeatAfter time.Duration
	Now         func() time.Time
}

// Deps are the outside world.
type Deps struct {
	Board    BoardSource
	Triage   Triager
	Home     HomeReader
	Notify   Notifier
	Pool     Pool
	State    State
	Recorded func(Receipt) // test hook; nil in production
}

// Result is what a run did.
type Result struct {
	Receipt Receipt
	Digest  string // digest file path, empty if none was written
}

// Run does one run. It always appends a receipt, including when it is paused, skipped or fails. A failure sends the
// vizier one message saying so, unless the run is paused.
func Run(ctx context.Context, d Deps, o Options) (Result, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.MaxInFlight <= 0 {
		o.MaxInFlight = 3
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 25
	}
	if o.RepeatAfter <= 0 {
		o.RepeatAfter = 14 * 24 * time.Hour
	}
	now := o.Now().UTC()
	rec := Receipt{At: now, Mode: string(o.Mode), DryRun: o.DryRun, Counts: map[string]int{}}
	finish := func(outcome, detail string, err error) (Result, error) {
		rec.Outcome, rec.Detail = outcome, detail
		if aerr := d.State.AppendReceipt(rec); aerr != nil && err == nil {
			err = aerr
		}
		if d.Recorded != nil {
			d.Recorded(rec)
		}
		return Result{Receipt: rec, Digest: rec.Digest}, err
	}
	if o.Mode != Weekly && o.Mode != Daily {
		return finish("failed", "unknown mode", fmt.Errorf("mode must be weekly or daily, got %q", o.Mode))
	}
	if d.State.Paused() {
		return finish("paused", "pause file present: "+d.State.PauseFile(), nil)
	}
	fail := func(stage string, err error) (Result, error) {
		msg := fmt.Sprintf("Backlog run (%s) FAILED at %s: %v\nNo coordinator was messaged. Receipt: %s", o.Mode, stage, err, filepath.Join(d.State.Dir, "receipts.jsonl"))
		if terr := d.Notify.TellVizier(ctx, msg); terr != nil {
			msg += fmt.Sprintf("\n(vizier message also failed: %v)", terr)
		}
		return finish("failed", stage+": "+err.Error(), err)
	}

	mem, err := d.State.Load()
	if err != nil {
		return fail("load memory", err)
	}
	cards, err := d.Board.Cards(ctx)
	if err != nil {
		return fail("snapshot boards", err)
	}
	rec.Counts["cards"] = len(cards)
	byKey := map[string]Card{}
	for _, c := range cards {
		byKey[c.Key] = c
	}

	if o.Mode == Weekly {
		// Triage only what is new or changed since the last weekly snapshot; an unchanged card keeps its verdict.
		var todo []Card
		for _, c := range cards {
			v, have := mem.Verdicts[c.Key]
			if !have || v.UpdatedAt != c.UpdatedAt {
				todo = append(todo, c)
			}
		}
		rec.Counts["triaged"] = len(todo)
		if len(todo) > 0 {
			var seat Seat
			for i := 0; i < len(todo); i += o.BatchSize {
				end := i + o.BatchSize
				if end > len(todo) {
					end = len(todo)
				}
				vs, s, terr := d.Triage.Triage(ctx, todo[i:end])
				if terr != nil {
					return fail("triage", terr)
				}
				seat = s
				for _, v := range vs {
					c, ok := byKey[v.Key]
					if !ok {
						continue // a verdict for a card that is not in this batch is ignored
					}
					v.Board, v.UpdatedAt = c.Board, c.UpdatedAt
					mem.Verdicts[v.Key] = v
				}
			}
			rec.Seat = &seat
		}
		// A card that left backlog/todo has no verdict to keep.
		for k := range mem.Verdicts {
			if _, ok := byKey[k]; !ok {
				delete(mem.Verdicts, k)
				delete(mem.Sent, k)
			}
		}
		mem.Seen = map[string]string{}
		for _, c := range cards {
			mem.Seen[c.Key] = c.UpdatedAt
		}
	}

	// Current verdicts only: a verdict stands while the card has the updatedAt it was triaged at.
	current := map[string]Verdict{}
	for k, v := range mem.Verdicts {
		if c, ok := byKey[k]; ok && c.UpdatedAt == v.UpdatedAt {
			current[k] = v
		}
	}
	for _, v := range current {
		rec.Counts["label:"+v.Label]++
	}

	msgs := map[string]*coordMsg{} // by board
	boardMsg := func(b string) *coordMsg {
		if msgs[b] == nil {
			msgs[b] = &coordMsg{board: b}
		}
		return msgs[b]
	}
	var homeNote []string
	sentNow := map[string]Sent{}
	due := func(key, kind string) bool {
		s, ok := mem.Sent[key]
		return !ok || s.Kind != kind || now.Sub(s.At) >= o.RepeatAfter
	}

	if o.Mode == Weekly {
		var gated []string
		for _, v := range current {
			if v.Label == LabelMKGated {
				gated = append(gated, v.Key)
			}
		}
		sort.Strings(gated)
		states := map[string]string{}
		if len(gated) > 0 {
			states, err = d.Home.States(ctx, gated)
			if err != nil {
				return fail("home cross-check", err)
			}
		}
		for _, k := range gated {
			v := current[k]
			st := states[k]
			kind := "gated:" + st
			rec.Counts["home:"+orNone(st)]++
			if !due(k, kind) {
				continue
			}
			m := boardMsg(v.Board)
			switch st {
			case "ruled", "closed":
				m.ruled = append(m.ruled, v)
			case "open", "display":
				homeNote = append(homeNote, k)
				continue // mk already has the card: nothing to tell the coordinator
			default:
				m.noHome = append(m.noHome, v)
			}
			sentNow[k] = Sent{Kind: kind, At: now}
		}
		for _, v := range current {
			if v.Label == LabelNeedsSpec && due(v.Key, "needs-spec") {
				boardMsg(v.Board).needsSpec = append(boardMsg(v.Board).needsSpec, v)
				sentNow[v.Key] = Sent{Kind: "needs-spec", At: now}
			}
			if v.Label == LabelClose && due(v.Key, "close") {
				boardMsg(v.Board).closeIt = append(boardMsg(v.Board).closeIt, v)
				sentNow[v.Key] = Sent{Kind: "close", At: now}
			}
		}
	}

	// Ready dispatch (both modes): only newly-ready cards, only to a coordinator with room, only with quota headroom.
	var skipped []string
	if len(current) > 0 {
		ok, why, perr := true, "", error(nil)
		if d.Pool != nil {
			ok, why, perr = d.Pool.Headroom(ctx)
		}
		if perr != nil {
			return fail("quota headroom", perr)
		}
		inflight, ierr := d.Board.InFlight(ctx)
		if ierr != nil {
			return fail("in-flight count", ierr)
		}
		var ready []Verdict
		for _, v := range current {
			if v.Label == LabelReady && due(v.Key, "ready") {
				ready = append(ready, v)
			}
		}
		sort.Slice(ready, func(i, j int) bool {
			return prioRank(byKey[ready[i].Key].Priority) < prioRank(byKey[ready[j].Key].Priority) || (prioRank(byKey[ready[i].Key].Priority) == prioRank(byKey[ready[j].Key].Priority) && ready[i].Key < ready[j].Key)
		})
		room := map[string]int{}
		for _, v := range ready {
			if !ok {
				skipped = append(skipped, v.Key+" (quota: "+why+")")
				continue
			}
			if c := byKey[v.Key]; c.Working > 0 {
				skipped = append(skipped, v.Key+" (a live agent is already on it)")
				continue
			}
			r, seen := room[v.Board]
			if !seen {
				r = o.MaxInFlight - inflight[v.Board]
			}
			if r <= 0 {
				skipped = append(skipped, v.Key+fmt.Sprintf(" (%s has %d in flight)", v.Board, inflight[v.Board]))
				room[v.Board] = 0
				continue
			}
			room[v.Board] = r - 1
			boardMsg(v.Board).ready = append(boardMsg(v.Board).ready, v)
			sentNow[v.Key] = Sent{Kind: "ready", At: now}
		}
	}
	rec.Counts["ready_skipped"] = len(skipped)

	// Deliver. A dry run delivers nothing to coordinators; a board with no coordinator is reported to the vizier.
	var unrouted []string
	boards := make([]string, 0, len(msgs))
	for b, m := range msgs {
		if !m.empty() {
			boards = append(boards, b)
		}
	}
	sort.Strings(boards)
	delivered := map[string]bool{}
	for _, b := range boards {
		thr, cerr := d.Notify.Coordinator(ctx, b)
		if cerr != nil {
			return fail("find coordinator for "+b, cerr)
		}
		if thr == "" {
			unrouted = append(unrouted, b)
			continue
		}
		if o.DryRun {
			continue
		}
		text := msgs[b].text(o.Mode, now)
		if terr := d.Notify.Tell(ctx, thr, text); terr != nil {
			return fail("message "+thr+" ("+b+")", terr)
		}
		rec.Messaged = append(rec.Messaged, b+":"+thr)
		for _, v := range msgs[b].all() {
			delivered[v.Key] = true
		}
	}
	if !o.DryRun {
		for k, s := range sentNow {
			if delivered[k] {
				mem.Sent[k] = s
			}
		}
		if err := d.State.Save(mem); err != nil {
			return fail("save memory", err)
		}
	} else if o.Mode == Weekly {
		// A dry run still remembers verdicts, so the first real run does not pay for the triage twice.
		if err := d.State.Save(Memory{Verdicts: mem.Verdicts, Sent: mem.Sent, Seen: mem.Seen}); err != nil {
			return fail("save memory", err)
		}
	}

	digest := renderDigest(o, now, rec, msgs, boards, unrouted, skipped, homeNote)
	dpath := filepath.Join(d.State.Dir, "digests", now.Format("20060102T150405Z")+"-"+string(o.Mode)+".md")
	if err := writeAtomic(dpath, []byte(digest)); err != nil {
		return fail("write digest", err)
	}
	rec.Digest = dpath
	summary := fmt.Sprintf("Backlog run (%s%s): %d cards, %d triaged now; %s. Digest: %s",
		o.Mode, map[bool]string{true: ", DRY RUN: no coordinator was messaged", false: ""}[o.DryRun], rec.Counts["cards"], rec.Counts["triaged"], countLine(rec.Counts), dpath)
	if len(rec.Messaged) > 0 {
		summary += fmt.Sprintf("\nMessaged %d coordinator(s): %s", len(rec.Messaged), strings.Join(rec.Messaged, ", "))
	}
	if len(unrouted) > 0 {
		summary += "\nNo single coordinator thread for: " + strings.Join(unrouted, ", ")
	}
	if err := d.Notify.TellVizier(ctx, summary); err != nil {
		return finish("failed", "vizier message: "+err.Error(), err)
	}
	return finish("ok", "", nil)
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func prioRank(p string) int {
	switch p {
	case "urgent":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	}
	return 4
}

func countLine(c map[string]int) string {
	var parts []string
	for _, k := range []string{"label:ready", "label:needs-spec", "label:mk-gated", "label:close", "label:standing", "home:open", "home:ruled", "home:none", "ready_skipped"} {
		if c[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", strings.TrimPrefix(k, "label:"), c[k]))
		}
	}
	if len(parts) == 0 {
		return "nothing to report"
	}
	return strings.Join(parts, " ")
}
