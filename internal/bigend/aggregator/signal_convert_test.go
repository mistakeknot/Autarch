package aggregator

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestEventToSignal_TaskBlocked(t *testing.T) {
	evt := Event{
		Type:      "task.blocked",
		Project:   "/proj",
		EntityID:  "TASK-001",
		Timestamp: time.Now(),
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed for task.blocked")
	}
	if sig.Type != signals.SignalTaskBlocked {
		t.Fatalf("expected type %q, got %q", signals.SignalTaskBlocked, sig.Type)
	}
	if sig.Source != "intermute" {
		t.Fatalf("expected source 'intermute', got %q", sig.Source)
	}
	if sig.Severity != signals.SeverityWarning {
		t.Fatalf("expected severity Warning, got %q", sig.Severity)
	}
}

func TestEventToSignal_RunFailed(t *testing.T) {
	evt := Event{
		Type:      "run.failed",
		EntityID:  "RUN-001",
		Timestamp: time.Now(),
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed for run.failed")
	}
	if sig.Type != signals.SignalExecutionDrift {
		t.Fatalf("expected type %q, got %q", signals.SignalExecutionDrift, sig.Type)
	}
	if sig.Severity != signals.SeverityWarning {
		t.Fatalf("expected severity Warning, got %q", sig.Severity)
	}
}

func TestEventToSignal_RunWaiting(t *testing.T) {
	evt := Event{
		Type:      "run.waiting",
		EntityID:  "RUN-002",
		Timestamp: time.Now(),
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed for run.waiting")
	}
	if sig.Type != signals.SignalExecutionDrift {
		t.Fatalf("expected type %q, got %q", signals.SignalExecutionDrift, sig.Type)
	}
	if sig.Severity != signals.SeverityInfo {
		t.Fatalf("expected severity Info, got %q", sig.Severity)
	}
}

func TestEventToSignal_SpecRevised(t *testing.T) {
	evt := Event{
		Type:      "spec.revised",
		EntityID:  "SPEC-001",
		Timestamp: time.Now(),
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed for spec.revised")
	}
	if sig.Type != signals.SignalSpecHealthLow {
		t.Fatalf("expected type %q, got %q", signals.SignalSpecHealthLow, sig.Type)
	}
}

func TestEventToSignal_Unmapped(t *testing.T) {
	evt := Event{
		Type:      "message.sent",
		EntityID:  "MSG-001",
		Timestamp: time.Now(),
	}
	_, ok := eventToSignal(evt)
	if ok {
		t.Fatal("expected conversion to fail for unmapped event type")
	}
}

func TestEventToSignal_ZeroTimestampDefaultsToNow(t *testing.T) {
	evt := Event{
		Type:     "task.blocked",
		EntityID: "TASK-002",
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	if sig.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt when input timestamp is zero")
	}
}
