package serve

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const testToken = "test-token"

// newTestServer builds a Server over a temp scan root holding an initialized
// Gurgeh project "demo" and an uninitialized project "plain".
func newTestServer(t *testing.T, allow ...string) (*Server, *httptest.Server, string) {
	t.Helper()
	scan := t.TempDir()
	if err := os.MkdirAll(filepath.Join(scan, "demo", ".gurgeh", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(scan, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(t.TempDir(), "serve.token")
	if err := os.WriteFile(tokPath, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{Addr: "127.0.0.1:0", ProjectDirs: []string{scan}, TokenPath: tokPath, AllowOrigins: allow})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts, scan
}

func writeSpecFile(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "id: \"" + id + "\"\ntitle: \"Title\"\nsummary: \"Summary\"\n"
	if err := os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
