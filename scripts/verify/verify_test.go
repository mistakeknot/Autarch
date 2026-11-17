//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReportSchema(t *testing.T) {
	dir := t.TempDir()
	r := Report{Schema: "autarch-verify/v1", Verdict: "INCOMPLETE", Base: strings64(), Head: strings64(), MergeBase: strings64(), Manifest: map[string]string{"driver": strings64()}, ManifestHash: strings64(), Journeys: []Result{{Flow: "home-pick", Verdict: "INCOMPLETE", Cause: "offline cache miss"}}}
	if err := writeReport(dir, filepath.Join(dir, "ledger.jsonl"), r); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "report.md", "ledger.jsonl"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || len(b) == 0 {
			t.Fatal(name, err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "report.json"))
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "base", "head", "merge_base", "head_tree", "e2e_tree", "verifier_commit", "verifier_manifest", "verifier_manifest_sha256", "changed_paths", "journeys", "artifacts", "bb_app_source", "bb_app_sha256", "cache", "verdict", "wall_seconds", "same_head_suites"} {
		if _, ok := data[key]; !ok {
			t.Fatal("missing", key)
		}
	}
}

func TestVerifierChangeSurvivesIncompletePrecedence(t *testing.T) {
	dir := t.TempDir()
	r := Report{VerifierChanged: true, Verdict: verdict([]string{"VERIFIER-CHANGED", "INCOMPLETE"})}
	if err := writeReport(dir, filepath.Join(dir, "ledger.jsonl"), r); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "report.md"))
	if !contains(string(b), "VERIFIER-CHANGED") || !contains(string(b), "INCOMPLETE") {
		t.Fatal(string(b))
	}
}

func TestMarkdownIncludesSameHeadAcceptanceGaps(t *testing.T) {
	dir := t.TempDir()
	cause := "INCOMPLETE: owned-server fixture configuration not supplied; Chromium revision 1208 unavailable"
	r := Report{Verdict: "INCOMPLETE", Suites: map[string]string{"owned-real acceptance": cause}}
	if err := writeReport(dir, filepath.Join(dir, "ledger.jsonl"), r); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil || !strings.Contains(string(b), cause) {
		t.Fatalf("same-head acceptance gap missing: %s (%v)", b, err)
	}
}

func TestNamespaceSetupFailureReportsIncomplete(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if err := setupFailureReport(fmt.Errorf("namespace unavailable"), out, filepath.Join(dir, "state"), 0.1); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r Report
	if err = json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Verdict != "INCOMPLETE" || r.Head != "" || r.Cause != "namespace unavailable" {
		t.Fatal(r)
	}
}

func TestEvidenceSymlinksCannotWriteLiveData(t *testing.T) {
	for _, kind := range []string{"report.md", "ledger.jsonl"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			victim := filepath.Join(t.TempDir(), "live.db")
			if err := os.WriteFile(victim, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(victim, filepath.Join(dir, kind)); err != nil {
				t.Fatal(err)
			}
			if err := writeReport(dir, filepath.Join(dir, "ledger.jsonl"), Report{Verdict: "INCOMPLETE"}); err == nil {
				t.Fatal("symlink accepted")
			}
			b, _ := os.ReadFile(victim)
			if string(b) != "unchanged" {
				t.Fatal("live data changed")
			}
		})
	}
}

func TestLogSymlinkCannotWriteLiveData(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "live.db")
	log := filepath.Join(dir, "log")
	if err := os.WriteFile(victim, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, log); err != nil {
		t.Fatal(err)
	}
	result := runGroup([]string{"/usr/bin/true"}, dir, childEnv(dir, "/usr/bin:/bin"), time.Second, log)
	b, _ := os.ReadFile(victim)
	if result.Verdict != "INCOMPLETE" || string(b) != "unchanged" {
		t.Fatal(result, string(b))
	}
}

