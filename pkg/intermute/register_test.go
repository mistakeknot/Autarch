package intermute

import (
	"context"
	"os"
	"testing"
	"time"

	ic "github.com/mistakeknot/intermute/client"
)

func TestRegisterNoURLNoop(t *testing.T) {
	t.Setenv("INTERMUTE_URL", "")

	stop, err := Register(context.Background(), Options{Name: "test"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if stop == nil {
		t.Fatalf("expected non-nil stop")
	}
}

func TestRegisterNoURLSkipsClient(t *testing.T) {
	t.Setenv("INTERMUTE_URL", "")
	called := false
	orig := newClient
	newClient = func(string, ...ic.Option) *ic.Client {
		called = true
		return orig("http://example.com")
	}
	t.Cleanup(func() { newClient = orig })

	_, err := Register(context.Background(), Options{Name: "test"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if called {
		t.Fatalf("expected newClient not to be called when URL missing")
	}
}

func TestRegisterWarnsOnPartialConfig(t *testing.T) {
	t.Setenv("INTERMUTE_URL", "")
	t.Setenv("INTERMUTE_PROJECT", "proj")
	// Ensure this does not error; warning behavior is verified by integration tests/logs.
	_, err := Register(context.Background(), Options{Name: "test"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	os.Unsetenv("INTERMUTE_PROJECT")
}

func TestRegisterHeartbeatHasDeadline(t *testing.T) {
	t.Setenv("INTERMUTE_URL", "http://example.com")
	t.Setenv("INTERMUTE_HEARTBEAT_INTERVAL", "10ms")

	origClient := newClient
	origRegister := registerAgent
	origHeartbeat := heartbeat
	t.Cleanup(func() {
		newClient = origClient
		registerAgent = origRegister
		heartbeat = origHeartbeat
	})

	newClient = func(string, ...ic.Option) *ic.Client {
		return &ic.Client{}
	}
	registerAgent = func(context.Context, *ic.Client, ic.Agent) (ic.Agent, error) {
		return ic.Agent{ID: "agent-1"}, nil
	}

	hbCh := make(chan bool, 1)
	heartbeat = func(ctx context.Context, _ *ic.Client, _ string) error {
		_, ok := ctx.Deadline()
		select {
		case hbCh <- ok:
		default:
		}
		return nil
	}

	stop, err := Register(context.Background(), Options{Name: "test"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer stop()

	select {
	case ok := <-hbCh:
		if !ok {
			t.Fatalf("expected heartbeat context to have deadline")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected heartbeat to be called")
	}
}
