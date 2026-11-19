package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mistakeknot/autarch/internal/homeask"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseUnknownProjectReportsFailureAndContinues(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	f := &homeask.CardFiler{Now: filingTestNow, Run: s.run, StateDir: t.TempDir(), LockDir: t.TempDir()}
	var ask map[string]any
	raw, err := os.ReadFile(writeAsk(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &ask); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"bad", "good"} {
		project := "P1"
		if key == "bad" {
			project = "missing-project"
		}
		r := homeask.CardRequest{Project: project, Title: "Choose?", Ask: ask, Key: key, Thread: "thr_x", NonBlocking: true}
		if _, err := f.FileCard(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	// The missing board remains unavailable while the other request can release.
	f.Run = func(ctx context.Context, env []string, args ...string) homeask.BBResult {
		if len(args) >= 5 && strings.Join(args[:3], " ") == "tasks label list" && args[4] == "missing-project" {
			return homeask.BBResult{Code: 1, Stderr: []byte("project not found: missing-project")}
		}
		return s.run(ctx, env, args...)
	}
	oldF, oldC := newCardFiler, checkFilingPreconditions
	t.Cleanup(func() { newCardFiler = oldF; checkFilingPreconditions = oldC })
	newCardFiler = func() *homeask.CardFiler { return f }
	checkFilingPreconditions = func(string, string) error { return nil }
	cmd := needsMkReleaseCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("missing project must be a reported request fault: %v; %s", err, &out)
	}
	var report struct {
		Groups   map[string][]homeask.CardResult `json:"groups"`
		Failures map[string]string               `json:"failures"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Groups["other"]) != 1 || !strings.Contains(report.Failures["bad"], "missing-project") || !strings.Contains(report.Failures["bad"], "no longer exists in bb") || strings.Contains(report.Failures["bad"], "home is down") {
		t.Fatal(out.String())
	}
	raw, err = os.ReadFile(filepath.Join(f.StateDir, "filings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]struct {
		Held bool `json:"held"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if entry, ok := state["bad"]; !ok || !entry.Held {
		t.Fatalf("lost held request: %s", raw)
	}
}

func TestReleasePrintsDeliveredGroupsAndKeyedFailures(t *testing.T) {
	s := &stubBB{}
	f := &homeask.CardFiler{Now: filingTestNow, Run: s.run, LockDir: t.TempDir()}
	var ask map[string]any
	raw, _ := os.ReadFile(writeAsk(t))
	json.Unmarshal(raw, &ask)
	r := homeask.CardRequest{Project: "P1", Title: "Choose?", Ask: ask, Key: "good", Thread: "thr_x", NonBlocking: true}
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "flow.png")
	os.WriteFile(p, []byte("image"), 0600)
	r.Key = "bad"
	r.Attachments = []string{p}
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	os.Remove(p)
	oldF, oldC := newCardFiler, checkFilingPreconditions
	t.Cleanup(func() { newCardFiler = oldF; checkFilingPreconditions = oldC })
	newCardFiler = func() *homeask.CardFiler { return f }
	checkFilingPreconditions = func(string, string) error { return nil }
	cmd := needsMkReleaseCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("per-entry failures must not fail the release timer: %v", err)
	}
	var report struct {
		Groups   map[string][]homeask.CardResult `json:"groups"`
		Failures map[string]string               `json:"failures"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("missing report: %s: %v", out.String(), err)
	}
	if len(report.Groups["other"]) != 1 || report.Failures["bad"] == "" {
		t.Fatal(out.String())
	}
}

func TestReleaseStillFailsWhenStateCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "filings.json"), []byte("{broken"), 0600); e != nil {
		t.Fatal(e)
	}
	old := newCardFiler
	t.Cleanup(func() { newCardFiler = old })
	newCardFiler = func() *homeask.CardFiler { return &homeask.CardFiler{Now: filingTestNow, StateDir: dir} }
	cmd := needsMkReleaseCmd()
	cmd.SetOut(&bytes.Buffer{})
	if e := cmd.Execute(); e == nil {
		t.Fatal("batch state failure was suppressed")
	}
}
