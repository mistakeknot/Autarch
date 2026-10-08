package main

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/mistakeknot/autarch/internal/mycroft/escalate"
	"github.com/mistakeknot/autarch/internal/serve"

	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/spawn"
)

func TestRunPathSetsDecisionQueue(t *testing.T) {
	db, err := mycroft.OpenDB(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	o := buildOrchestrator(db, spawn.NewClaudeCodeSpawner("", "Demarch"), mycroft.DefaultConfig())
	if !o.HasQueue() {
		t.Fatal("production orchestrator has no decision queue")
	}
}

type recFiler struct{ asks []homeask.Ask }

func (f *recFiler) FileForPull(_ context.Context, a homeask.Ask) (string, error) {
	f.asks = append(f.asks, a)
	return "d1", nil
}

// Home accepts an ask only when project is a serve project name and project_root is
// the root serve resolves for it (estate: the Uqbar directory). This mirrors that check.
func homeAccepts(t *testing.T, a homeask.Ask, dirs []string, uqbar string) bool {
	t.Helper()
	if a.Project == "estate" {
		return uqbar != "" && a.ProjectRoot == uqbar
	}
	root, err := serve.NewResolver(dirs).Resolve(a.Project)
	return err == nil && root == a.ProjectRoot
}

func TestDefaultHomeWiringFilesWhatHomeAccepts(t *testing.T) {
	scan, uqbar := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(scan, "Demarch"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &recFiler{}
	q := escalate.NewDecisionQueue()
	q.SetHomeRoots(f, nil, homeRoots(listFrom([]string{scan}), uqbar))
	// The bead label is lowercased; the project directory is not.
	if err := q.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "D-1", BeadTitle: "t", Labels: []string{"project:demarch"}}); err != nil {
		t.Fatal(err)
	}
	if err := q.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "E-1", BeadTitle: "t"}); err != nil {
		t.Fatal(err)
	}
	if len(f.asks) != 2 {
		t.Fatalf("filed %d, want 2", len(f.asks))
	}
	for _, a := range f.asks {
		if !homeAccepts(t, a, []string{scan}, uqbar) {
			t.Errorf("Home would reject %+v", a)
		}
	}
	// An unknown project, or estate without an Uqbar, files nothing.
	q2 := escalate.NewDecisionQueue()
	f2 := &recFiler{}
	q2.SetHomeRoots(f2, nil, homeRoots(listFrom([]string{scan}), ""))
	if q2.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "X-1", Labels: []string{"project:nope"}}) == nil ||
		q2.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "E-2"}) == nil || len(f2.asks) != 0 {
		t.Fatalf("expected refusals, filed %+v", f2.asks)
	}
}

func listFrom(dirs []string) func() ([]serve.ProjectInfo, error) {
	return func() ([]serve.ProjectInfo, error) { return serve.NewResolver(dirs).Projects(), nil }
}

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// startServe runs a real serve over explicit --project-dir roots, apart from the Bigend defaults.
func startServe(t *testing.T, dirs []string, token string) (url, tokPath string) {
	t.Helper()
	tokPath = filepath.Join(t.TempDir(), "serve.token")
	if err := os.WriteFile(tokPath, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := serve.New(serve.Config{Addr: "127.0.0.1:0", ProjectDirs: dirs, TokenPath: tokPath})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts.URL, tokPath
}

// Mycroft asks the running serve for roots, so an explicit --project-dir that differs from
// the Bigend defaults still files what Home accepts.
func TestRootsComeFromServeNotBigendDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // Bigend defaults resolve elsewhere
	explicit := t.TempDir()
	if err := os.MkdirAll(filepath.Join(explicit, "Elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	url, tok := startServe(t, []string{explicit}, testToken)
	f := &recFiler{}
	q := escalate.NewDecisionQueue()
	q.SetHomeRoots(f, nil, homeRoots(serveProjects(url, tok), ""))
	if err := q.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "D-1", Labels: []string{"project:elsewhere"}}); err != nil {
		t.Fatal(err)
	}
	if len(f.asks) != 1 || !homeAccepts(t, f.asks[0], []string{explicit}, "") {
		t.Fatalf("filed %+v", f.asks)
	}
	// the Bigend-default resolver alone cannot see that project
	if _, _, err := homeRoots(listFrom(nil), "")("Elsewhere"); err == nil {
		t.Fatal("defaults unexpectedly resolve the explicit project")
	}
}

