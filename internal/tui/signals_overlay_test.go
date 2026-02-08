package tui

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestSignalsOverlayToggle(t *testing.T) {
	o := NewSignalsOverlay()
	if o.Visible() {
		t.Fatalf("overlay should start hidden")
	}

	o.Toggle()
	if !o.Visible() {
		t.Fatalf("overlay should be visible after first toggle")
	}

	o.Toggle()
	if o.Visible() {
		t.Fatalf("overlay should be hidden after second toggle")
	}
}

func TestSignalsOverlayRenderEmpty(t *testing.T) {
	o := NewSignalsOverlay()
	o.SetSize(80, 24)
	o.Toggle()

	view := o.View()
	if view == "" {
		t.Fatalf("expected non-empty view")
	}
}

func TestSignalsOverlayRenderWithSignals(t *testing.T) {
	o := NewSignalsOverlay()
	o.SetSize(80, 24)
	o.Toggle()

	o.loaded = true
	o.signals = []signals.Signal{
		{
			ID:        "sig-1",
			Type:      signals.SignalExecutionDrift,
			Source:    "coldwine",
			Severity:  signals.SeverityWarning,
			Title:     "Task execution drifting",
			CreatedAt: time.Now(),
		},
	}

	view := o.View()
	if view == "" {
		t.Fatalf("expected non-empty view")
	}
}

func TestSignalsOverlayClose(t *testing.T) {
	o := NewSignalsOverlay()
	o.Toggle()
	o.Close()

	if o.Visible() {
		t.Fatalf("overlay should be hidden after close")
	}
}
