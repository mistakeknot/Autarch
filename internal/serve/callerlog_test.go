package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/callerlog"
)

func TestCallerLog(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	log := callerlog.New(dir)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gurgeh/{project}/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "response")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("same response"))
	})
	plain := guard("secret", nil, "127.0.0.1:0", mux)
	logged := callerLogging(log, mux, plain)
	for _, outcome := range []string{"ok", "origin", "host", "token"} {
		req := httptest.NewRequest("GET", "http://localhost/gurgeh/private-project/?q=query-probe", strings.NewReader("body-probe"))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("X-Probe", "header-probe")
		req.Header.Set("X-Autarch-Thread", "thr-test")
		switch outcome {
		case "origin":
			req.Header.Set("Origin", "origin-probe")
		case "host":
			req.Host = "host-probe"
		case "token":
			req.Header.Set("Authorization", "Bearer token-probe")
		}
		a, b := httptest.NewRecorder(), httptest.NewRecorder()
		plain.ServeHTTP(a, req.Clone(req.Context()))
		logged.ServeHTTP(b, req)
		if a.Code != b.Code || a.Body.String() != b.Body.String() || !reflect.DeepEqual(a.Header(), b.Header()) {
			t.Fatal("response changed")
		}
	}
	log.Close()
	data, err := os.ReadFile(filepath.Join(dir, "caller-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines %d", len(lines))
	}
	for i, line := range lines {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		keys := []string{"ts", "method", "route", "status", "guard", "claimed_thread", "peer", "would_deny_cards:file", "would_deny_cards:read", "would_deny_admin"}
		if len(row) != len(keys) {
			t.Fatalf("fields %v", row)
		}
		for _, k := range keys {
			if _, ok := row[k]; !ok {
				t.Fatal(k)
			}
		}
		if row["route"] != "GET /gurgeh/{project}/" || row["guard"] != []string{"ok", "origin", "host", "token"}[i] || row["claimed_thread"] != "thr-test" {
			t.Fatalf("row %v", row)
		}
		if _, err := time.Parse(time.RFC3339Nano, row["ts"].(string)); err != nil {
			t.Fatal(err)
		}
	}
	for _, probe := range []string{"query-probe", "body-probe", "header-probe", "origin-probe", "host-probe", "token-probe", "private-project", "secret"} {
		if strings.Contains(string(data), probe) {
			t.Fatalf("leaked %s", probe)
		}
	}
	fi, _ := os.Stat(filepath.Join(dir, "caller-log.jsonl"))
	if fi.Mode().Perm() != 0600 {
		t.Fatal(fi.Mode())
	}
}

func TestCallerLogUnwritable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(dir, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	log := callerlog.New(dir)
	defer log.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	w := httptest.NewRecorder()
	callerLogging(log, mux, guard("", nil, "127.0.0.1:0", mux)).ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/health", nil))
	if w.Code != 200 || w.Body.String() != "ok" {
		t.Fatal(w)
	}
}

func TestCallerLogRedirectDoesNotLeakPath(t *testing.T) {
	l := callerlog.New(t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/gurgeh/{project}/", func(http.ResponseWriter, *http.Request) {})
	w := httptest.NewRecorder()
	callerLogging(l, mux, guard("x", nil, "127.0.0.1:0", mux)).ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/gurgeh/redirect-private", nil))
	l.Close()
	data, _ := os.ReadFile(filepath.Join(l.Dir(), "caller-log.jsonl"))
	if strings.Contains(string(data), "redirect-private") {
		t.Fatalf("redirect leaked: %s", data)
	}
}

func TestCallerLogScopeTable(t *testing.T) {
	for _, tc := range []struct {
		path       string
		file, read bool
	}{{"/api/projects", true, true}, {"/signals/", false, true}, {"/gurgeh/p/", false, true}, {"/bigend/", false, false}, {"/health", false, false}} {
		t.Run(tc.path, func(t *testing.T) {
			l := callerlog.New(t.TempDir())
			mux := http.NewServeMux()
			for _, pattern := range []string{"GET /api/projects", "/signals/", "/gurgeh/{project}/", "/bigend/", "GET /health"} {
				mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
			}
			req := httptest.NewRequest("GET", "http://localhost"+tc.path, nil)
			req.Header.Set("Authorization", "Bearer x")
			callerLogging(l, mux, guard("x", nil, "127.0.0.1:0", mux)).ServeHTTP(httptest.NewRecorder(), req)
			l.Close()
			data, _ := os.ReadFile(filepath.Join(l.Dir(), "caller-log.jsonl"))
			var row map[string]any
			json.Unmarshal(data, &row)
			if row["would_deny_cards:file"] != !tc.file || row["would_deny_cards:read"] != !tc.read || row["would_deny_admin"] != false {
				t.Fatal(row)
			}
		})
	}
}

func TestCallerLogPreservesWriterCapabilities(t *testing.T) {
	l := callerlog.New(t.TempDir())
	defer l.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Hijacker); ok {
			t.Error("introduced Hijacker on recorder")
		}
		if _, ok := w.(http.Flusher); !ok {
			t.Error("lost Flusher")
		}
	})
	callerLogging(l, mux, mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://localhost/health", nil))
}
