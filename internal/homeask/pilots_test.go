package homeask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func enablePilot(t *testing.T, f *CardFiler, name string) {
	t.Helper()
	d, _ := f.stateDir()
	os.MkdirAll(d, 0700)
	s := fmt.Sprintf(`{"%s":{"enabled":true,"live_since":"2026-01-01T00:00:00Z","until":"2099-01-01T00:00:00Z"}}`, name)
	if err := os.WriteFile(filepath.Join(d, "pilots.json"), []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestPilotColdreadPrecedent(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "coldread")
	r := testReq()
	r.Type = "design"
	_, err := f.FileCard(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "Precedent:") {
		t.Fatalf("missing precedent must refuse with fix, got %v", err)
	}
}
func TestPilotAbsentLegacyBytes(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	r := testReq()
	r.NonBlocking = true
	f.Now = func() time.Time { return time.Date(2026, 1, 9, 12, 0, 0, 0, time.UTC) }
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	d, _ := f.stateDir()
	raw, err := os.ReadFile(filepath.Join(d, "filings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "delivered_at") || strings.Contains(string(raw), "filed_at") || strings.Contains(string(raw), "held_reason") {
		t.Fatal("pilot fields changed legacy bytes")
	}
}

type fakeColdReader struct {
	body string
	err  error
	text string
}

func (r *fakeColdReader) Read(_ context.Context, s string) ([]byte, error) {
	r.text = s
	return []byte(r.body), r.err
}
func TestPilotColdreadVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		urgent, fail bool
	}{
		{"clear", `{"decision":"Ship","options":{"yes":"Ship","no":"Wait"},"undefined_terms":[],"verdict":"clear"}`, false, false},
		{"missing", `{"decision":"Ship","options":{"yes":"Ship"},"undefined_terms":[],"verdict":"clear"}`, false, true},
		{"undefined", `{"decision":"Ship","options":{"yes":"Ship","no":"Wait"},"undefined_terms":["widget"],"verdict":"clear"}`, false, true},
		{"warn", `{"decision":"Ship","options":{},"undefined_terms":[],"verdict":"unclear"}`, true, false},
		{"unparsable", `not JSON`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			enablePilot(t, f, "coldread")
			reader := &fakeColdReader{body: tc.body}
			f.ColdReader = reader
			r := testReq()
			r.Urgent = tc.urgent
			_, e := f.FileCard(context.Background(), r)
			if (e != nil) != tc.fail {
				t.Fatal(e)
			}
			if reader.text == "" {
				t.Fatal("reader not called")
			}
		})
	}
}
func TestPilotColdreadUnavailable(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "coldread")
	f.ColdReader = &fakeColdReader{err: fmt.Errorf("not installed")}
	if _, e := f.FileCard(context.Background(), testReq()); e != nil {
		t.Fatal(e)
	}
}
func TestPilotLifecycle(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f.Now = func() time.Time { return now }
	if f.pilot("coldread") {
		t.Fatal("default on")
	}
	if e := f.SetPilot("coldread", true); e != nil {
		t.Fatal(e)
	}
	p, _ := f.Pilots()
	if p["coldread"].Until.Sub(p["coldread"].LiveSince) != 14*24*time.Hour {
		t.Fatal(p)
	}
	if !f.pilot("coldread") {
		t.Fatal("not on")
	}
	t.Setenv(EnvPilotsOff, "1")
	if f.pilot("coldread") {
		t.Fatal("off ignored")
	}
	t.Setenv(EnvPilotsOff, "")
	now = now.Add(14 * 24 * time.Hour)
	if f.pilot("coldread") {
		t.Fatal("expired on")
	}
	if e := f.SetPilot("coldread", false); e != nil {
		t.Fatal(e)
	}
}
func TestPilotColdreadLive(t *testing.T) {
	if os.Getenv("AUTARCH_TEST_LIVE_COLDREAD") != "1" {
		t.Skip("subscription smoke is opt-in")
	}
	r := testReq()
	r.Title = "Choose the example button colour"
	r.Ask["question"] = "Which background colour should the example button use?"
	r.Ask["options"] = []any{map[string]any{"id": "blue", "label": "Blue background"}, map[string]any{"id": "green", "label": "Green background"}}
	d := `{"title":"Choose the example button colour","question":"Which background colour should the example button use?","options":[{"id":"blue","label":"Blue background"},{"id":"green","label":"Green background"}]}`
	b, e := (ClaudeColdReader{Bin: os.Getenv("AUTARCH_TEST_CLAUDE_BIN")}).Read(context.Background(), r.Title+"\n"+d)
	t.Logf("model JSON: %s; error: %v", b, e)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := decodeColdRead(b); e != nil {
		t.Fatal(e)
	}
}
func TestPilotColdreadEnvironment(t *testing.T) {
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_USE_BEDROCK", "AWS_SECRET_ACCESS_KEY", "GOOGLE_APPLICATION_CREDENTIALS"} {
		t.Setenv(k, "fake-secret")
	}
	for _, v := range coldReadEnv() {
		if strings.Contains(v, "fake-secret") {
			t.Fatal("credential retained")
		}
	}
}
func TestPilotDaycapHolds(t *testing.T) {
	t.Setenv("AUTARCH_CARD_DAILY_CAP", "1")
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "daycap")
	r := testReq()
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	r.Key = "second"
	res, e := f.FileCard(context.Background(), r)
	if e != nil || res.Queued {
		t.Fatalf("second delivery must file: %+v %v", res, e)
	}
}
func TestPilotDaycapReleaseRankAndDay(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "1")
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	enablePilot(t, f, "daycap")
	now := time.Date(2026, 11, 7, 7, 59, 0, 0, time.UTC)
	f.Now = func() time.Time { return now }
	a := testReq()
	a.Key = "old"
	a.NonBlocking = true
	a.Blocks = nil
	if _, e := f.FileCard(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	now = now.Add(time.Second)
	b := testReq()
	b.Key = "unblocks-more"
	b.NonBlocking = true
	if _, e := f.FileCard(context.Background(), b); e != nil {
		t.Fatal(e)
	}
	groups, e := f.Release(context.Background())
	if e != nil || len(groups["other"]) != 2 || groups["other"][0].Key != a.Key {
		t.Fatal(groups, e)
	}
	now = now.Add(2 * time.Minute)
	groups, e = f.Release(context.Background())
	if e != nil || len(groups["other"]) != 0 {
		t.Fatal(groups, e)
	}
}
func TestPilotDaycapExemptAndWeeklyFirst(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "1")
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "daycap")
	f.WeeklyBudget = 1
	r := testReq()
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	r.Key = "over-week"
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	r.Key = "urgent"
	r.Urgent = true
	if res, e := f.FileCard(context.Background(), r); e != nil || res.Queued {
		t.Fatal(res, e)
	}
}
func TestPilotPacificDST(t *testing.T) {
	for _, s := range []string{"2026-11-01T08:30:00Z", "2026-11-01T09:30:00Z"} {
		v, _ := time.Parse(time.RFC3339, s)
		if pacificDay(v) != "2026-11-01" {
			t.Fatal(s)
		}
	}
}
func TestPilotLinkcheckDeadAtFiling(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "linkcheck")
	r := testReq()
	r.Ask["question"] = "Open https://invalid.invalid/preview ?"
	_, e := f.FileCard(context.Background(), r)
	if e == nil || !strings.Contains(e.Error(), "https://invalid.invalid/preview") {
		t.Fatalf("dead URL must refuse and name URL: %v", e)
	}
}
func TestPilotMetricsHeld(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "linkcheck")
	r := testReq()
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	d, _ := f.stateDir()
	b, e := os.ReadFile(filepath.Join(d, "pilots-metrics.jsonl"))
	if e != nil || !strings.Contains(string(b), `"event":"held"`) {
		t.Fatalf("held metric missing: %s %v", b, e)
	}
}
func TestPilotColdreadForcedMissing(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "coldread")
	f.ColdReader = ClaudeColdReader{Bin: filepath.Join(t.TempDir(), "missing-claude")}
	r := testReq()
	res, e := f.FileCard(context.Background(), r)
	if e != nil || res.ID == "" {
		t.Fatal(res, e)
	}
	d, _ := f.stateDir()
	b, e := os.ReadFile(filepath.Join(d, "pilots-metrics.jsonl"))
	if e != nil || !strings.Contains(string(b), `"result":"skip"`) {
		t.Fatal(string(b), e)
	}
	t.Logf("skip metric: %s", b)
}
func TestPilotDaycapConcurrent(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "3")
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	enablePilot(t, f, "daycap")
	var wg sync.WaitGroup
	var mu sync.Mutex
	delivered, held := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := testReq()
			r.Key = fmt.Sprintf("parallel-%d", i)
			res, e := f.FileCard(context.Background(), r)
			if e != nil {
				t.Error(e)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if res.Queued {
				held++
			} else {
				delivered++
			}
		}(i)
	}
	wg.Wait()
	if delivered != 8 || held != 0 {
		t.Fatal(delivered, held)
	}
}
func TestPilotReportPacificAndPrivacy(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "linkcheck")
	r := testReq()
	r.NonBlocking = true
	f.Now = func() time.Time { return time.Date(2026, 11, 2, 7, 59, 0, 0, time.UTC) }
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	if e := f.PilotReport(&b, "2026-11-01"); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(b.String(), "2026-11-01 0 1 0") || !strings.Contains(b.String(), "not visible to the filer") {
		t.Fatal(b.String())
	}
}

