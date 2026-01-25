package intermute

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestStartRegistersAgentWhenConfigured(t *testing.T) {
	registered := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			registered <- struct{}{}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"agent_id": "agent-1",
				"session_id": "session-1",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/agents/agent-1/heartbeat":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("INTERMUTE_URL", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stop, err := Start(ctx)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if stop == nil {
		t.Fatalf("expected stop function")
	}
	stop()

	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatalf("expected registration call")
	}

	_ = os.Unsetenv("INTERMUTE_URL")
}
