package backlogrun

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeBoard struct {
	cards    []Card
	inflight map[string]int
}

func (f *fakeBoard) Cards(context.Context) ([]Card, error)            { return f.cards, nil }
func (f *fakeBoard) InFlight(context.Context) (map[string]int, error) { return f.inflight, nil }

type fakeTriage struct {
	labels map[string]string
	calls  int
	err    error
}

func (f *fakeTriage) Triage(_ context.Context, cs []Card) ([]Verdict, Seat, error) {
	f.calls++
	if f.err != nil {
		return nil, Seat{}, f.err
	}
	var out []Verdict
	for _, c := range cs {
		out = append(out, Verdict{Key: c.Key, Label: f.labels[c.Key], Reason: "r " + c.Key})
	}
	return out, Seat{Role: "coordination", ProfileRef: "p", Backend: "claude", Model: "m", Effort: "low", PolicyHash: "h"}, nil
}

type fakeHome map[string]string

func (f fakeHome) States(_ context.Context, keys []string) (map[string]string, error) {
	o := map[string]string{}
	for _, k := range keys {
		if s, ok := f[k]; ok {
			o[k] = s
		}
	}
	return o, nil
}

type fakeNotify struct {
	coord  map[string]string
	told   map[string][]string
	vizier []string
}

func (f *fakeNotify) Coordinator(_ context.Context, b string) (string, error) { return f.coord[b], nil }
func (f *fakeNotify) Tell(_ context.Context, t, m string) error {
	if f.told == nil {
		f.told = map[string][]string{}
	}
	f.told[t] = append(f.told[t], m)
	return nil
}
func (f *fakeNotify) TellVizier(_ context.Context, m string) error {
	f.vizier = append(f.vizier, m)
	return nil
}

type fakePool struct{ ok bool }

func (f fakePool) Headroom(context.Context) (bool, string, error) { return f.ok, "low", nil }

func card(key, board string) Card {
	return Card{Key: key, Board: board, Title: key, Status: "backlog", Priority: "high", UpdatedAt: "2026-10-15T00:00:00Z"}
}

func setup(t *testing.T) (Deps, *fakeBoard, *fakeTriage, *fakeNotify) {
	t.Helper()
	b := &fakeBoard{cards: []Card{card("AUTA-1", "AUTA"), card("AUTA-2", "AUTA"), card("AUTA-3", "AUTA"), card("AUTA-4", "AUTA")}, inflight: map[string]int{}}
	tr := &fakeTriage{labels: map[string]string{"AUTA-1": LabelReady, "AUTA-2": LabelNeedsSpec, "AUTA-3": LabelMKGated, "AUTA-4": LabelMKGated}}
	n := &fakeNotify{coord: map[string]string{"AUTA": "thr_a"}}
	return Deps{Board: b, Triage: tr, Home: fakeHome{"AUTA-3": "ruled", "AUTA-4": "open"}, Notify: n, Pool: fakePool{true}, State: State{Dir: t.TempDir()}}, b, tr, n
}

var t0 = time.Date(2026, 10, 19, 14, 0, 0, 0, time.UTC)

func opts(m Mode) Options { return Options{Mode: m, Now: func() time.Time { return t0 }} }

func TestWeeklyRoutesEachKindAndSkipsOpenHome(t *testing.T) {
	d, _, _, n := setup(t)
	res, err := Run(context.Background(), d, opts(Weekly))
	if err != nil || res.Receipt.Outcome != "ok" {
		t.Fatalf("run: %v %+v", err, res.Receipt)
	}
	if len(n.told["thr_a"]) != 1 {
		t.Fatalf("want one message per coordinator, got %d", len(n.told["thr_a"]))
	}
	m := n.told["thr_a"][0]
	for _, want := range []string{"\nREADY (", "AUTA-1", "NEEDS-SPEC", "AUTA-2", "ALREADY RULED", "AUTA-3"} {
		if !strings.Contains(m, want) {
			t.Errorf("message lacks %q:\n%s", want, m)
		}
	}
	if strings.Contains(m, "AUTA-4") {
		t.Errorf("a card with an open Home card must not be nagged:\n%s", m)
	}
	if len(n.vizier) != 1 || !strings.Contains(n.vizier[0], res.Digest) {
		t.Errorf("want one vizier message naming the digest, got %v", n.vizier)
	}
}

