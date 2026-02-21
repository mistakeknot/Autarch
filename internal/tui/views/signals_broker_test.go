package views

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestSignalsView_BrokerPush(t *testing.T) {
	broker := signals.NewBroker()
	v := NewSignalsView(nil)
	v.SetBroker(broker)

	cmd := v.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a command")
	}

	want := signals.Signal{
		ID:        "TEST-001",
		Type:      signals.SignalTaskBlocked,
		Source:    "test",
		Severity:  signals.SeverityWarning,
		Title:     "test signal",
		CreatedAt: time.Now(),
	}

	v2, _ := v.Update(brokerSignalMsg{signal: want})
	sv := v2.(*SignalsView)
	if len(sv.signals) != 1 {
		t.Fatalf("expected 1 signal after broker push, got %d", len(sv.signals))
	}
	if sv.signals[0].ID != "TEST-001" {
		t.Fatalf("expected signal ID 'TEST-001', got %q", sv.signals[0].ID)
	}
}

func TestSignalsView_WaitBrokerSignal_Delivers(t *testing.T) {
	broker := signals.NewBroker()
	v := NewSignalsView(nil)
	v.SetBroker(broker)

	v.brokerDone = make(chan struct{})
	v.brokerSub = broker.Subscribe(nil)

	cmd := v.waitBrokerSignal()

	msgCh := make(chan tea.Msg, 1)
	go func() { msgCh <- cmd() }()

	want := signals.Signal{ID: "DELIVER-001", Type: signals.SignalTaskBlocked}
	broker.Publish(want)

	select {
	case msg := <-msgCh:
		got, ok := msg.(brokerSignalMsg)
		if !ok {
			t.Fatalf("expected brokerSignalMsg, got %T", msg)
		}
		if got.signal.ID != "DELIVER-001" {
			t.Fatalf("unexpected signal ID: %q", got.signal.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: signal not delivered")
	}
}

func TestSignalsView_BlurClosesSubscription(t *testing.T) {
	broker := signals.NewBroker()
	v := NewSignalsView(nil)
	v.SetBroker(broker)

	v.Init()
	if v.brokerSub == nil {
		t.Fatal("expected brokerSub to be set after Init()")
	}

	v.Blur()
	if v.brokerSub != nil {
		t.Fatal("expected brokerSub to be nil after Blur()")
	}
}

func TestSignalsView_DoubleInitNoLeak(t *testing.T) {
	broker := signals.NewBroker()
	v := NewSignalsView(nil)
	v.SetBroker(broker)

	v.Init()
	sub1 := v.brokerSub

	v.Blur()

	v.Init()
	sub2 := v.brokerSub

	if sub1 == sub2 {
		t.Fatal("expected different subscription after Blur+Init cycle")
	}
}

func TestSignalsView_NilBrokerFallback(t *testing.T) {
	v := NewSignalsView(nil)
	cmd := v.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a command even without broker")
	}
}

var _ tea.Msg = brokerSignalMsg{}
