package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/pkg/httpapi"
)

type listResponse struct {
	OK    bool           `json:"ok"`
	Data  []specSummary  `json:"data"`
	Meta  *httpapi.Meta  `json:"meta"`
	Error *httpapi.Error `json:"error,omitempty"`
}

type specSummary struct {
	ID string `json:"id"`
}

type errorResponse struct {
	OK    bool           `json:"ok"`
	Error *httpapi.Error `json:"error"`
}

func TestSpecsPaginationOffsetLimit(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, filepath.Join(root, ".gurgeh", "specs"), "PRD-001")
	writeSpec(t, filepath.Join(root, ".gurgeh", "specs"), "PRD-002")
	writeSpec(t, filepath.Join(root, ".gurgeh", "specs"), "PRD-003")

	s := New(root)
	s.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/specs?offset=1&limit=1", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != "PRD-002" {
		t.Fatalf("expected PRD-002, got %+v", resp.Data)
	}
	if resp.Meta == nil || resp.Meta.Cursor != "2" || resp.Meta.Limit != 1 {
		t.Fatalf("unexpected meta: %+v", resp.Meta)
	}
}

func TestSpecsOffsetBeatsCursor(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, filepath.Join(root, ".gurgeh", "specs"), "PRD-001")
	writeSpec(t, filepath.Join(root, ".gurgeh", "specs"), "PRD-002")
	writeSpec(t, filepath.Join(root, ".gurgeh", "specs"), "PRD-003")

	s := New(root)
	s.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/specs?cursor=1&offset=2&limit=1", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != "PRD-003" {
		t.Fatalf("expected PRD-003, got %+v", resp.Data)
	}
}

func TestSpecsIncludeArchived(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, filepath.Join(root, ".gurgeh", "specs"), "PRD-001")
	writeSpec(t, filepath.Join(root, ".gurgeh", "archived", "specs"), "PRD-002")

	s := New(root)
	s.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/specs", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected only active specs, got %d", len(resp.Data))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/specs?include_archived=true", nil)
	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	resp = listResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected archived specs included, got %d", len(resp.Data))
	}
}

func TestSpecsLegacyPraudeRoot(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, filepath.Join(root, ".praude", "specs"), "PRD-001")

	s := New(root)
	s.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/specs", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 spec from legacy root, got %d", len(resp.Data))
	}
}

func TestSpecsNotFound(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	s.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/specs/PRD-404", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != httpapi.ErrNotFound {
		t.Fatalf("expected not_found error, got %+v", resp.Error)
	}
}

func TestSpecsMethodNotAllowed(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	s.routes()

	req := httptest.NewRequest(http.MethodPost, "/api/specs", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != httpapi.ErrInvalidRequest {
		t.Fatalf("expected invalid_request error, got %+v", resp.Error)
	}
}

func writeSpec(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, id+".yaml")
	content := "id: \"" + id + "\"\ntitle: \"Title\"\nsummary: \"Summary\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}
