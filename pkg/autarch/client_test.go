package autarch

import (
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
)

// mockDataSource implements DataSource for testing.
type mockDataSource struct {
	specs    []Spec
	epics    []Epic
	stories  []Story
	tasks    []Task
	insights []Insight
}

func (m *mockDataSource) ListSpecs(status string) ([]Spec, error)              { return m.specs, nil }
func (m *mockDataSource) ListEpics(specID string) ([]Epic, error)              { return m.epics, nil }
func (m *mockDataSource) ListStories(epicID string) ([]Story, error)           { return m.stories, nil }
func (m *mockDataSource) ListTasks(status, agent string) ([]Task, error)       { return m.tasks, nil }
func (m *mockDataSource) ListInsights(specID, category string) ([]Insight, error) {
	return m.insights, nil
}

func TestIsDialError_ConnectionRefused(t *testing.T) {
	// Simulate a net.OpError with dial + ECONNREFUSED
	err := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Addr: &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 7338,
		},
		Err: &os.SyscallError{
			Syscall: "connect",
			Err:     syscall.ECONNREFUSED,
		},
	}
	if !isDialError(err) {
		t.Error("expected isDialError to return true for ECONNREFUSED")
	}
}

func TestIsDialError_Timeout(t *testing.T) {
	// Timeouts should NOT trigger fallback
	err := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &os.SyscallError{
			Syscall: "connect",
			Err:     syscall.ETIMEDOUT,
		},
	}
	if isDialError(err) {
		t.Error("isDialError should return false for timeout errors")
	}
}

func TestIsDialError_NonNetError(t *testing.T) {
	if isDialError(errors.New("some other error")) {
		t.Error("isDialError should return false for non-net errors")
	}
}

func TestIsDialError_NonDialOp(t *testing.T) {
	err := &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: &os.SyscallError{
			Syscall: "read",
			Err:     syscall.ECONNREFUSED,
		},
	}
	if isDialError(err) {
		t.Error("isDialError should return false for non-dial operations")
	}
}

func TestClient_FallbackOnDialError(t *testing.T) {
	mock := &mockDataSource{
		specs: []Spec{{ID: "local-1", Title: "From Local"}},
	}

	// Client pointing to a port where nothing is listening
	client := NewClient("http://127.0.0.1:19999")
	client.WithFallback(mock)

	if client.InFallbackMode() {
		t.Error("should not start in fallback mode")
	}

	// This should fail to connect and trigger fallback
	specs, err := client.ListSpecs("")
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}

	if !client.InFallbackMode() {
		t.Error("should be in fallback mode after dial failure")
	}

	if len(specs) != 1 || specs[0].ID != "local-1" {
		t.Errorf("expected local fallback data, got %v", specs)
	}
}

func TestClient_SessionStickyFallback(t *testing.T) {
	callCount := 0
	mock := &countingDataSource{
		callCount: &callCount,
		specs:     []Spec{{ID: "local-1"}},
		epics:     []Epic{{ID: "local-epic"}},
	}

	client := NewClient("http://127.0.0.1:19999")
	client.WithFallback(mock)

	// First call triggers fallback
	_, err := client.ListSpecs("")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !client.InFallbackMode() {
		t.Fatal("should be in fallback after first call")
	}

	// Second call should go directly to fallback (not try HTTP again)
	epics, err := client.ListEpics("")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(epics) != 1 || epics[0].ID != "local-epic" {
		t.Errorf("expected fallback epics, got %v", epics)
	}

	// Both calls should have used the mock
	if *mock.callCount != 2 {
		t.Errorf("expected 2 fallback calls, got %d", *mock.callCount)
	}
}

func TestClient_NoFallbackWithoutConfig(t *testing.T) {
	// Client with no fallback configured
	client := NewClient("http://127.0.0.1:19999")

	_, err := client.ListSpecs("")
	if err == nil {
		t.Error("expected error when no fallback and server unreachable")
	}
	if client.InFallbackMode() {
		t.Error("should not be in fallback mode without fallback configured")
	}
}

func TestClient_InFallbackMode_Default(t *testing.T) {
	client := NewClient("http://127.0.0.1:7338")
	if client.InFallbackMode() {
		t.Error("new client should not be in fallback mode")
	}
}

// countingDataSource wraps a mockDataSource and counts calls.
type countingDataSource struct {
	callCount *int
	specs     []Spec
	epics     []Epic
	stories   []Story
	tasks     []Task
	insights  []Insight
}

func (c *countingDataSource) ListSpecs(status string) ([]Spec, error) {
	*c.callCount++
	return c.specs, nil
}

func (c *countingDataSource) ListEpics(specID string) ([]Epic, error) {
	*c.callCount++
	return c.epics, nil
}

func (c *countingDataSource) ListStories(epicID string) ([]Story, error) {
	*c.callCount++
	return c.stories, nil
}

func (c *countingDataSource) ListTasks(status, agent string) ([]Task, error) {
	*c.callCount++
	return c.tasks, nil
}

func (c *countingDataSource) ListInsights(specID, category string) ([]Insight, error) {
	*c.callCount++
	return c.insights, nil
}

func TestClient_WriteMethodsBlockedInFallback(t *testing.T) {
	mock := &mockDataSource{
		specs: []Spec{{ID: "local-1"}},
	}

	client := NewClient("http://127.0.0.1:19999")
	client.WithFallback(mock)

	// Trigger fallback via a read
	_, err := client.ListSpecs("")
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if !client.InFallbackMode() {
		t.Fatal("should be in fallback mode")
	}

	// All write methods should return ErrFallbackReadOnly
	if _, err := client.CreateSpec(Spec{}); !errors.Is(err, ErrFallbackReadOnly) {
		t.Errorf("CreateSpec: got %v, want ErrFallbackReadOnly", err)
	}
	if _, err := client.UpdateSpec(Spec{ID: "x"}); !errors.Is(err, ErrFallbackReadOnly) {
		t.Errorf("UpdateSpec: got %v, want ErrFallbackReadOnly", err)
	}
	if err := client.DeleteSpec("x"); !errors.Is(err, ErrFallbackReadOnly) {
		t.Errorf("DeleteSpec: got %v, want ErrFallbackReadOnly", err)
	}
	if _, err := client.CreateTask(Task{}); !errors.Is(err, ErrFallbackReadOnly) {
		t.Errorf("CreateTask: got %v, want ErrFallbackReadOnly", err)
	}
	if err := client.LinkInsight("x", "y"); !errors.Is(err, ErrFallbackReadOnly) {
		t.Errorf("LinkInsight: got %v, want ErrFallbackReadOnly", err)
	}
	if _, err := client.AssignTask("x", "a"); !errors.Is(err, ErrFallbackReadOnly) {
		t.Errorf("AssignTask: got %v, want ErrFallbackReadOnly", err)
	}
}
