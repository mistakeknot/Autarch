package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestNeedsMkBudgetNamesPacificPeriod(t *testing.T) {
	old := newCardFiler
	t.Cleanup(func() { newCardFiler = old })
	f := &homeask.CardFiler{StateDir: t.TempDir(), Now: func() time.Time {
		return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	}}
	newCardFiler = func() *homeask.CardFiler { return f }
	cmd := needsMkBudgetCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--project", "autarch"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var status map[string]any
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["week_start"] != "2026-10-29T12:00:00-07:00" || status["week_end"] != "2026-11-05T12:00:00-08:00" || status["time_zone"] != "America/Los_Angeles" {
		t.Fatal(out.String())
	}
}
