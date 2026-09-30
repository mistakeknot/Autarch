package signals

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerServesHealthAndIsIdempotent(t *testing.T) {
	s := NewServer(nil)
	h1 := s.Handler()
	h2 := s.Handler() // must not panic on duplicate route registration
	if h1 == nil || h2 == nil {
		t.Fatal("nil handler")
	}
	ts := httptest.NewServer(h1)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