func TestPilotReportIncludesStoredPicksAndDeduplicatesLegacyRows(t *testing.T) {
	d := t.TempDir()
	metric := `{"time":"2026-11-02T12:00:00Z","event":"picked","event_id":"pick:example","option_kind":"needs-context","pasted_error":true}`
	if err := os.WriteFile(filepath.Join(d, "pilots-metrics.jsonl"), []byte(metric+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	f := &CardFiler{StateDir: d, Run: func(_ context.Context, _ []string, args ...string) BBResult {
		if strings.Join(args, " ") != "plugin rpc call autarch pilotMetrics --json" {
			t.Fatalf("unexpected call %v", args)
		}
		calls++
		return BBResult{Stdout: []byte(`{"metrics":[` + metric + `]}`)}
	}}
	var out bytes.Buffer
	if err := f.PilotReport(&out, "2026-11-01"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(out.String(), "2026-11-02 0 0 0 1 1") {
		t.Fatal(out.String(), calls)
	}
	if err := os.Remove(filepath.Join(d, "pilots-metrics.jsonl")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := f.PilotReport(&out, "2026-11-01"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "2026-11-02 0 0 0 1 1") {
		t.Fatal(out.String())
	}
}

func TestPilotReportLabelsUnavailableStoredMetrics(t *testing.T) {
	for _, response := range []BBResult{{Err: fmt.Errorf("offline")}, {Stdout: []byte(`{}`)}} {
		f := &CardFiler{StateDir: t.TempDir(), Run: func(context.Context, []string, ...string) BBResult { return response }}
		var out bytes.Buffer
		if err := f.PilotReport(&out, ""); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Plugin pick metrics unavailable; report includes local metrics only.") {
			t.Fatal(out.String())
		}
	}
}

func TestPilotReportMalformedRPCTimeKeepsLocalCounts(t *testing.T) {
	d := t.TempDir()
	local := `{"time":"2026-11-02T12:00:00Z","event":"delivered"}`
	if err := os.WriteFile(filepath.Join(d, "pilots-metrics.jsonl"), []byte(local+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &CardFiler{StateDir: d, Run: func(context.Context, []string, ...string) BBResult {
		// A valid row before the bad one must not leak partial RPC counts into the report.
		return BBResult{Stdout: []byte(`{"metrics":[{"time":"2026-11-02T12:00:00Z","event":"picked","option_kind":"needs-context","pasted_error":true},{"time":"not-a-time","event":"picked"}]}`)}
	}}
	var out bytes.Buffer
	if err := f.PilotReport(&out, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Plugin pick metrics unavailable; report includes local metrics only.") ||
		!strings.Contains(out.String(), "2026-11-02 1 0 0 0 0 0 0 0 0 0\n") {
		t.Fatal(out.String())
	}
}

func TestPilotAbsentExactLegacyState(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	r := testReq()
	r.NonBlocking = true
	f.Now = func() time.Time { return time.Date(2026, 1, 9, 12, 0, 0, 0, time.UTC) }
	res, e := f.FileCard(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	wantResult := `{"queued":true,"card":"","request":"0b9d1c6e-0000-4000-8000-000000000001","replay":false}`
	result, _ := json.Marshal(res)
	if string(result) != wantResult {
		t.Fatalf("result bytes changed: %s", result)
	}
	// Original filingEntry fields and order, independently pinned before pilot additions.
	type legacyEntry struct {
		DeliveredCard    string      `json:"delivered_card,omitempty"`
		DeliveredKey     string      `json:"delivered_key,omitempty"`
		DeliveredProject string      `json:"delivered_project,omitempty"`
		CreationKey      string      `json:"creation_key,omitempty"`
		Uncharged        bool        `json:"uncharged,omitempty"`
		Refunded         bool        `json:"refunded,omitempty"`
		TerminalReason   string      `json:"terminal_reason,omitempty"`
		Request          CardRequest `json:"request"`
		Identity         string      `json:"identity"`
		Project          string      `json:"project"`
		Week             string      `json:"week"`
		Attempted        bool        `json:"attempted,omitempty"`
		Held             bool        `json:"held"`
		Supersedes       string      `json:"supersedes,omitempty"`
		Update           bool        `json:"update"`
	}
	ident, _ := RequestIdentity(r)
	want, _ := json.Marshal(map[string]legacyEntry{r.Key: {Request: r, Identity: ident, Project: "autarch", Week: "2026-01-08", Held: true}})
	d, _ := f.stateDir()
	got, _ := os.ReadFile(filepath.Join(d, "filings.json"))
	if !bytes.Equal(got, want) {
		t.Fatalf("legacy state bytes changed\nwant %s\ngot %s", want, got)
	}
	if _, e := os.Stat(filepath.Join(d, "pilots-metrics.jsonl")); !os.IsNotExist(e) {
		t.Fatal("absent pilots created metrics", e)
	}
}
func TestPilotColdreadRefusesAPIAuthentication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	script := `#!/bin/sh
case " $* " in
 *" auth status "*) printf '%s' '{"loggedIn":true,"authMethod":"apiKey","subscriptionType":null}' ;;
 *) printf '%s' '{"decision":"Choose","options":{},"undefined_terms":[],"verdict":"clear"}' ;;
esac
`
	if e := os.WriteFile(bin, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := (ClaudeColdReader{Bin: bin}).Read(context.Background(), "made-up card"); e == nil {
		t.Fatal("API authentication must never invoke a model")
	}
}
func TestPilotNullConfigIsError(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	d, _ := f.stateDir()
	os.MkdirAll(d, 0700)
	os.WriteFile(filepath.Join(d, "pilots.json"), []byte("null"), 0600)
	if _, e := f.Pilots(); e == nil {
		t.Fatal("null pilot config must return error rather than panic on start")
	}
}
func TestPilotDaycapRecoversItsOwnDelivery(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "1")
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	enablePilot(t, f, "daycap")
	r := testReq()
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	fail := true
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if fail && len(args) > 1 && strings.Join(args[:2], " ") == "tasks comment" {
			fail = false
			return BBResult{Code: 1, Stderr: []byte("temporary routing failure")}
		}
		return bb.run(ctx, env, args...)
	}
	if _, e := f.Release(context.Background()); e == nil {
		t.Fatal("fixture did not fail routing")
	}
	groups, e := f.Release(context.Background())
	if e != nil || len(groups["other"]) != 1 || bb.count("tasks", "create") != 1 {
		t.Fatalf("own delivery reservation prevented recovery: %+v %v", groups, e)
	}
}
func TestPilotDaycapDoesNotCountNotificationCacheAsNewDelivery(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "1")
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "daycap")
	r := testReq()
	s := filingState{"old-card": {Request: r, Uncharged: true, DeliveredCard: "existing-card", DeliveredAt: f.clock().Format(time.RFC3339Nano)}}
	full, e := f.dailyFull(s, r)
	if e != nil || full {
		t.Fatalf("notification-only entry must not consume a new delivery slot: %v %v", full, e)
	}
}