func TestNoNagWithinRepeatWindowAndUnchangedCardsNotRetriaged(t *testing.T) {
	d, _, tr, n := setup(t)
	if _, err := Run(context.Background(), d, opts(Weekly)); err != nil {
		t.Fatal(err)
	}
	calls := tr.calls
	if _, err := Run(context.Background(), d, opts(Weekly)); err != nil {
		t.Fatal(err)
	}
	if tr.calls != calls {
		t.Errorf("unchanged cards were triaged again")
	}
	if len(n.told["thr_a"]) != 1 {
		t.Errorf("second run repeated a message: %d", len(n.told["thr_a"]))
	}
}

func TestChangedCardIsRetriaged(t *testing.T) {
	d, b, tr, _ := setup(t)
	Run(context.Background(), d, opts(Weekly))
	calls := tr.calls
	b.cards[1].UpdatedAt = "2026-10-18T00:00:00Z"
	Run(context.Background(), d, opts(Weekly))
	if tr.calls != calls+1 {
		t.Errorf("changed card not re-triaged")
	}
}

func TestDailyOnlyReadyAndRespectsInFlight(t *testing.T) {
	d, b, _, n := setup(t)
	Run(context.Background(), d, opts(Weekly))
	// New ready card appears after the weekly run; the daily run does not triage, so it cannot know it. A ready
	// card from the weekly run that was already sent is not repeated.
	n.told = nil
	if _, err := Run(context.Background(), d, opts(Daily)); err != nil {
		t.Fatal(err)
	}
	if len(n.told) != 0 {
		t.Errorf("daily repeated already-sent cards: %v", n.told)
	}
	// Fresh state, three in flight: no message.
	d2, b2, _, n2 := setup(t)
	b2.inflight["AUTA"] = 3
	_ = b
	Run(context.Background(), d2, opts(Weekly))
	for _, m := range n2.told["thr_a"] {
		if strings.Contains(m, "\nREADY (") {
			t.Errorf("ready card sent to a coordinator with 3 in flight:\n%s", m)
		}
	}
}

func TestLowHeadroomSkipsReadyOnly(t *testing.T) {
	d, _, _, n := setup(t)
	d.Pool = fakePool{false}
	res, _ := Run(context.Background(), d, opts(Weekly))
	m := strings.Join(n.told["thr_a"], "\n")
	if strings.Contains(m, "\nREADY (") || !strings.Contains(m, "NEEDS-SPEC") {
		t.Errorf("low headroom should drop only ready:\n%s", m)
	}
	if res.Receipt.Counts["ready_skipped"] != 1 {
		t.Errorf("skip not counted: %v", res.Receipt.Counts)
	}
}

