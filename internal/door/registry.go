package door

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// The registry diff (plan WI-6). mk kept the tmux session list a second time,
// by hand, in an Apple Note in the same line format the session names use.
// This file reads that note and says where the two copies disagree. It is a
// one-time migration aid: it shows drift, reconciles nothing, persists
// nothing.

// ParseRegistryNote reads mk's note format: section lines beginning with "——"
// (naming an emulator) and blank lines are skipped; every other line is one
// seat, parsed exactly as a tmux session name would be.
func ParseRegistryNote(r io.Reader) ([]Seat, error) {
	var seats []Seat
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "——") || strings.HasPrefix(line, "--") {
			continue
		}
		seats = append(seats, ParseSeat(line))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("registry note: %w", err)
	}
	return seats, nil
}

// Drift is one disagreement between the note and the live seats.
type Drift struct {
	Kind  string // "stale id" | "renamed" | "no seat" | "not in note"
	Topic string
	Note  string // what the note says
	Live  string // what tmux says
}

// DiffRegistry compares note seats to live threads, by resume id first and by
// topic second:
//
//	same id, different topic      → "renamed"     (rakes-of-the-new-sun → rakes-of-the-new-book)
//	same topic, different id      → "stale id"    (ushas/bridger 21434d6f vs ef9ad21a)
//	note seat with no live match  → "no seat"     (shadewright, garden-salon, the no-id topics)
//	live thread with no note seat → "not in note" (ryan, spellswords@hermes, 28, 30, kimifork, mobile)
//
// Note lines sharing one id (autarch / estate / cujgel) are one seat with
// several topics, not drift. Order: the note's order, then unmatched live
// threads in their given order.
func DiffRegistry(note []Seat, live []Thread) []Drift {
	liveByID := make(map[string]Thread)
	liveByTopic := make(map[string]Thread)
	for _, th := range live {
		if th.Seat.ResumeID != "" {
			liveByID[th.Seat.ResumeID] = th
		}
		liveByTopic[th.Seat.Topic] = th
	}
	matched := make(map[string]bool) // live session names claimed by a note seat

	type noteSeat struct {
		id     string
		topics []string
	}
	byID := make(map[string]*noteSeat)
	var withID []*noteSeat
	var noID []Seat
	for _, s := range note {
		if s.ResumeID == "" {
			noID = append(noID, s)
			continue
		}
		ns, ok := byID[s.ResumeID]
		if !ok {
			ns = &noteSeat{id: s.ResumeID}
			byID[s.ResumeID] = ns
			withID = append(withID, ns)
		}
		ns.topics = append(ns.topics, s.Topic)
	}

	var drifts []Drift
	for _, ns := range withID {
		label := strings.Join(ns.topics, " / ")
		if th, ok := liveByID[ns.id]; ok {
			matched[th.Session] = true
			if !containsString(ns.topics, th.Seat.Topic) {
				drifts = append(drifts, Drift{Kind: "renamed", Topic: label, Note: label, Live: th.Seat.Topic})
			}
			continue
		}
		found := false
		for _, topic := range ns.topics {
			th, ok := liveByTopic[topic]
			if !ok {
				continue
			}
			matched[th.Session] = true
			drifts = append(drifts, Drift{Kind: "stale id", Topic: topic, Note: shortID(ns.id), Live: shortIDOrNone(th.Seat.ResumeID)})
			found = true
			break
		}
		if !found {
			drifts = append(drifts, Drift{Kind: "no seat", Topic: label, Note: shortID(ns.id), Live: "none"})
		}
	}
	for _, s := range noID {
		if th, ok := liveByTopic[s.Topic]; ok {
			matched[th.Session] = true
			continue
		}
		drifts = append(drifts, Drift{Kind: "no seat", Topic: s.Topic, Note: "no id", Live: "none"})
	}
	for _, th := range live {
		if !matched[th.Session] {
			drifts = append(drifts, Drift{Kind: "not in note", Topic: th.Seat.Topic, Note: "absent", Live: th.Session})
		}
	}
	// Most actionable first: a stale id or a rename is a line mk would fix
	// today; a seatless topic is a decision; a live seat the note never had
	// is news. Within a kind the note's own order holds, so the block under
	// its line cap shows what matters before what merely differs.
	sort.SliceStable(drifts, func(i, j int) bool { return driftRank[drifts[i].Kind] < driftRank[drifts[j].Kind] })
	return drifts
}

var driftRank = map[string]int{"stale id": 0, "renamed": 1, "not in note": 2, "no seat": 3}

// DriftLine is one drift as a plain line, shared by the threads screen and the
// threads subcommand.
func DriftLine(d Drift) string {
	return fmt.Sprintf("%s: %s · note %s · live %s", d.Kind, d.Topic, d.Note, d.Live)
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// shortID is the first eight characters of a resume id, the length mk's eye
// already uses to tell threads apart.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func shortIDOrNone(id string) string {
	if id == "" {
		return "no id in name"
	}
	return shortID(id)
}
