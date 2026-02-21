package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

func TestSignalsOverlay_BrokerPush(t *testing.T) {
	broker := signals.NewBroker()
	o := NewSignalsOverlay()
	o.SetBroker(broker)

	// Simulate toggle (opens overlay)
	o.Toggle()

	want := signals.Signal{
		ID:        "OV-001",
		Type:      signals.SignalTaskBlocked,
		Source:    "test",
		Title:     "overlay test",
		CreatedAt: time.Now(),
	}

	consumed, _ := o.Update(brokerOverlaySignalMsg{signal: want})
	if !consumed {
		t.Fatal("expected overlay to consume brokerOverlaySignalMsg")
	}
	if len(o.signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(o.signals))
	}
}

func TestSignalsOverlay_WaitBrokerSignal_Delivers(t *testing.T) {
	broker := signals.NewBroker()
	o := NewSignalsOverlay()
	o.SetBroker(broker)
	o.brokerDone = make(chan struct{})
	o.brokerSub = broker.Subscribe(nil)

	cmd := o.waitBrokerOverlaySignal()
	msgCh := make(chan tea.Msg, 1)
	go func() { msgCh <- cmd() }()

	want := signals.Signal{ID: "OV-DELIVER-001", Type: signals.SignalTaskBlocked}
	broker.Publish(want)

	select {
	case msg := <-msgCh:
		got, ok := msg.(brokerOverlaySignalMsg)
		if !ok {
			t.Fatalf("expected brokerOverlaySignalMsg, got %T", msg)
		}
		if got.signal.ID != "OV-DELIVER-001" {
			t.Fatalf("unexpected signal ID: %q", got.signal.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: signal not delivered")
	}
}

func TestSignalsOverlay_ToggleCloseCleanup(t *testing.T) {
	broker := signals.NewBroker()
	o := NewSignalsOverlay()
	o.SetBroker(broker)

	o.Toggle() // open
	if o.brokerSub == nil {
		t.Fatal("expected subscription after opening overlay")
	}

	o.Toggle() // close
	if o.brokerSub != nil {
		t.Fatal("expected subscription to be nil after closing overlay")
	}
}

func TestSignalsOverlay_NilBrokerWorks(t *testing.T) {
	o := NewSignalsOverlay()
	cmd := o.Toggle()
	_ = cmd // should not panic
}

var _ tea.Msg = brokerOverlaySignalMsg{}
