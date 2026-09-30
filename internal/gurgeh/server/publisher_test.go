package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	psignals "github.com/mistakeknot/autarch/pkg/signals"
)

func TestPublisherReceivesSignalsAndDialsNothing(t *testing.T) {
	var dialed atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dialed.Add(1)
	}))
	defer other.Close()
	t.Setenv("AUTARCH_SIGNALS_URL", other.URL)

	root := t.TempDir()
	writeSpec(t, filepath.Join(root, ".gurgeh", "specs"), "PRD-001")

	var mu sync.Mutex
	var got []psignals.Signal
	s := New(root, WithPublisher(func(sig psignals.Signal) {
		mu.Lock()
		got = append(got, sig)
		mu.Unlock()
	}))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/specs/PRD-001")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d", resp.StatusCode)
	}
	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n == 0 {
		t.Fatal("publisher was not called")
	}
	if d := dialed.Load(); d != 0 {
		t.Fatalf("default signals URL was dialed %d times", d)
	}
}
