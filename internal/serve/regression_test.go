package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/mistakeknot/autarch/pkg/callerlog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestThreadHeaderInvalidOrEmpty(t *testing.T) {
	for _, id := range []string{"", "bad\r\nheader", "bad\tthread", "bad thread"} {
		t.Run(id, func(t *testing.T) {
			t.Setenv("BB_THREAD_ID", id)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, ok := r.Header["X-Autarch-Thread"]; ok {
					t.Error("sent invalid or empty thread")
				}
				w.Write([]byte(`[]`))
			}))
			defer srv.Close()
			if _, err := FetchProjects(srv.URL, tokenFile(t, testTok), time.Second); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRejectedScopesAndThreadCap(t *testing.T) {
	l := callerlog.New(t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects", func(http.ResponseWriter, *http.Request) {})
	r := httptest.NewRequest("GET", "http://localhost/api/projects", nil)
	r.Header.Set("X-Autarch-Thread", strings.Repeat("x", 1024))
	callerLogging(l, mux, guard("secret", nil, "127.0.0.1:0", mux)).ServeHTTP(httptest.NewRecorder(), r)
	l.Close()
	data, err := os.ReadFile(filepath.Join(l.Dir(), "caller-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"would_deny_cards:file", "would_deny_cards:read", "would_deny_admin"} {
		if row[k] != true {
			t.Errorf("%s = %v", k, row[k])
		}
	}
	if len(row["claimed_thread"].(string)) > 128 {
		t.Error("uncapped thread")
	}
}
func TestServeShutdownDrainsAudit(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{Addr: "127.0.0.1:0", TokenPath: filepath.Join(dir, "token"), CallerLogDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	closer, ok := any(s).(interface{ Close() })
	if !ok {
		t.Fatal("server has no audit lifecycle close")
	}
	defer closer.Close()
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "caller-log.jsonl"))
	if err != nil || !strings.Contains(string(data), "GET /health") {
		t.Fatalf("not drained: %s %v", data, err)
	}
}
func TestHijackedUpgradeStatus(t *testing.T) {
	l := callerlog.New(t.TempDir())
	mux := http.NewServeMux()
	finished := make(chan struct{})
	mux.HandleFunc("/signals/", func(w http.ResponseWriter, r *http.Request) {
		c, b, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer c.Close()
		b.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		b.Flush()
	})
	logged := callerLogging(l, mux, mux)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(finished); logged.ServeHTTP(w, r) }))
	defer srv.Close()
	c, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	c.Write([]byte("GET /signals/ HTTP/1.1\r\nHost: localhost\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"))
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	<-finished
	l.Close()
	data, _ := os.ReadFile(filepath.Join(l.Dir(), "caller-log.jsonl"))
	var row map[string]any
	json.Unmarshal(data, &row)
	if row["status"] != float64(101) {
		t.Fatal(row)
	}
}