// When serve cannot be asked (down, wrong token, non-loopback), nothing is filed.
func TestServeUnavailableFilesNothing(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "P"), 0o755); err != nil {
		t.Fatal(err)
	}
	url, tok := startServe(t, []string{dir}, testToken)
	badTok := filepath.Join(t.TempDir(), "bad.token")
	if err := os.WriteFile(badTok, []byte("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, list := range map[string]func() ([]serve.ProjectInfo, error){
		"wrong token":  serveProjects(url, badTok),
		"down":         serveProjects("http://127.0.0.1:1", tok),
		"no token":     serveProjects(url, filepath.Join(t.TempDir(), "missing")),
		"non-loopback": serveProjects("http://example.com", tok),
	} {
		f := &recFiler{}
		if _, err := list(); err == nil || !strings.Contains(err.Error(), "start `autarch serve` (see AGENTS.md)") {
			t.Errorf("%s: error should tell the operator to start serve, got %v", name, err)
		}
		q := escalate.NewDecisionQueue()
		q.SetHomeRoots(f, nil, homeRoots(list, ""))
		if q.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "D-1", Labels: []string{"project:p"}}) == nil || len(f.asks) != 0 {
			t.Errorf("%s: expected refusal, filed %+v", name, f.asks)
		}
	}
}

// The production queue files through the card filer, threadless, and reads Home through the
// same filer: no legacy filer, no private list.
func TestBuildOrchestratorQueueHoldsACardFiler(t *testing.T) {
	scan, uqbar := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(scan, "Demarch"), 0o755); err != nil {
		t.Fatal(err)
	}
	url, tok := startServe(t, []string{scan}, testToken)
	t.Setenv("AUTARCH_SERVE_URL", url)
	t.Setenv("AUTARCH_SERVE_TOKEN_FILE", tok)
	t.Setenv("AUTARCH_UQBAR_DIR", uqbar)
	t.Setenv(homeask.EnvTasksProjects, "demarch=TP1")
	t.Setenv("BB_THREAD_ID", "thr-leak")
	db, err := mycroft.OpenDB(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := buildOrchestrator(db, spawn.NewClaudeCodeSpawner("", "Demarch"), mycroft.DefaultConfig()).Queue()
	cf, ok := q.Filer().(*homeask.CardFiler)
	if !ok {
		t.Fatalf("queue filer is %T, want *homeask.CardFiler", q.Filer())
	}
	var calls []string
	var threadEnv string
	cf.LockDir = t.TempDir()
	cf.Run = func(_ context.Context, env []string, args ...string) homeask.BBResult {
		calls = append(calls, strings.Join(args, " "))
		for _, kv := range env {
			if strings.HasPrefix(kv, "BB_THREAD_ID=") {
				threadEnv = kv
			}
		}
		switch strings.Join(args[:min(3, len(args))], " ") {
		case "home get --request":
			return homeask.BBResult{Stdout: []byte(`{"status":"absent"}`)}
		case "tasks label list":
			return homeask.BBResult{Stdout: []byte(`{"labels":[{"name":"needs-mk"}]}`)}
		case "tasks project list":
			return homeask.BBResult{Stdout: []byte(`{"projects":[{"id":"TP1","name":"x"}]}`)}
		case "tasks list --status":
			return homeask.BBResult{Stdout: []byte(`{"tasks":[],"nextCursor":null}`)}
		case "tasks create --project":
			return homeask.BBResult{Stdout: []byte(`{"task":{"id":"T9","projectId":"TP1"}}`)}
		}
		return homeask.BBResult{Code: 1, Stderr: []byte("unexpected " + strings.Join(args, " "))}
	}
	err = q.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "D-1", BeadTitle: "t", Labels: []string{"project:demarch"}})
	if err != nil {
		t.Fatalf("%v; calls=%v", err, calls)
	}
	all := strings.Join(calls, "\n")
	if !strings.Contains(all, "tasks create --project TP1") || !strings.Contains(all, "--label needs-mk") {
		t.Errorf("no card created in the bound tasks project:\n%s", all)
	}
	if strings.Contains(all, "home ask") || strings.Contains(all, "tasks comment") {
		t.Errorf("a pull filing must not use home ask or post a routing comment:\n%s", all)
	}
	if threadEnv != "" {
		t.Errorf("the pull filing carried a thread: %s", threadEnv)
	}
}