func TestDryRunMessagesOnlyTheVizierButWritesDigest(t *testing.T) {
	d, _, _, n := setup(t)
	res, err := Run(context.Background(), d, Options{Mode: Weekly, DryRun: true, Now: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	if len(n.told) != 0 || len(n.vizier) != 1 {
		t.Errorf("dry run: told=%v vizier=%d", n.told, len(n.vizier))
	}
	if b, _ := os.ReadFile(res.Digest); !strings.Contains(string(b), "AUTA-1") {
		t.Errorf("digest missing the ready card")
	}
	// The dry run does not mark anything as sent, so the real run still delivers.
	if _, err := Run(context.Background(), d, opts(Weekly)); err != nil || len(n.told["thr_a"]) != 1 {
		t.Errorf("real run after dry run did not deliver: %v", err)
	}
}

func TestPausedDoesNothingButReceipt(t *testing.T) {
	d, _, tr, n := setup(t)
	os.WriteFile(d.State.PauseFile(), nil, 0o600)
	res, _ := Run(context.Background(), d, opts(Weekly))
	if res.Receipt.Outcome != "paused" || tr.calls != 0 || len(n.vizier) != 0 || len(n.told) != 0 {
		t.Errorf("paused run acted: %+v", res.Receipt)
	}
}

func TestTriageFailureFailsClosedAndTellsVizier(t *testing.T) {
	d, _, tr, n := setup(t)
	tr.err = errors.New("no routing seat")
	res, err := Run(context.Background(), d, opts(Weekly))
	if err == nil || res.Receipt.Outcome != "failed" {
		t.Fatalf("want failure, got %v %+v", err, res.Receipt)
	}
	if len(n.told) != 0 || len(n.vizier) != 1 || !strings.Contains(n.vizier[0], "FAILED") {
		t.Errorf("told=%v vizier=%v", n.told, n.vizier)
	}
}

func TestReceiptAppended(t *testing.T) {
	d, _, _, _ := setup(t)
	Run(context.Background(), d, opts(Weekly))
	b, err := os.ReadFile(filepath.Join(d.State.Dir, "receipts.jsonl"))
	if err != nil || !strings.Contains(string(b), `"outcome":"ok"`) || !strings.Contains(string(b), `"profile_ref":"p"`) {
		t.Errorf("receipt: %v %s", err, b)
	}
}

func TestParseVerdicts(t *testing.T) {
	cs := []Card{card("A-1", "A"), card("A-2", "A")}
	vs, err := ParseVerdicts("sure\n```json\n[{\"key\":\"A-1\",\"label\":\"ready\",\"reason\":\"x\"},{\"key\":\"ZZ-9\",\"label\":\"ready\"}]\n```", cs)
	if err != nil || len(vs) != 1 || vs[0].Key != "A-1" {
		t.Errorf("%v %v", vs, err)
	}
	if _, err := ParseVerdicts(`[{"key":"A-1","label":"banana"}]`, cs); err == nil {
		t.Error("unknown label accepted")
	}
	if _, err := ParseVerdicts("no array", cs); err == nil {
		t.Error("no array accepted")
	}
}

func TestHeadroom(t *testing.T) {
	ok, _, err := headroomFrom([]byte(`{"accepting":true,"accounts":[{"enabled":true,"fiveHourUtilization":0.2,"sevenDayUtilization":0.4,"heldUntil":null}]}`))
	if err != nil || !ok {
		t.Errorf("roomy: %v %v", ok, err)
	}
	ok, _, _ = headroomFrom([]byte(`{"accepting":true,"accounts":[{"enabled":true,"fiveHourUtilization":0.9,"heldUntil":null}]}`))
	if ok {
		t.Error("90 percent account counted as headroom")
	}
	if _, _, err := headroomFrom([]byte(`{}`)); err == nil {
		t.Error("missing accepting field accepted")
	}
}

func TestHomeIsReadOnlyAndPicksMostLiveState(t *testing.T) {
	db := filepath.Join(t.TempDir(), "data.db")
	w, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE cards (task_id TEXT, card_key TEXT, state TEXT, title TEXT, deleted_at TEXT)`,
		`INSERT INTO cards VALUES ('1','AUTA-9','ruled','t',NULL),('2','AUTA-9','open','t',NULL),('3','AUTA-8','closed','t',NULL),('4','AUTA-7','open','t','2026-01-01')`,
	} {
		if _, err := w.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	h := Home{DB: db}
	got, err := h.States(context.Background(), []string{"AUTA-9", "AUTA-8", "AUTA-7", "AUTA-6"})
	if err != nil || got["AUTA-9"] != "open" || got["AUTA-8"] != "closed" || len(got) != 2 {
		t.Errorf("states: %v %v", got, err)
	}
	ro, _ := sql.Open("sqlite", "file:"+db+"?mode=ro")
	defer ro.Close()
	if _, err := ro.Exec(`DELETE FROM cards`); err == nil {
		t.Error("read-only connection accepted a write")
	}
	if _, err := (Home{DB: filepath.Join(t.TempDir(), "missing.db")}).States(context.Background(), []string{"A-1"}); err == nil {
		t.Error("missing database must be an error, not created")
	}
}
