package homeask

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func pilotListener(t *testing.T) net.Listener {
	t.Helper()
	addr := "127.0.0.1:0"
	if bin, e := exec.LookPath("free-port"); e == nil {
		b, e := exec.Command(bin).Output()
		if e != nil {
			t.Fatal(e)
		}
		addr = "127.0.0.1:" + strings.TrimSpace(string(b))
	}
	l, e := net.Listen("tcp", addr)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	return l
}
func TestPilotDeadPreviewsStayHeldWakeOnce(t *testing.T) {
	for _, kind := range []string{"closed", "502"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			l := pilotListener(t)
			url := "https://" + l.Addr().String() + "/preview"
			if kind == "closed" {
				l.Close()
			} else {
				s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
				s.Listener.Close()
				s.Listener = l
				s.StartTLS()
				defer s.Close()
				f.Links = ArtifactLinkChecker{Client: s.Client()}
			}
			r := testReq()
			r.NonBlocking = true
			r.Ask["question"] = "See " + url + " ?"
			if _, e := f.FileCard(ctx, r); e != nil {
				t.Fatal(e)
			}
			enablePilot(t, f, "linkcheck")
			var wakes []string
			f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
				if len(args) > 1 && args[0] == "thread" && args[1] == "tell" {
					wakes = append(wakes, strings.Join(args, " "))
					return BBResult{}
				}
				return bb.run(ctx, env, args...)
			}
			for i := 0; i < 2; i++ {
				if _, e := f.Release(ctx); e == nil {
					t.Fatal("dead preview released")
				}
			}
			s, _, done, e := f.openState()
			if e != nil {
				t.Fatal(e)
			}
			held := s[r.Key].Held
			done()
			if !held || len(wakes) != 1 || !strings.Contains(wakes[0], url) || !strings.Contains(wakes[0], r.Thread) {
				t.Fatal(held, wakes)
			}
		})
	}
}
func TestPilotLinkStatusAndRedirects(t *testing.T) {
	l := pilotListener(t)
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loop" {
			http.Redirect(w, r, "/loop", 302)
			return
		}
		var code int
		fmt.Sscanf(r.URL.Path, "/%d", &code)
		w.WriteHeader(code)
	}))
	s.Listener.Close()
	s.Listener = l
	s.StartTLS()
	defer s.Close()
	for _, code := range []int{200, 204, 301, 401, 403, 404, 410, 500, 502, 503} {
		r := testReq()
		r.Ask["question"] = fmt.Sprintf("%s/%d", s.URL, code)
		v := (ArtifactLinkChecker{Client: s.Client()}).Check(context.Background(), r)
		want := code == 404 || code == 410 || code >= 500
		if (len(v) > 0) != want {
			t.Fatal(code, v)
		}
	}
}
func TestPilotScriptAndPRChecks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "build.sh")
	os.WriteFile(p, []byte("echo ok"), 0600)
	h := sha256.Sum256([]byte("echo ok"))
	r := testReq()
	r.Ask["question"] = fmt.Sprintf("PR example/demo#12 head abcdef012345\nsha256: %x\nbash %s", h, p)
	c := ArtifactLinkChecker{PR: func(context.Context, string) (string, error) { return "abcdef0123456789", nil }}
	if v := c.Check(context.Background(), r); len(v) != 0 {
		t.Fatal(v)
	}
	os.WriteFile(p, []byte("changed"), 0600)
	c.PR = func(context.Context, string) (string, error) { return "fedcba0123456789", nil }
	v := c.Check(context.Background(), r)
	if len(v) != 2 || !strings.Contains(failureText(v), "changed") {
		t.Fatal(v)
	}
}
func TestPilotRecheckDryRunNoWrites(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	r := testReq()
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	enablePilot(t, f, "linkcheck")
	d, _ := f.stateDir()
	before, _ := os.ReadFile(filepath.Join(d, "filings.json"))
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) > 3 && strings.Join(args[:3], " ") == "home get --card" {
			return okRes(CardView{TaskID: args[3], State: "open", CardKey: "TEST-1"})
		}
		return bb.run(ctx, env, args...)
	}
	var out bytes.Buffer
	if e := f.RecheckLinks(context.Background(), &out, true); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(filepath.Join(d, "filings.json"))
	if !bytes.Equal(before, after) || bb.count("thread", "tell") != 0 {
		t.Fatal("dry-run mutated")
	}
}
