package agent

import "testing"

type fakeHealthRunner struct {
	alive      bool
	restarted  bool
	restartArg string
}

func (f *fakeHealthRunner) IsAlive(session string) bool {
	return f.alive
}

func (f *fakeHealthRunner) Restart(session string) error {
	f.restarted = true
	f.restartArg = session
	return nil
}

func TestHealthMonitorRestartsCrashedSession(t *testing.T) {
	runner := &fakeHealthRunner{alive: false}
	monitor := NewHealthMonitor(runner, true)
	monitor.Check("tand-123")
	if !runner.restarted {
		t.Fatal("expected restart call")
	}
	if runner.restartArg != "tand-123" {
		t.Fatalf("expected restart arg tand-123, got %s", runner.restartArg)
	}
}
