package signals

import (
	"strconv"
	"testing"
	"time"
)

func TestPublishDelivery(t *testing.T) {
	broker := NewBroker()
	sub := broker.Subscribe(nil)
	defer sub.Close()

	want := testSignal(1)
	broker.Publish(want)

	got := recvSignal(t, sub.Chan())
	if got.ID != want.ID {
		t.Fatalf("unexpected signal ID: got %q want %q", got.ID, want.ID)
	}
	if got.Type != want.Type {
		t.Fatalf("unexpected signal type: got %q want %q", got.Type, want.Type)
	}
}

func TestPublishOverflow(t *testing.T) {
	broker := NewBroker()
	sub := broker.Subscribe(nil)
	defer sub.Close()

	for i := 0; i < 64; i++ {
		broker.Publish(testSignal(i))
	}
	broker.Publish(testSignal(64))

	for i := 1; i <= 64; i++ {
		got := recvSignal(t, sub.Chan())
		wantID := strconv.Itoa(i)
		if got.ID != wantID {
			t.Fatalf("unexpected signal at position %d: got ID %q want %q", i, got.ID, wantID)
		}
	}
}

func TestPublishDropCounter(t *testing.T) {
	broker := NewBroker()
	sub := broker.Subscribe(nil)
	defer sub.Close()

	for i := 0; i < 64; i++ {
		broker.Publish(testSignal(i))
	}
	if got := broker.Dropped.Load(); got != 0 {
		t.Fatalf("unexpected dropped count before overflow: got %d want 0", got)
	}

	for i := 64; i < 67; i++ {
		broker.Publish(testSignal(i))
	}

	if got := broker.Dropped.Load(); got != 3 {
		t.Fatalf("unexpected dropped count after overflow: got %d want 3", got)
	}
}

func testSignal(id int) Signal {
	return Signal{
		ID:        strconv.Itoa(id),
		Type:      SignalTaskBlocked,
		Source:    "test",
		Title:     "test",
		CreatedAt: time.Now(),
	}
}

func recvSignal(t *testing.T, ch <-chan Signal) Signal {
	t.Helper()

	select {
	case sig := <-ch:
		return sig
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signal")
		return Signal{}
	}
}