func TestOwnedReadRefusesEscapes(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(t.TempDir(), "live.db")
	if err := os.WriteFile(victim, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "result.json")
	if err := os.Symlink(victim, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := readOwnedFile(root, alias); err == nil {
		t.Fatal("owned read followed an escape")
	}
}

func TestNamespaceCannotReadUnboundHostData(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("INCOMPLETE: bwrap missing")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("INCOMPLETE: node missing")
	}
	outside := t.TempDir()
	victim := filepath.Join(outside, "live.db")
	if err = os.WriteFile(victim, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	work := ownedTestDir(t)
	product := filepath.Join(work, "product")
	if err = os.Mkdir(product, 0700); err != nil {
		t.Fatal(err)
	}
	argv := namespace(work, product, []string{node}, []string{node, "-e", "try { require('node:fs').readFileSync(process.argv[1]); process.exit(0); } catch (e) { console.error(e.code); process.exit(37); }", victim})
	log := filepath.Join(work, "read.log")
	result := runGroup(argv, product, childEnv(work, "/usr/bin:/bin"), 5*time.Second, log)
	b, _ := os.ReadFile(log)
	if result.Verdict != "INCOMPLETE" || !contains(string(b), "ENOENT") {
		t.Fatal("unbound host data reached child", result, string(b))
	}
}

func TestAppManifestFramesFileContents(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for name, content := range map[string]string{filepath.Join(first, "a"): "x", filepath.Join(first, "b"): "y", filepath.Join(second, "a"): "xb\x00y"} {
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := treeHash(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := treeHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("different app trees share an ambiguously framed manifest")
	}
}

func TestManifestTracksHeadBuildHelpers(t *testing.T) {
	root := ownedTestDir(t)
	dir := filepath.Join(root, plugin, "scripts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "build-identity.mjs")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		if _, err := git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	r := Report{Manifest: map[string]string{"driver": "fixed"}}
	if err := productVerifierManifest(root, &r); err != nil {
		t.Fatal(err)
	}
	first := r.ManifestHash
	if err := os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := productVerifierManifest(root, &r); err != nil {
		t.Fatal(err)
	}
	if r.ManifestHash == first {
		t.Fatal("head helper changed without changing verifier identity")
	}
}
func strings64() string { return "0000000000000000000000000000000000000000000000000000000000000000" }

func ownedTestDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func TestMapRegistersDeferredJourneys(t *testing.T) {
	rows, err := loadMap("map.tsv")
	if err != nil {
		t.Fatal(err)
	}
	_, flows := selectFlows([]string{"go.mod"}, rows, nil)
	if len(flows) != 13 {
		t.Fatalf("shared dependency selected %d journeys", len(flows))
	}
	for _, row := range rows {
		if row.Flow != "home-pick" && row.Implemented {
			t.Fatal("deferred journey marked implemented", row)
		}
	}
	paths, selected := selectFlows([]string{plugin + "/app.tsx"}, rows, nil)
	if paths[0].Coverage != "NO-FLOW" || len(selected) < 2 {
		t.Fatal(paths, selected)
	}
}

func TestMapCoversActualSharedImplementations(t *testing.T) {
	rows, err := loadMap("map.tsv")
	if err != nil {
		t.Fatal(err)
	}
	for name, flow := range map[string]string{"internal/homefooter/footer.go": "cli-needsmk-footer", "internal/homeask/filing_budget.go": "cli-needsmk-budget", "internal/homeask/filer.go": "cli-needsmk-file", plugin + "/store.ts": "home-pick", "internal/gurgeh/tui/screen.go": "tui-navigation"} {
		_, selected := selectFlows([]string{name}, rows, nil)
		found := false
		for _, v := range selected {
			if v == flow {
				found = true
			}
		}
		if !found {
			t.Errorf("%s omitted %s", name, flow)
		}
	}
}

func TestOfflineNamespaceOutbound(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("INCOMPLETE: bwrap missing")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("INCOMPLETE: node missing")
	}
	// Hosted toolchains need not live below the namespace's system mounts.
	// Put Node outside the workspace to require an explicit toolchain bind.
	toolDir := t.TempDir()
	nodeBytes, err := os.ReadFile(node)
	if err != nil {
		t.Fatal(err)
	}
	node = filepath.Join(toolDir, "node")
	if err = os.WriteFile(node, nodeBytes, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	node, npmCLI, tools, err := nodeToolchain()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	product := filepath.Join(work, "product")
	if err = os.Mkdir(product, 0700); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"install-hook", "fake-journey"} {
		t.Run(kind, func(t *testing.T) {
			script := filepath.Join(product, "outbound.cjs")
			// A numeric external address avoids relying on DNS failure as proof of network isolation.
			if err = os.WriteFile(script, []byte("if(process.execPath!=="+strconv.Quote(node)+"){console.error('unexpected Node executable');process.exit(39)};const s=require('node:net').connect(443,'192.0.2.1');s.setTimeout(500);s.on('connect',()=>process.exit(0));s.on('error',e=>{console.error(e.code);process.exit(37)});s.on('timeout',()=>process.exit(38));"), 0600); err != nil {
				t.Fatal(err)
			}
			argv := []string{node, script}
			if kind == "install-hook" {
				if err = os.WriteFile(filepath.Join(product, "package.json"), []byte(`{"name":"outbound-probe","version":"1.0.0","scripts":{"install":"node outbound.cjs"}}`), 0600); err != nil {
					t.Fatal(err)
				}
				argv = []string{node, npmCLI, "--prefix", product, "install", "--offline", "--no-audit", "--no-fund", "--cache", filepath.Join(work, "npm-cache")}
			}
			r := runGroup(namespace(work, product, tools, argv), product, childEnv(work, toolDir+":/usr/bin:/bin"), 10*time.Second, filepath.Join(work, kind+".log"))
			if r.Verdict != "INCOMPLETE" || r.TimedOut {
				t.Fatal("outbound connection was not refused inside namespace", r)
			}
			log, err := os.ReadFile(filepath.Join(work, kind+".log"))
			if err != nil || !contains(string(log), "ENETUNREACH") {
				t.Fatalf("probe failed without proof of refused outbound access: %s %v", log, err)
			}
		})
	}
}

func TestMissingFakeNeverFallsBack(t *testing.T) {
	work := t.TempDir()
	product := filepath.Join(work, "product")
	app := filepath.Join(work, "app")
	cache := filepath.Join(work, "cache")
	seed := filepath.Join(work, "seed")
	for _, dir := range []string{filepath.Join(product, plugin, "e2e"), filepath.Join(app, "dist"), cache, seed} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(product, plugin, "e2e/real.ts"), []byte(`"pick-click":`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "dist/bb-app.js"), []byte("unused"), 0600); err != nil {
		t.Fatal(err)
	}
	report := Report{Artifacts: map[string]string{}, Cache: map[string]any{}}
	got := runHomePick(work, product, work, cache, app, seed, Row{Flow: "home-pick", Seconds: 1}, &report)
	if got.Verdict != "INCOMPLETE" || !contains(got.Cause, "verified fake bb missing") {
		t.Fatal(got)
	}
}

func TestOfflineCacheMissIsIncomplete(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("INCOMPLETE: bwrap missing")
	}
	work := ownedTestDir(t)
	product := filepath.Join(work, "product")
	app := filepath.Join(work, "app")
	cache := filepath.Join(work, "cache")
	seed := filepath.Join(work, "seed")
	for _, dir := range []string{filepath.Join(product, plugin, "e2e"), filepath.Join(app, "dist"), cache, seed} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{filepath.Join(product, plugin, "e2e/real.ts"): `"pick-click":`, filepath.Join(product, plugin, "e2e/fake-bb.mjs"): "#!/usr/bin/env node\n", filepath.Join(app, "dist/bb-app.js"): "unused", filepath.Join(product, plugin, "package-lock.json"): `{"packages":{"node_modules/playwright-core":{"version":"1.58.2"}}}`}
	for name, content := range files {
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture"}} {
		if _, err := git(product, args...); err != nil {
			t.Fatal(err)
		}
	}
	report := Report{Artifacts: map[string]string{}, Cache: map[string]any{}}
	got := runHomePick(work, product, work, cache, app, seed, Row{Flow: "home-pick", Seconds: 5}, &report)
	if got.Verdict != "INCOMPLETE" || !contains(got.Cause, "offline cache miss") {
		t.Fatal(got)
	}
}

