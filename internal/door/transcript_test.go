package door

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindTranscriptPicksNewest(t *testing.T) {
	root := t.TempDir()
	id := "abc123ef"
	oldPath := filepath.Join(root, "-e-old", id+".jsonl")
	newPath := filepath.Join(root, "-e-new", id+".jsonl")
	touch(t, oldPath, time.Now().Add(-time.Hour))
	touch(t, newPath, time.Now())

	got, err := FindTranscript(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if got != newPath {
		t.Fatalf("FindTranscript = %q, want newest %q", got, newPath)
	}

	if _, err := FindTranscript(root, "nope"); err == nil {
		t.Fatal("an id with no transcript must be an error")
	}
}

// TestLastTurnIgnoresBookkeepingTail is the probe's own finding made
// executable: every transcript's mtime reads as today because of
// untimestamped bookkeeping rows tacked on after the last real turn. The
// last conversational entry, not the file's tail shape, is the answer.
func TestLastTurnIgnoresBookkeepingTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	lines := []string{
		`{"type":"user","timestamp":"2026-08-25T09:00:00Z","message":"hi"}`,
		`{"type":"assistant","timestamp":"2026-08-25T10:00:00Z","message":"hello"}`,
		`{"type":"bridge-session","data":"x"}`,
		`{"type":"mode","mode":"default"}`,
		`{"type":"permission-mode","mode":"default"}`,
		`{"type":"last-prompt","prompt":"x"}`,
		`{"type":"atis-latch","latch":true}`,
		`{"type":"system/init","subtype":"init"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LastTurn(path, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("LastTurn = %v, want %v (bookkeeping tail must not count)", got, want)
	}

	empty := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(empty, []byte(`{"type":"bridge-session","data":"x"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = LastTurn(empty, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Fatalf("an all-bookkeeping tail must read zero time, got %v", got)
	}
}

// TestGardensLongestRootWins is WI-3's attribution contract: a path under a
// nested project (Autarch inside Sylveste, the way this repo actually sits)
// credits Autarch, and a path directly under Sylveste that Autarch's root
// does not contain credits Sylveste.
func TestGardensLongestRootWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	content := `{"type":"user","timestamp":"2026-09-01T00:00:00Z","message":"working in /e/Sylveste/apps/Autarch/x today"}` + "\n" +
		`{"type":"user","timestamp":"2026-09-01T00:01:00Z","message":"also touched /e/Sylveste/y over here"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	projects := []Project{
		{Name: "Sylveste", Root: "/e/Sylveste"},
		{Name: "Autarch", Root: "/e/Sylveste/apps/Autarch"},
	}
	hits, err := Gardens(path, projects, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, h := range hits {
		got[h.Name] = h.Mentions
	}
	if got["Autarch"] != 1 || got["Sylveste"] != 1 {
		t.Fatalf("gardens = %+v (want Autarch 1, Sylveste 1)", hits)
	}
}

// TestGardensNoPathsIsEmptyNotError is the taxes case from the probe: a
// non-code garden's transcript mentions no project path at all, and that is
// a real, empty measurement -- never an error.
func TestGardensNoPathsIsEmptyNotError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	body := `{"type":"user","timestamp":"2026-09-01T00:00:00Z","message":"just talking about taxes, no code here"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	projects := []Project{{Name: "Sylveste", Root: "/e/Sylveste"}}
	hits, err := Gardens(path, projects, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("want no gardens for a non-code transcript, got %+v", hits)
	}
}
