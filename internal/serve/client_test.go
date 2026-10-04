package serve

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testTok = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func tokenFile(t *testing.T, tok string) string {
	p := filepath.Join(t.TempDir(), "serve.token")
	if err := os.WriteFile(p, []byte(tok+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFetchProjectsReadsTheListWithTheToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testTok {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"auraken","root":"/srv/projects/auraken"}]`))
	}))
	defer srv.Close()
	got, err := FetchProjects(srv.URL, tokenFile(t, testTok), time.Second)
	if err != nil || len(got) != 1 || got[0].Root != "/srv/projects/auraken" {
		t.Fatalf("got %v, %v", got, err)
	}
}

// A dead serve is reported at once, by name and address, and is told apart from other failures.
func TestFetchProjectsServeDownIsNamedAndFast(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens here now
	start := time.Now()
	_, err = FetchProjects("http://"+addr, tokenFile(t, testTok), 3*time.Second)
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
	if !errors.Is(err, ErrNotRunning) || !strings.Contains(err.Error(), "autarch serve is not running ("+addr+")") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckRootAcceptsAResolvedRoot(t *testing.T) {
	ps := []ProjectInfo{{Name: "auraken", Root: "/srv/projects/auraken"}}
	if err := CheckRoot(ps, "/srv/projects/auraken"); err != nil {
		t.Fatal(err)
	}
}

// The case seen on 2026-10-02: /srv/projects/Auraken was accepted at file time and failed later.
func TestCheckRootRejectsWrongCaseNamingTheNearestMatch(t *testing.T) {
	ps := []ProjectInfo{{Name: "auraken", Root: "/srv/projects/auraken"}, {Name: "autarch", Root: "/srv/projects/autarch"}}
	err := CheckRoot(ps, "/srv/projects/Auraken")
	if err == nil || !strings.Contains(err.Error(), "/srv/projects/auraken") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckRootRejectsAnUnknownRootAndListsNone(t *testing.T) {
	err := CheckRoot([]ProjectInfo{{Name: "a", Root: "/x/a"}}, "/y/zzz")
	if err == nil || !strings.Contains(err.Error(), "not a project root serve resolves") {
		t.Fatalf("err = %v", err)
	}
}