func TestAmbientLiveDestinationRefused(t *testing.T) {
	for _, key := range []string{"AUTARCH_HOME_DB", "HOME_BB_DATA", "BB_DATA_DIR"} {
		t.Run(key, func(t *testing.T) {
			live := t.TempDir()
			value := live
			if key == "AUTARCH_HOME_DB" {
				value = filepath.Join(live, "data.db")
			}
			t.Setenv(key, value)
			if _, err := safePath(filepath.Join(live, "out"), "/unrelated"); err == nil {
				t.Fatal("live destination accepted")
			}
		})
	}
}

func TestCacheParentCannotExposeLiveHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := safePath(home, "/unrelated"); err == nil {
		t.Fatal("mounting the caller's whole HOME was accepted")
	}
}

func TestRepoLockSerializesRuns(t *testing.T) {
	name := filepath.Join(t.TempDir(), "repo.lock")
	first, err := acquireRepoLock(name)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := acquireRepoLock(name); err == nil {
		second.Close()
		t.Fatal("concurrent run acquired repository lock")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := acquireRepoLock(name)
	if err != nil {
		t.Fatal(err)
	}
	third.Close()
}

func TestRepoLockDescriptorIsNotInherited(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("INCOMPLETE: node missing")
	}
	dir := t.TempDir()
	name := filepath.Join(dir, "repo.lock")
	lock, err := acquireRepoLock(name)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	script := `const fs=require('node:fs');for(const fd of fs.readdirSync('/proc/self/fd')){try{if(fs.readlinkSync('/proc/self/fd/'+fd)===process.argv[1])process.exit(37)}catch{}}`
	result := runGroup([]string{node, "-e", script, name}, dir, childEnv(dir, "/usr/bin:/bin"), 3*time.Second, filepath.Join(dir, "fd.log"))
	if result.Verdict != "PASS" {
		t.Fatal("child inherited repository lock", result)
	}
}

