package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/icdata"
)

func TestHandleSessionsFiltersByQuery(t *testing.T) {
	agg := &fakeAgg{state: aggregator.State{
		Sessions: []aggregator.TmuxSession{
			{Name: "codex", UnifiedState: icdata.StatusWaiting},
			{Name: "claude", UnifiedState: icdata.StatusActive},
		},
	}}
	srv := NewServer(config.ServerConfig{}, agg)

	req := httptest.NewRequest(http.MethodGet, "/sessions?q=!waiting", nil)
	w := httptest.NewRecorder()
	srv.handleSessions(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "font-mono\">codex") || strings.Contains(body, "font-mono\">claude") {
		t.Fatalf("expected filtered sessions in response")
	}
}

func TestSessionsTemplateShowsQueryValue(t *testing.T) {
	agg := &fakeAgg{state: aggregator.State{Sessions: []aggregator.TmuxSession{{Name: "codex"}}}}
	srv := NewServer(config.ServerConfig{}, agg)
	req := httptest.NewRequest(http.MethodGet, "/sessions?q=codex", nil)
	w := httptest.NewRecorder()
	srv.handleSessions(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "id=\"sessions-search\"") {
		t.Fatalf("expected search input in response")
	}
	start := strings.Index(body, "id=\"sessions-search\"")
	end := strings.Index(body[start:], ">")
	if start == -1 || end == -1 {
		t.Fatalf("expected search input markup")
	}
	chunk := body[start : start+end]
	if !strings.Contains(chunk, "value=\"codex\"") {
		t.Fatalf("expected query value in input")
	}
}

func TestSessionsGroupedByProject(t *testing.T) {
	agg := &fakeAgg{state: aggregator.State{
		Sessions: []aggregator.TmuxSession{
			{Name: "a", ProjectPath: "/p/one"},
			{Name: "b", ProjectPath: "/p/two"},
		},
	}}
	srv := NewServer(config.ServerConfig{}, agg)
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	srv.handleSessions(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "Project: one") || !strings.Contains(body, "Project: two") {
		t.Fatalf("expected project group headers in response")
	}
}
