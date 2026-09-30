package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerServesHealth(t *testing.T) {
	srv := NewServer(Config{Addr: "127.0.0.1:0"})
	ts := httptest.NewServer(srv.Handler())
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
