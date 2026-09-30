package serve

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func do(t *testing.T, method, url string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func bearer() map[string]string { return map[string]string{"Authorization": "Bearer " + testToken} }

func TestHealthNoToken(t *testing.T) {
	_, ts, _ := newTestServer(t)
	resp := do(t, "GET", ts.URL+"/health", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["projects"].(float64) != 2 {
		t.Fatalf("unexpected health: %v", body)
	}
	if _, ok := body["build"].(map[string]any)["exe_sha256"]; !ok {
		t.Fatalf("missing build info: %v", body)
	}
}

func TestBigendNeedsToken(t *testing.T) {
	_, ts, _ := newTestServer(t)
	if c := do(t, "GET", ts.URL+"/bigend/api/sessions", nil).StatusCode; c != 401 {
		t.Fatalf("no token: got %d", c)
	}
	if c := do(t, "GET", ts.URL+"/bigend/api/sessions", bearer()).StatusCode; c != 200 {
		t.Fatalf("token: got %d", c)
	}
}

func TestQueryTokenRejected(t *testing.T) { // [A-22]
	_, ts, _ := newTestServer(t)
	if c := do(t, "GET", ts.URL+"/bigend/api/sessions?token="+testToken, nil).StatusCode; c != 401 {
		t.Fatalf("got %d", c)
	}
}

func TestBadHostForbidden(t *testing.T) {
	_, ts, _ := newTestServer(t)
	h := bearer()
	h["Host"] = "evil.example:8110"
	if c := do(t, "GET", ts.URL+"/bigend/api/sessions", h).StatusCode; c != 403 {
		t.Fatalf("got %d", c)
	}
}

func TestBadOriginForbidden(t *testing.T) {
	_, ts, _ := newTestServer(t)
	h := bearer()
	h["Origin"] = "http://evil.example"
	if c := do(t, "GET", ts.URL+"/bigend/api/sessions", h).StatusCode; c != 403 {
		t.Fatalf("plain route: got %d", c)
	}
	// WebSocket upgrade with a bad Origin.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hdr := http.Header{"Authorization": {"Bearer " + testToken}, "Origin": {"http://evil.example"}}
	_, resp, err := websocket.Dial(ctx, ts.URL+"/signals/ws", &websocket.DialOptions{HTTPHeader: hdr})
	if err == nil {
		t.Fatal("expected websocket dial to fail")
	}
	if resp == nil || resp.StatusCode != 403 {
		t.Fatalf("ws bad origin: resp=%v err=%v", resp, err)
	}
}

func TestAllowedOrigin(t *testing.T) {
	_, ts, _ := newTestServer(t, "http://good.example")
	h := bearer()
	h["Origin"] = "http://good.example"
	if c := do(t, "GET", ts.URL+"/bigend/api/sessions", h).StatusCode; c != 200 {
		t.Fatalf("got %d", c)
	}
}

func TestSignalsWebSocketWithBearer(t *testing.T) {
	_, ts, _ := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hdr := http.Header{"Authorization": {"Bearer " + testToken}}
	c, _, err := websocket.Dial(ctx, ts.URL+"/signals/ws", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c.Close(websocket.StatusNormalClosure, "")
}

func TestGurgehMount(t *testing.T) {
	_, ts, _ := newTestServer(t)
	if c := do(t, "GET", ts.URL+"/gurgeh/demo/api/specs", bearer()).StatusCode; c != 200 {
		t.Fatalf("demo: got %d", c)
	}
	if c := do(t, "GET", ts.URL+"/gurgeh/nope/api/specs", bearer()).StatusCode; c != 404 {
		t.Fatalf("unknown: got %d", c)
	}
	if c := do(t, "GET", ts.URL+"/gurgeh/plain/api/specs", bearer()).StatusCode; c != 404 {
		t.Fatalf("no .gurgeh: got %d", c)
	}
}

func TestTokenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "serve.token")
	tok, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 64 {
		t.Fatalf("token len %d", len(tok))
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	again, err := LoadOrCreateToken(path)
	if err != nil || again != tok {
		t.Fatalf("reload: %v %q", err, again)
	}
	loose := filepath.Join(dir, "loose.token")
	if err := os.WriteFile(loose, []byte("abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateToken(loose); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestTokenFileRejectsWeakToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weak.token")
	if err := os.WriteFile(path, []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateToken(path); err == nil || !strings.Contains(err.Error(), "64 hex") {
		t.Fatalf("expected weak-token refusal, got %v", err)
	}
}

func TestServeReturnsListenerErrorWithoutLeakingWaiter(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // Serve on a closed listener fails at once
	s := &Server{handler: http.NewServeMux()}
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Serve(ctx, ln); err == nil {
		t.Fatal("expected listener error")
	}
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutine leaked: %d -> %d", before, after)
	}
}
