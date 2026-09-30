package serve

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func evalDir(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestResolveAmbiguousBaseName(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	mkdirs(t, filepath.Join(a, "x"), filepath.Join(b, "x"), filepath.Join(a, "y"))
	r := NewResolver([]string{a, b})
	_, err := r.Resolve("x")
	if err == nil || !strings.Contains(err.Error(), `ambiguous project "x"`) {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	if _, err := r.Resolve("y"); err != nil {
		t.Fatalf("y: %v", err)
	}
	if _, err := r.Resolve("missing"); err == nil {
		t.Fatal("expected unknown-project error")
	}
}

func TestResolveRefusesSymlinkOutsideDirs(t *testing.T) {
	scan, outside := t.TempDir(), t.TempDir()
	mkdirs(t, filepath.Join(outside, "secret"), filepath.Join(scan, "ok"))
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(scan, "link")); err != nil {
		t.Fatal(err)
	}
	r := NewResolver([]string{scan})
	if _, err := r.Resolve("link"); err == nil {
		t.Fatal("symlink escaping ProjectDirs must not resolve")
	}
	if _, err := r.Resolve("ok"); err != nil {
		t.Fatalf("ok: %v", err)
	}
}

func TestResolveFollowsRetargetAfterReread(t *testing.T) {
	scan := t.TempDir()
	mkdirs(t, filepath.Join(scan, "one"), filepath.Join(scan, "two"))
	link := filepath.Join(scan, "proj")
	if err := os.Symlink(filepath.Join(scan, "one"), link); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	r := NewResolver([]string{scan})
	r.now = func() time.Time { return now }

	got, err := r.Resolve("proj")
	if err != nil || got != evalDir(t, filepath.Join(scan, "one")) {
		t.Fatalf("first: %q %v", got, err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(scan, "two"), link); err != nil {
		t.Fatal(err)
	}
	// Within the TTL the cached root is reused.
	if got, _ := r.Resolve("proj"); got != evalDir(t, filepath.Join(scan, "one")) {
		t.Fatalf("cached: %q", got)
	}
	now = now.Add(projectTTL + time.Second)
	if got, _ := r.Resolve("proj"); got != evalDir(t, filepath.Join(scan, "two")) {
		t.Fatalf("after re-read: %q", got)
	}
}

func TestProjectsEndpointReportsDevIno(t *testing.T) {
	_, ts, scan := newTestServer(t)
	get := func() map[string]ProjectInfo {
		resp := do(t, "GET", ts.URL+"/api/projects", bearer())
		if resp.StatusCode != 200 {
			t.Fatalf("got %d", resp.StatusCode)
		}
		var list []ProjectInfo
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatal(err)
		}
		m := map[string]ProjectInfo{}
		for _, p := range list {
			m[p.Name] = p
		}
		return m
	}
	before := get()["demo"]
	if before.Root != evalDir(t, filepath.Join(scan, "demo")) || before.Ino == 0 {
		t.Fatalf("unexpected: %+v", before)
	}
	fi, _ := os.Stat(before.Root)
	if fi == nil {
		t.Fatal("stat")
	}
	// Replace the root with a new directory at the same path (old one kept
	// aside so the inode cannot be reused).
	old := filepath.Join(t.TempDir(), "demo-old")
	if err := os.Rename(filepath.Join(scan, "demo"), old); err != nil {
		t.Fatal(err)
	}
	mkdirs(t, filepath.Join(scan, "demo", ".gurgeh", "specs"))
	after := get()["demo"]
	if after.Ino == before.Ino {
		t.Fatalf("ino did not change: %d", after.Ino)
	}
}

func TestBigendMountDoesNotListEscapingSymlink(t *testing.T) {
	_, ts, scan := newTestServer(t)
	outside := t.TempDir()
	mkdirs(t, filepath.Join(outside, "secret", ".gurgeh", "specs"))
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(scan, "escape")); err != nil {
		t.Fatal(err)
	}
	// The Bigend manager discovers at construction, so build a fresh server over the same scan root.
	tokPath := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(tokPath, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s2, err := New(Config{Addr: "127.0.0.1:0", ProjectDirs: []string{scan}, TokenPath: tokPath})
	if err != nil {
		t.Fatal(err)
	}
	ts2 := httptest.NewServer(s2.Handler())
	defer ts2.Close()
	_ = ts
	for _, path := range []string{"/bigend/api/projects", "/api/projects"} {
		resp := do(t, "GET", ts2.URL+path, bearer())
		b, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(b), "escape") || strings.Contains(string(b), "secret") {
			t.Fatalf("%s lists the escaping symlink: %s", path, b)
		}
	}
	if c := do(t, "GET", ts2.URL+"/gurgeh/escape/api/specs", bearer()).StatusCode; c != 404 {
		t.Fatalf("gurgeh mount for escaping symlink: got %d", c)
	}
}

// A project directory replaced by a symlink to outside the scan dir after
// discovery must stop resolving even within the cache TTL.
func TestResolveRefusesRetargetOutsideWithinTTL(t *testing.T) {
	scan, outside := t.TempDir(), t.TempDir()
	mkdirs(t, filepath.Join(scan, "demo"), filepath.Join(outside, "secret"))
	r := NewResolver([]string{scan})
	if _, err := r.Resolve("demo"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(scan, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(scan, "demo")); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Resolve("demo"); err == nil {
		t.Fatalf("retargeted-outside project resolved to %q", got)
	}
}

func TestProjectsOmitsRetargetedOutsideWithinTTL(t *testing.T) {
	scan, outside := t.TempDir(), t.TempDir()
	mkdirs(t, filepath.Join(scan, "demo"), filepath.Join(outside, "secret"))
	r := NewResolver([]string{scan})
	if len(r.Projects()) != 1 {
		t.Fatal("expected one project")
	}
	os.Remove(filepath.Join(scan, "demo"))
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(scan, "demo")); err != nil {
		t.Fatal(err)
	}
	if ps := r.Projects(); len(ps) != 0 {
		t.Fatalf("retargeted project still listed: %+v", ps)
	}
}