func TestFakeListenerRefusesWildcard(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", "localhost", "example.invalid"} {
		listener, err := startFakeListener(host)
		if err == nil {
			listener.Close()
			t.Fatal("unsafe bind accepted", host)
		}
	}
}

func TestFakeListenerProof(t *testing.T) {
	if os.Getenv("AUTARCH_VERIFY_LISTENER_HELPER") == "1" {
		listener, err := startFakeListener("127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		return
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("INCOMPLETE: bwrap missing")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	product := filepath.Join(work, "product")
	if err = os.Mkdir(product, 0700); err != nil {
		t.Fatal(err)
	}
	env := append(childEnv(work, "/usr/bin:/bin"), "AUTARCH_VERIFY_LISTENER_HELPER=1")
	result := runGroup(namespace(work, product, []string{binary}, []string{binary, "-test.run=^TestFakeListenerProof$"}), product, env, 5*time.Second, filepath.Join(work, "listener.log"))
	if result.Verdict != "PASS" {
		log, _ := os.ReadFile(filepath.Join(work, "listener.log"))
		t.Fatal(result, string(log))
	}
}

func TestPrecedence(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{nil, "NO-FLOW"}, {[]string{"PASS"}, "PASS"}, {[]string{"NO-FLOW"}, "NO-FLOW"},
		{[]string{"NO-FLOW", "VERIFIER-CHANGED"}, "VERIFIER-CHANGED"},
		{[]string{"VERIFIER-CHANGED", "INCOMPLETE"}, "INCOMPLETE"},
		{[]string{"FAIL", "INCOMPLETE"}, "FAIL"},
	} {
		if got := verdict(tc.in); got != tc.want {
			t.Fatalf("%v: %s", tc.in, got)
		}
	}
}

func TestPartialAndUnknownCoverage(t *testing.T) {
	rows := []Row{{Glob: "app.tsx", Flow: "home-pick", Coverage: "partial", Implemented: true}, {Glob: "app.tsx", Flow: "real-bb-regression", Coverage: "partial"}}
	paths, flows := selectFlows([]string{"app.tsx", "new.ts"}, rows, []string{"extra"})
	if paths[0].Coverage != "NO-FLOW" || paths[1].Coverage != "NO-FLOW" {
		t.Fatal(paths)
	}
	if len(flows) != 3 {
		t.Fatal(flows)
	}
	if verdict([]string{paths[0].Coverage, "INCOMPLETE"}) == "PASS" {
		t.Fatal("deferred wiring passed")
	}
}

func TestOutputRefusals(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{filepath.Join(root, ".bb-machines", "out"), filepath.Join(root, "checkout", "out")} {
		if _, err := safePath(name, filepath.Join(root, "checkout")); err == nil {
			t.Fatal(name)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := safePath(filepath.Join(root, "link", "out"), "/unrelated"); err == nil {
		t.Fatal("symlink escape accepted")
	}
}

func TestMachineDirectoryComponents(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name string
		path string
		deny bool
	}{
		{"middle", filepath.Join(root, ".bb-machines", "out"), true},
		{"trailing", filepath.Join(root, ".bb-machines"), true},
		{"lookalike", filepath.Join(root, "x.bb-machines-old", "out"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := safePath(tc.path, filepath.Join(root, "checkout"))
			if (err != nil) != tc.deny {
				t.Fatalf("safePath(%q): %v; want refusal %v", tc.path, err, tc.deny)
			}
		})
	}
}

func TestChildEnvironment(t *testing.T) {
	for _, k := range []string{"AUTARCH_HOME_DB", "HOME_BB_DATA", "BB_SERVER_URL", "BB_DATA_DIR", "BB_THREAD_ID", "XDG_STATE_HOME"} {
		t.Setenv(k, "planted-live")
	}
	env := childEnv("/owned", "/owned/bin")
	for _, v := range env {
		if contains(v, "planted-live") {
			t.Fatal(v)
		}
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "env.log")
	result := runGroup([]string{"/usr/bin/env"}, dir, env, time.Second, log)
	b, err := os.ReadFile(log)
	if err != nil || result.Verdict != "PASS" || contains(string(b), "planted-live") {
		t.Fatal(result, string(b), err)
	}
}

func TestTimeoutKillsGroup(t *testing.T) {
	dir := ownedTestDir(t)
	work := filepath.Join(dir, "work")
	source := filepath.Join(work, "source")
	mirror := filepath.Join(work, "objects.git")
	product := filepath.Join(work, "product")
	log := filepath.Join(dir, "timeout.log")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-qm", "fixture"}, {"clone", "--bare", "--no-local", source, mirror}} {
		if _, err := git(source, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := git(mirror, "worktree", "add", "--detach", product, "HEAD"); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(product, "child.pid")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("INCOMPLETE: node missing")
	}
	script := `const child=require('node:child_process').spawn(process.execPath,['-e','setTimeout(()=>{},30000)'],{stdio:'ignore'});require('node:fs').writeFileSync(process.argv[1],String(child.pid));setTimeout(()=>{},30000);`
	result := runGroup([]string{node, "-e", script, pidFile}, product, childEnv(dir, "/usr/bin:/bin"), 300*time.Millisecond, log)
	if result.Verdict != "INCOMPLETE" || !result.TimedOut {
		t.Fatal(result)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	if err = cleanupWork(work, mirror, product, true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(work); !os.IsNotExist(err) {
		t.Fatal("temporary worktree survived timeout", err)
	}
	if _, err = os.Stat(log); err != nil {
		t.Fatal("evidence was removed", err)
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil {
		fields := strings.Fields(string(stat)[strings.LastIndex(string(stat), ")")+1:])
		if len(fields) == 0 || fields[0] != "Z" {
			t.Fatal("child still running after timeout", string(stat))
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestKnownFailureOutranksOuterDeadline(t *testing.T) {
	lines := []byte("malformed\n{\"scenario\":\"pick-click\",\"pass\":false}\n")
	if got := journeyVerdict(true, 1, lines); got != "FAIL" {
		t.Fatal(got)
	}
	if got := journeyVerdict(true, 1, nil); got != "INCOMPLETE" {
		t.Fatal(got)
	}
}

func TestBehaviorTimeoutIsFailure(t *testing.T) {
	if classifyHarness(1, []byte("{\"scenario\":\"pick-click\",\"pass\":false,\"error\":\"timed out waiting for pick\"}\n")) != "FAIL" {
		t.Fatal("product timeout must fail")
	}
	if classifyHarness(1, nil) != "INCOMPLETE" {
		t.Fatal("setup failure must be incomplete")
	}
}
