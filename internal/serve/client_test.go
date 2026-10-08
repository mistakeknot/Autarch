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

func TestFetchProjectsMissingTokenIsNotCreated(t *testing.T) {
	p := filepath.Join(t.TempDir(), "serve.token")
	if _, err := FetchProjects("http://127.0.0.1:1", p, time.Second); err == nil {
		t.Fatal("want an error")
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("token was created")
	}
}

// the filer put /tmp/wimby, a root that is not WIMBY's, in the ask and the card was hidden.
func TestCheckAskRejectsARootThatIsNotTheNamedProjectsAndNamesTheExpectedOne(t *testing.T) {
	ps := []ProjectInfo{{Name: "WIMBY", Root: "/srv/projects/WIMBY"}, {Name: "wimby-tmp", Root: "/tmp/wimby"}}
	err := CheckAsk(ps, "WIMBY", "/tmp/wimby")
	if err == nil || !strings.Contains(err.Error(), `expected project_root "/srv/projects/WIMBY"`) {
		t.Fatalf("err = %v", err)
	}
	if err := CheckAsk(ps, "wimby", "/tmp/wimby"); err == nil || !strings.Contains(err.Error(), "/srv/projects/WIMBY") {
		t.Fatalf("case-insensitive project name: err = %v", err)
	}
}

func TestCheckAskAcceptsTheNamedProjectsRootAndNamesTheOwnerOfAForeignOne(t *testing.T) {
	ps := []ProjectInfo{{Name: "WIMBY", Root: "/srv/projects/WIMBY"}, {Name: "autarch", Root: "/srv/projects/autarch"}}
	if err := CheckAsk(ps, "WIMBY", "/srv/projects/WIMBY"); err != nil {
		t.Fatal(err)
	}
	err := CheckAsk(ps, "nosuch", "/srv/projects/autarch")
	if err == nil || !strings.Contains(err.Error(), `set project to "autarch"`) {
		t.Fatalf("err = %v", err)
	}
	if err := CheckAsk(ps, "WIMBY", "/elsewhere"); err == nil {
		t.Fatal("an unlisted root must still be refused")
	}
}
