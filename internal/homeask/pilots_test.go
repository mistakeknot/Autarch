package homeask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	f.Now = func() time.Time { return time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC) }
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
	d, _, _ := r.Description()
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
