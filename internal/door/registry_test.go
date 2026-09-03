package door

import (
	"os"
	"testing"
)

// threadsFromNames seats live threads from bare tmux names, the way the diff
// sees them.
func threadsFromNames(names ...string) []Thread {
	out := make([]Thread, 0, len(names))
	for _, n := range names {
		out = append(out, Thread{Session: n, Seat: ParseSeat(n)})
	}
	return out
}

func findDrift(drifts []Drift, kind, topic string) (Drift, bool) {
	for _, d := range drifts {
		if d.Kind == kind && d.Topic == topic {
			return d, true
		}
	}
	return Drift{}, false
}

func TestDiffRegistryFindsTheThreeKnownDrifts(t *testing.T) {
	f, err := os.Open("testdata/session-note.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	note, err := ParseRegistryNote(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(note) != 52 {
		t.Fatalf("the note has 52 topic lines, parsed %d", len(note))
	}

	// The live side is the probe's tmux table, reduced to the seats the
	// known drifts need plus two that must produce none.
	live := threadsFromNames(
		"iterm[autarch - 5920c9b1-6a3f-4a7d-8566-e6067aaeaf01",
		"iterm[concordance - fca9cfa0-60ee-460d-ad2b-10688853d70c",
		"iterm[ushas/bridger - ef9ad21a-0965-45cb-b4bf-fda7f5a358b3",
		"iterm[]rakes-of-the-new-book - 5c7a44a3-4bff-461e-9ad2-06f0f7c7e18a",
		"rio]solwend - 21cc6bd2-139f-4127-9c2d-7ea821858209",
		"rio[ryan",
	)
	drifts := DiffRegistry(note, live)

	d, ok := findDrift(drifts, "stale id", "ushas/bridger")
	if !ok || d.Note != "21434d6f" || d.Live != "ef9ad21a" {
		t.Fatalf("ushas/bridger must read as a stale id (note 21434d6f, live ef9ad21a): %+v %v", d, ok)
	}
	d, ok = findDrift(drifts, "renamed", "rakes-of-the-new-sun")
	if !ok || d.Live != "rakes-of-the-new-book" {
		t.Fatalf("rakes must read as renamed to rakes-of-the-new-book: %+v %v", d, ok)
	}
	if _, ok := findDrift(drifts, "no seat", "shadewright"); !ok {
		t.Fatal("shadewright has a transcript and no tmux seat: must read as no seat")
	}
	d, ok = findDrift(drifts, "no seat", "tldrs")
	if !ok || d.Note != "no id" {
		t.Fatalf("a no-id topic with no live seat must read as no seat with note 'no id': %+v %v", d, ok)
	}
	if _, ok := findDrift(drifts, "not in note", "ryan"); !ok {
		t.Fatal("a live seat the note never listed must read as not in note")
	}
	for _, d := range drifts {
		for _, quiet := range []string{"autarch", "estate", "cujgel", "concordance", "solwend"} {
			if d.Topic == quiet || d.Live == quiet {
				t.Fatalf("%s is in agreement between note and tmux and must produce no drift: %+v", quiet, d)
			}
		}
	}
}

func TestDiffRegistrySharedIdIsNotDrift(t *testing.T) {
	id := "5920c9b1-6a3f-4a7d-8566-e6067aaeaf01"
	note := []Seat{ParseSeat("iterm[autarch - " + id), ParseSeat("iterm[estate - " + id), ParseSeat("[cujgel - " + id)}

	if drifts := DiffRegistry(note, threadsFromNames("iterm[autarch - "+id)); len(drifts) != 0 {
		t.Fatalf("three note topics on one live id are one seat, not drift: %+v", drifts)
	}
	drifts := DiffRegistry(note, threadsFromNames("iterm[hud - "+id))
	if len(drifts) != 1 || drifts[0].Kind != "renamed" || drifts[0].Live != "hud" {
		t.Fatalf("the same id under a topic the note never used is one rename: %+v", drifts)
	}
}
