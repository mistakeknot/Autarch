package serve

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	psignals "github.com/mistakeknot/autarch/pkg/signals"
)

func TestGurgehSpecUpdateReachesSignalsSubscriber(t *testing.T) {
	// The default signals URL points at a closed port: delivery must go through the shared broker.
	t.Setenv("AUTARCH_SIGNALS_URL", "http://127.0.0.1:1")
	_, ts, scan := newTestServer(t)
	specDir := filepath.Join(scan, "demo", ".gurgeh", "specs")
	writeSpecFile(t, specDir, "PRD-001")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hdr := http.Header{"Authorization": {"Bearer " + testToken}}
	c, _, err := websocket.Dial(ctx, ts.URL+"/signals/ws", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Give the server-side subscription a moment to register, then touch the spec.
	time.Sleep(200 * time.Millisecond)
	if code := do(t, "GET", ts.URL+"/gurgeh/demo/api/specs/PRD-001", bearer()).StatusCode; code != 200 {
		t.Fatalf("spec get: %d", code)
	}

	rctx, rcancel := context.WithTimeout(ctx, 2*time.Second)
	defer rcancel()
	var sig psignals.Signal
	if err := wsjson.Read(rctx, c, &sig); err != nil {
		t.Fatalf("no signal within 2s: %v", err)
	}
	if sig.Source != "gurgeh" || sig.SpecID != "PRD-001" {
		t.Fatalf("unexpected signal: %+v", sig)
	}
}
