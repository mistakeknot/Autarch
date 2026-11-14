package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mistakeknot/autarch/internal/homeask"
	"os"
	"path/filepath"
	"testing"
)

func TestReleasePrintsDeliveredGroupsAndKeyedFailures(t *testing.T) {
	s := &stubBB{}
	f := &homeask.CardFiler{Run: s.run, LockDir: t.TempDir()}
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
	if err := cmd.Execute(); err == nil {
		t.Fatal("partial failure must exit nonzero")
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
