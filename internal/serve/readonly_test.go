package serve

import "testing"

func TestReadOnly(t *testing.T) {
	_, ts, _ := newTestServer(t)
	methods := []string{"POST", "PUT", "PATCH", "DELETE"}
	for _, path := range []string{"/api/decisions", "/api/projects", "/api/decisions/1", "/health", "/", "/api/other"} {
		for _, m := range methods {
			c := do(t, m, ts.URL+path, bearer()).StatusCode
			if c != 404 && c != 405 {
				t.Errorf("%s %s: got %d, want 404 or 405", m, path, c)
			}
		}
	}
	if c := do(t, "GET", ts.URL+"/api/decisions", bearer()).StatusCode; c != 404 {
		t.Errorf("GET /api/decisions: got %d, want 404", c)
	}
}
