//go:build linux

// Autarch's advisory exact-head verifier. Source: bead mk-05q0j.3.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"time"
)

const plugin = "integrations/bb-plugin-autarch"

var verifierPaths = []string{"scripts/verify", ".claude/skills/autarch-verify"}
var headVerifierPaths = []string{plugin + "/e2e", plugin + "/scripts", plugin + "/__tests__/fake-bb-server.ts", plugin + "/__tests__/fixtures"}

func verifierInput(name string) bool {
	for _, group := range [][]string{verifierPaths, headVerifierPaths} {
		for _, prefix := range group {
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				return true
			}
		}
	}
	return false
}

func coverageVerdicts(paths []Mapping) ([]string, bool) {
	values := []string{}
	nothing := true
	for _, p := range paths {
		if p.Coverage != "ignored" {
			nothing = false
		}
		if p.Coverage == "NO-FLOW" {
			values = append(values, "NO-FLOW")
		}
		if verifierInput(p.Path) {
			values = append(values, "VERIFIER-CHANGED")
		}
	}
	if nothing {
		values = append(values, "NO-FLOW")
	}
	return values, nothing
}

func resolveRange(root, base, head string) (string, string, error) {
	b, err := git(root, "rev-parse", "--verify", base+"^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("resolve base: %w", err)
	}
	h, err := git(root, "rev-parse", "--verify", head+"^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("resolve head: %w", err)
	}
	if b == h {
		return "", "", fmt.Errorf("base and head resolve to the same commit; nothing can be verified")
	}
	return b, h, nil
}

func seedMirror(root, mirror, head string) error {
	if _, err := git(root, "clone", "--bare", "--no-local", root, mirror); err != nil {
		return err
	}
	// Bare clones copy branch/tag tips, not remote-tracking or dangling heads.
	// Fetch only the already resolved local object; network transport stays off.
	if _, err := git(mirror, "fetch", "--no-tags", root, head); err != nil {
		return fmt.Errorf("resolved head unavailable in owned mirror: %w", err)
	}
	return nil
}

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func treeHash(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			if target != root && !strings.HasPrefix(target, root+"/") {
				return fmt.Errorf("app copy has external symlink: %s", rel)
			}
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := json.NewEncoder(h).Encode([]string{rel, "link", link}); err != nil {
				return err
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("app copy contains non-regular file: %s", rel)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		fileHash := sha256.New()
		_, err = io.Copy(fileHash, f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		return json.NewEncoder(h).Encode([]string{rel, "file", hex.EncodeToString(fileHash.Sum(nil))})
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeFresh(name string, b []byte) error {
	fd, err := syscall.Open(name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	_, err = f.Write(b)
	return err
}
func git(dir string, args ...string) (string, error) {
	// Even metadata helpers get a private network namespace. Git hooks and file
	// monitors are disabled; only local object transport is permitted.
	gitArgs := append([]string{"/usr/bin/git", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "protocol.allow=never", "-c", "protocol.file.allow=always", "-C", dir}, args...)
	command := append([]string{"-n", "19", "/usr/bin/ionice", "-c3", "/usr/bin/bwrap", "--unshare-net", "--die-with-parent", "--ro-bind", "/", "/", "--dev", "/dev", "--bind", "/tmp", "/tmp", "--"}, gitArgs...)
	c := exec.Command("/usr/bin/nice", command...)
	c.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	var output bytes.Buffer
	c.Stdout = &output
	c.Stderr = &output
	e, _ := waitGroup(c, 60*time.Second)
	b := output.Bytes()
	if e != nil {
		return "", fmt.Errorf("git %v: %s: %w", args, b, e)
	}
	return strings.TrimSpace(string(b)), nil
}
func cleanupWork(work, mirror, product string, created bool) error {
	var gitErr error
	if created {
		_, gitErr = git(mirror, "worktree", "remove", "--force", product)
	}
	return errors.Join(gitErr, os.RemoveAll(work))
}

func repoLockName() string { return fmt.Sprintf("/tmp/autarch-verify-%d-1140086114.lock", os.Getuid()) }
func acquireRepoLock(name string) (*os.File, error) {
	fd, err := syscall.Open(name, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	var stat syscall.Stat_t
	if err = syscall.Fstat(fd, &stat); err != nil {
		file.Close()
		return nil, err
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 || stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		file.Close()
		return nil, fmt.Errorf("unsafe repository lock")
	}
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("another verifier is running: %w", err)
	}
	return file, nil
}

type Report struct {
	VerifierChanged bool              `json:"verifier_changed"`
	Schema          string            `json:"schema"`
	Base            string            `json:"base"`
	Head            string            `json:"head"`
	MergeBase       string            `json:"merge_base"`
	HeadTree        string            `json:"head_tree"`
	E2ETree         string            `json:"e2e_tree"`
	Verifier        string            `json:"verifier_commit"`
	Manifest        map[string]string `json:"verifier_manifest"`
	ManifestHash    string            `json:"verifier_manifest_sha256"`
	Paths           []Mapping         `json:"changed_paths"`
	Journeys        []Result          `json:"journeys"`
	Artifacts       map[string]string `json:"artifacts"`
	Cache           map[string]any    `json:"cache"`
	AppSource       string            `json:"bb_app_source"`
	AppHash         string            `json:"bb_app_sha256"`
	Verdict         string            `json:"verdict"`
	Wall            float64           `json:"wall_seconds"`
	State           string            `json:"cache_state"`
	PR              string            `json:"pr"`
	Suites          map[string]string `json:"same_head_suites"`
	SuiteSource     string            `json:"suite_results_source"`
	SuiteClaimHead  string            `json:"suite_results_claimed_head,omitempty"`
	Cause           string            `json:"cause,omitempty"`
}

func loadMap(name string) ([]Row, error) {
	b, e := readOwnedFile(filepath.Dir(name), name)
	if e != nil {
		return nil, e
	}
	out := []Row{}
	for n, line := range strings.Split(string(b), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 7 {
			return nil, fmt.Errorf("map row %d: need seven columns", n+1)
		}
		var sec int
		if _, e = fmt.Sscanf(f[5], "%d", &sec); e != nil || sec < 1 {
			return nil, fmt.Errorf("invalid deadline")
		}
		if (f[2] != "full" && f[2] != "partial") || (f[3] != "implemented" && f[3] != "deferred") || (f[4] != "new" && f[4] != "regression") || strings.ContainsAny(f[0], "[:\\") {
			return nil, fmt.Errorf("invalid map row %d", n+1)
		}
		out = append(out, Row{f[0], f[1], f[2], f[3] == "implemented", f[4], sec, f[6]})
	}
	return out, nil
}
func manifest(root string) (map[string]string, string, error) {
	names, e := git(root, append([]string{"ls-files", "-z", "--"}, verifierPaths...)...)
	if e != nil {
		return nil, "", e
	}
	out := map[string]string{}
	for _, n := range strings.Split(names, "\x00") {
		if n == "" {
			continue
		}
		b, e := readOwnedFile(root, filepath.Join(root, n))
		if e != nil {
			return nil, "", e
		}
		out[n] = hash(b)
	}
	b, _ := json.Marshal(out)
	return out, hash(b), nil
}

// Head-owned harness fixtures and build helpers are executed inputs too. Keep
// them in the content manifest as well as recording the separate e2e tree ID.
func productVerifierManifest(product string, r *Report) error {
	names, err := git(product, append([]string{"ls-files", "-z", "--"}, headVerifierPaths...)...)
	if err != nil {
		return err
	}
	if r.Manifest == nil {
		r.Manifest = map[string]string{}
	}
	for _, name := range strings.Split(names, "\x00") {
		if name == "" {
			continue
		}
		b, err := readOwnedFile(product, filepath.Join(product, name))
		if err != nil {
			return err
		}
		r.Manifest["head:"+name] = hash(b)
	}
	b, err := json.Marshal(r.Manifest)
	if err != nil {
		return err
	}
	r.ManifestHash = hash(b)
	return nil
}
func changed(root, base, head string) ([]string, error) {
	text, e := git(root, "diff", "--name-status", "-z", "-M", base, head)
	if e != nil {
		return nil, e
	}
	f := strings.Split(text, "\x00")
	names := map[string]bool{}
	for i := 0; i < len(f) && f[i] != ""; {
		status := f[i]
		i++
		count := 1
		if status[0] == 'R' || status[0] == 'C' {
			count = 2
		}
		for j := 0; j < count; j++ {
			if i >= len(f) {
				return nil, fmt.Errorf("truncated diff")
			}
			names[f[i]] = true
			i++
		}
	}
	out := []string{}
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// namespace gives children only system executables, owned working paths and explicit caches.
// The caller's home, checkout, /tmp and credentials are hidden, even from install hooks.
func namespace(root, product string, ro []string, argv []string) []string {
	a := []string{"/usr/bin/bwrap", "--die-with-parent", "--unshare-net", "--unshare-pid", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp"}
	// No host root bind: relocated Home data and sockets outside /home must be
	// absent too. Only system code/config and explicitly declared inputs enter.
	for _, p := range []string{"/usr/bin", "/usr/sbin", "/usr/lib", "/usr/lib64", "/usr/libexec", "/usr/include", "/usr/share", "/bin", "/sbin", "/lib", "/lib64", "/sys", "/etc/ld.so.cache", "/etc/hosts", "/etc/resolv.conf", "/etc/nsswitch.conf", "/etc/passwd", "/etc/group", "/etc/localtime", "/etc/os-release", "/etc/ssl/certs", "/etc/ssl/openssl.cnf", "/etc/fonts"} {
		if _, err := os.Stat(p); err == nil {
			a = append(a, "--ro-bind", p, p)
		}
	}
	a = append(a, "--bind", root, root, "--bind", product, product)
	for _, p := range ro {
		a = append(a, "--ro-bind", p, p)
	}
	a = append(a, "--chdir", product, "--")
	return append(a, argv...)
}

func nodeToolchain() (string, string, []string, error) {
	resolve := func(name string) (string, error) {
		path, err := exec.LookPath(name)
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(path)
	}
	node, err := resolve("node")
	if err != nil {
		return "", "", nil, err
	}
	npmCLI, err := resolve("npm")
	if err != nil {
		return "", "", nil, err
	}
	if filepath.Base(npmCLI) != "npm-cli.js" {
		return "", "", nil, fmt.Errorf("npm must resolve to its package's npm-cli.js")
	}
	// Only Node's executable and npm's package enter the namespace. A hosted
	// toolchain prefix can contain unrelated programs and must not be exposed.
	return node, npmCLI, []string{node, filepath.Dir(filepath.Dir(npmCLI))}, nil
}

func writeReport(out, ledger string, r Report) error {
	if r.SuiteSource == "" {
		if len(r.Suites) != 0 {
			r.SuiteSource = "operator-supplied, unverified"
		} else {
			r.SuiteSource = "not supplied"
		}
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	if e = writeFresh(filepath.Join(out, "report.json"), append(b, '\n')); e != nil {
		return e
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Autarch verify: %s\n\nHead: `%s`\n\nVerifier: `%s`\n\nWall: %.3f s (%s). Advisory pilot.\n", r.Verdict, r.Head, r.Verifier, r.Wall, r.State)
	if r.VerifierChanged {
		md.WriteString("\nVERIFIER-CHANGED: the diff edits the verifier or product harness; inspect journey evidence.\n")
	}
	if r.Cause != "" {
		fmt.Fprintf(&md, "\n%s\n", r.Cause)
	}
	for _, j := range r.Journeys {
		fmt.Fprintf(&md, "\n- %s: **%s** (%s, %.3f s) %s\n", j.Flow, j.Verdict, j.Label, j.Wall, j.Cause)
	}
	for _, p := range r.Paths {
		if p.Coverage == "NO-FLOW" {
			fmt.Fprintf(&md, "\n- Uncovered: `%s` (selected: %s)\n", p.Path, strings.Join(p.Flows, ", "))
		}
	}
	suites := make([]string, 0, len(r.Suites))
	for name := range r.Suites {
		suites = append(suites, name)
	}
	sort.Strings(suites)
	if len(suites) != 0 {
		fmt.Fprintf(&md, "\nSuite results: %s. Claimed head: `%s`. Claims alone do not establish an incremental catch.\n", r.SuiteSource, r.SuiteClaimHead)
	}
	for _, name := range suites {
		fmt.Fprintf(&md, "\n- %s: %s\n", name, r.Suites[name])
	}
	if e = writeFresh(filepath.Join(out, "report.md"), []byte(md.String())); e != nil {
		return e
	}
	fd, e := syscall.Open(ledger, syscall.O_CREAT|syscall.O_WRONLY|syscall.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), ledger)
	defer f.Close()
	var stat syscall.Stat_t
	if e = syscall.Fstat(fd, &stat); e != nil {
		return e
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 || stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		return fmt.Errorf("unsafe ledger file")
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	b, _ = json.Marshal(r)
	_, e = f.Write(append(b, '\n'))
	return e
}
func main() {
	started := time.Now()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() { <-signals; close(stopRequested) }()
	if e := execute(started); e != nil {
		fmt.Fprintln(os.Stderr, e)
		if os.Geteuid() != 0 {
			out := ""
			if flag.Lookup("out") != nil {
				out = flag.Lookup("out").Value.String()
			}
			state := os.Getenv("XDG_STATE_HOME")
			if state == "" {
				state = filepath.Join(os.Getenv("HOME"), ".local/state")
			}
			if err := setupFailureReport(e, out, state, time.Since(started).Seconds()); err != nil {
				fmt.Fprintln(os.Stderr, "setup evidence:", err)
			}
		}
		os.Exit(2)
	}
}

// When setup cannot even resolve provenance (for example bwrap is missing),
// unknown identities stay empty and acceptance remains INCOMPLETE.
func setupFailureReport(cause error, out, stateHome string, wall float64) error {
	state, err := safePath(filepath.Join(stateHome, "autarch-verify"), "")
	if err != nil {
		return err
	}
	if out == "" {
		out = filepath.Join(state, "runs", fmt.Sprintf("setup-%d", time.Now().UnixNano()))
	}
	out, err = safePath(out, "")
	if err != nil {
		return err
	}
	for _, dir := range []string{state, out} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return writeReport(out, filepath.Join(state, "ledger.jsonl"), Report{Schema: "autarch-verify/v1", Verdict: "INCOMPLETE", Cause: cause.Error(), Wall: wall, Paths: []Mapping{}, Journeys: []Result{}})
}
func execute(started time.Time) (resultErr error) {
	base := flag.String("base", "", "base commit")
	head := flag.String("head", "", "head commit")
	pin := flag.String("verifier-commit", "", "required verifier SHA")
	outArg := flag.String("out", "", "evidence directory outside all checkouts")
	cacheRoot := flag.String("cache-root", "", "preprovisioned cache directory")
	app := flag.String("app-copy", "", "owned, read-only bb application copy")
	seed := flag.String("toolchain-seed", "", "offline bb toolchain seed")
	flowsArg := flag.String("flows", "", "additional journeys")
	state := flag.String("cache-state", "cold", "cold or warm measurement")
	pr := flag.String("pr", "", "PR URL")
	suiteFile := flag.String("suite-results", "", "same-head suite results JSON with head and results")
	flag.Parse()
	if os.Geteuid() == 0 {
		return fmt.Errorf("verification requires a non-root user")
	}
	if *base == "" || *head == "" || *pin == "" || (*state != "cold" && *state != "warm") {
		return fmt.Errorf("base, head, pinned verifier and cold/warm state required")
	}
	cwd, e := os.Getwd()
	if e != nil {
		return e
	}
	root, e := git(cwd, "rev-parse", "--show-toplevel")
	if e != nil {
		return e
	}
	current, e := git(root, "rev-parse", "HEAD")
	if e != nil {
		return e
	}
	if current != *pin {
		return fmt.Errorf("verifier commit mismatch")
	}
	info, ok := debug.ReadBuildInfo()
	builtRevision, modified := "", ""
	if ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				builtRevision = setting.Value
			}
			if setting.Key == "vcs.modified" {
				modified = setting.Value
			}
		}
	}
	if builtRevision != *pin || modified != "false" {
		return fmt.Errorf("driver must be built with VCS identity from the clean pinned verifier")
	}
	branch, e := git(root, "rev-parse", "--abbrev-ref", "HEAD")
	if e != nil {
		return e
	}
	if branch != "HEAD" {
		return fmt.Errorf("verifier must be detached")
	}
	status, e := git(root, "status", "--porcelain", "--untracked-files=all")
	if e != nil {
		return e
	}
	if status != "" {
		return fmt.Errorf("dirty verifier refused")
	}
	baseSHA, headSHA, e := resolveRange(root, *base, *head)
	if e != nil {
		return e
	}
	// State and output are validated before any directory is created.
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	stateDir, e := safePath(filepath.Join(stateHome, "autarch-verify"), root)
	if e != nil {
		return e
	}
	out := *outArg
	if out == "" {
		out = filepath.Join(stateDir, "runs", headSHA[:8]+"-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	out, e = safePath(out, root)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(stateDir, 0700); e != nil {
		return e
	}
	if e = os.MkdirAll(out, 0700); e != nil {
		return e
	}
	if _, err := os.Lstat(filepath.Join(out, "report.json")); err == nil {
		return fmt.Errorf("output already contains a report")
	}
	// Stable across clones and XDG roots: this is a per-repository tool.
	lock, e := acquireRepoLock(repoLockName())
	if e != nil {
		return e
	}
	defer lock.Close()
	mergeBase, e := git(root, "merge-base", baseSHA, headSHA)
	if e != nil {
		return e
	}
	headTree, e := git(root, "rev-parse", headSHA+"^{tree}")
	if e != nil {
		return e
	}
	r := Report{Schema: "autarch-verify/v1", Base: baseSHA, Head: headSHA, MergeBase: mergeBase, HeadTree: headTree, Verifier: *pin, State: *state, PR: *pr, Artifacts: map[string]string{}, Cache: map[string]any{}, Suites: map[string]string{"go test -race ./...": "not supplied", "plugin vitest": "not supplied"}, SuiteSource: "not supplied", Journeys: []Result{}}
	if executable, err := os.Executable(); err == nil {
		if bytes, err := os.ReadFile(executable); err == nil {
			r.Artifacts["verifier_binary_sha256"] = hash(bytes)
		}
	}
	if *suiteFile != "" {
		var supplied struct {
			Head    string            `json:"head"`
			Results map[string]string `json:"results"`
		}
		suitePath, err := safePath(*suiteFile, root)
		if err != nil {
			return err
		}
		b, err := readOwnedFile(filepath.Dir(suitePath), suitePath)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &supplied); err != nil {
			return err
		}
		if supplied.Head != headSHA || len(supplied.Results) == 0 {
			return fmt.Errorf("suite evidence must name this exact head and results")
		}
		r.Suites = supplied.Results
		r.SuiteSource = "operator-supplied, unverified"
		r.SuiteClaimHead = supplied.Head
	}
	r.Manifest, r.ManifestHash, e = manifest(root)
	if e != nil {
		return e
	}
	r.E2ETree, _ = git(root, "rev-parse", headSHA+":"+plugin+"/e2e")
	rows, e := loadMap(filepath.Join(root, "scripts/verify/map.tsv"))
	if e != nil {
		return e
	}
	names, e := changed(root, baseSHA, headSHA)
	if e != nil {
		return e
	}
	r.Paths, _ = selectFlows(names, rows, nil)
	_, flows := selectFlows(names, rows, strings.Split(*flowsArg, ","))
	values, nothing := coverageVerdicts(r.Paths)
	if nothing {
		r.Cause = "NO-FLOW: no non-ignored paths changed; nothing verified from this diff."
	}
	for _, p := range r.Paths {
		if verifierInput(p.Path) {
			r.VerifierChanged = true
		}
	}
	work, e := os.MkdirTemp("/tmp", "autarch-verify-")
	if e != nil {
		return e
	}
	product := filepath.Join(work, "product")
	// Copy Git objects into the owned area: build helpers need git archive, but the
	// namespace must never expose the caller's home just to resolve a .git pointer.
	mirror := filepath.Join(work, "objects.git")
	productCreated := false
	defer func() {
		if cleanupErr := cleanupWork(work, mirror, product, productCreated); cleanupErr != nil {
			values = append(values, "INCOMPLETE")
			r.Cause = cleanupErr.Error()
		}
		r.Wall = time.Since(started).Seconds()
		r.Verdict = verdict(values)
		if err := writeReport(out, filepath.Join(stateDir, "ledger.jsonl"), r); err != nil {
			resultErr = err
			return
		}
		fmt.Printf("%s %s\n", r.Verdict, filepath.Join(out, "report.md"))
	}()
	if e = seedMirror(root, mirror, headSHA); e != nil {
		values = append(values, "INCOMPLETE")
		r.Cause = e.Error()
		return nil
	}
	if _, e = git(mirror, "worktree", "add", "--detach", product, headSHA); e != nil {
		values = append(values, "INCOMPLETE")
		r.Cause = e.Error()
		return nil
	}
	productCreated = true
	if err := productVerifierManifest(product, &r); err != nil {
		values = append(values, "INCOMPLETE")
		r.Cause = "head verifier inputs: " + err.Error()
		return nil
	}
	for _, flow := range flows {
		select {
		case <-stopRequested:
			values = append(values, "INCOMPLETE")
			r.Cause = "run interrupted"
			return nil
		default:
		}
		var row *Row
		for i := range rows {
			if rows[i].Flow == flow {
				row = &rows[i]
				break
			}
		}
		if row == nil || !row.Implemented {
			r.Journeys = append(r.Journeys, Result{Flow: flow, Verdict: "INCOMPLETE", Cause: "required journey not implemented", Label: func() string {
				if row != nil {
					return row.Label
				}
				return "new"
			}()})
			values = append(values, "INCOMPLETE")
			continue
		}
		j := runHomePick(work, product, out, *cacheRoot, *app, *seed, *row, &r)
		r.Journeys = append(r.Journeys, j)
		values = append(values, j.Verdict)
	}
	return nil
}

func runHomePick(work, product, out, cache, app, seed string, row Row, report *Report) (result Result) {
	started := time.Now()
	defer func() { result.Wall = time.Since(started).Seconds() }()
	incomplete := func(cause string) Result {
		return Result{Flow: row.Flow, Verdict: "INCOMPLETE", Label: row.Label, Cause: cause}
	}
	scenario, err := readOwnedFile(work, filepath.Join(product, plugin, "e2e/real.ts"))
	if err != nil || !strings.Contains(string(scenario), `"pick-click":`) {
		return incomplete("not exercisable at this head: pick-click scenario absent")
	}
	if cache == "" || app == "" || seed == "" {
		return incomplete("offline cache, owned bb app copy and toolchain seed required")
	}
	for _, p := range []string{cache, app, seed} {
		if _, e := safePath(p, product); e != nil {
			return incomplete(e.Error())
		}
		if _, e := os.Stat(p); e != nil {
			return incomplete(e.Error())
		}
	}
	report.AppSource = app
	// Hash the executable source plus package metadata; the launcher records its loaded identities separately.
	if _, e := os.Stat(filepath.Join(app, "dist/bb-app.js")); e != nil {
		return incomplete(e.Error())
	}
	var e error
	report.AppHash, e = treeHash(app)
	if e != nil {
		return incomplete(e.Error())
	}
	fake := filepath.Join(product, plugin, "e2e/fake-bb.mjs")
	f, e := readOwnedFile(work, fake)
	if e != nil {
		return incomplete("verified fake bb missing")
	}
	expected, e := git(product, "rev-parse", "HEAD:"+plugin+"/e2e/fake-bb.mjs")
	actual, hashErr := git(product, "hash-object", fake)
	if e != nil || hashErr != nil || actual != expected {
		return incomplete("fake bb hash mismatch")
	}
	report.Artifacts["fake_bb_sha256"] = hash(f)
	owned := filepath.Join(work, "owned")
	for _, d := range []string{"home", "state", "data", "config", "cache", "tmp", "bin"} {
		if e = os.MkdirAll(filepath.Join(owned, d), 0700); e != nil {
			return incomplete(e.Error())
		}
	}
	if e = os.WriteFile(filepath.Join(owned, "bin/fake-bb.mjs"), f, 0600); e != nil {
		return incomplete(e.Error())
	}
	node, npmCLI, nodeRoots, e := nodeToolchain()
	if e != nil {
		return incomplete(e.Error())
	}
	goBin, e := exec.LookPath("go")
	if e != nil {
		return incomplete(e.Error())
	}
	goBin, e = filepath.EvalSymlinks(goBin)
	if e != nil {
		return incomplete(e.Error())
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	shim := []byte("#!/bin/sh\nexec " + quote(node) + " " + quote(filepath.Join(owned, "bin/fake-bb.mjs")) + " \"$@\"\n")
	if e = os.WriteFile(filepath.Join(owned, "bin/bb"), shim, 0700); e != nil {
		return incomplete(e.Error())
	}
	report.Artifacts["fake_bb_shim_sha256"] = hash(shim)
	npmShim := []byte("#!/bin/sh\nexec " + quote(node) + " " + quote(npmCLI) + " \"$@\"\n")
	if e = os.WriteFile(filepath.Join(owned, "bin/npm"), npmShim, 0700); e != nil {
		return incomplete(e.Error())
	}
	report.Artifacts["npm_shim_sha256"] = hash(npmShim)
	tools := append([]string{filepath.Dir(filepath.Dir(goBin))}, nodeRoots...)
	for _, toolRoot := range tools {
		if _, err := safePath(toolRoot, product); err != nil {
			return incomplete(err.Error())
		}
	}
	metadata := func(name string, args []string) ([]byte, error) {
		log := filepath.Join(out, name+".log")
		r := runGroup(namespace(work, product, tools, args), product, childEnv(owned, "/usr/bin:/bin"), time.Duration(row.Seconds)*time.Second-time.Since(started), log)
		if r.Verdict != "PASS" {
			return nil, fmt.Errorf("toolchain metadata: %s", r.Cause)
		}
		return os.ReadFile(log)
	}
	v, e := metadata("go-version", []string{goBin, "version"})
	if e != nil {
		return incomplete(e.Error())
	}
	nodeVersion, e := metadata("node-version", []string{node, "-p", "process.version+'-'+process.versions.modules+'-'+process.platform+'-'+process.arch"})
	if e != nil {
		return incomplete(e.Error())
	}
	lock, e := readOwnedFile(work, filepath.Join(product, plugin, "package-lock.json"))
	if e != nil {
		return incomplete(e.Error())
	}
	nodeKey := hash(lock) + "-" + strings.TrimSpace(string(nodeVersion))
	goKey := hash(v)
	report.Cache["node_key"] = nodeKey
	report.Cache["go_key"] = goKey
	report.Cache["platform"] = runtime.GOOS + "/" + runtime.GOARCH
	npmCache := filepath.Join(cache, "node", nodeKey)
	modCache := filepath.Join(cache, "gomod")
	goCache := filepath.Join(cache, "go", goKey)
	var lockData struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(lock, &lockData); err != nil {
		return incomplete(err.Error())
	}
	playwrightVersion := lockData.Packages["node_modules/playwright-core"].Version
	if playwrightVersion == "" || strings.ContainsAny(playwrightVersion, "/\\") {
		return incomplete("missing Playwright cache version")
	}
	browser := filepath.Join(cache, "playwright", playwrightVersion)
	report.Cache["playwright_key"] = playwrightVersion
	for _, p := range []string{npmCache, modCache, goCache, browser} {
		if _, e = safePath(p, product); e != nil {
			return incomplete(e.Error())
		}
		if _, e = os.Stat(p); e != nil {
			report.Cache[p] = false
			return incomplete("offline cache miss: " + p)
		}
		report.Cache[p] = true
	}
	// The existing owned-server harness discovers its provider catalog through the
	// installed CLI. Expose only its self-contained executable, never its HOME.
	provider, err := exec.LookPath("claude")
	if err != nil {
		return incomplete("owned-server prerequisite: provider CLI missing")
	}
	provider, err = filepath.EvalSymlinks(provider)
	if err != nil {
		return incomplete(err.Error())
	}
	if _, err = safePath(provider, product); err != nil {
		return incomplete(err.Error())
	}
	providerBytes, err := readOwnedFile(filepath.Dir(provider), provider)
	if err != nil {
		return incomplete(err.Error())
	}
	if len(providerBytes) < 4 || string(providerBytes[:4]) != "\x7fELF" {
		return incomplete("owned-server prerequisite: self-contained provider executable required")
	}
	report.Artifacts["provider_cli_sha256"] = hash(providerBytes)
	if err = os.WriteFile(filepath.Join(owned, "bin/claude"), providerBytes, 0700); err != nil {
		return incomplete(err.Error())
	}
	// Writable Go build cache is owned by this run; dependency and browser seeds remain read-only.
	if e = os.MkdirAll(filepath.Join(owned, "gocache"), 0700); e != nil {
		return incomplete(e.Error())
	}
	env := childEnv(owned, filepath.Join(owned, "bin")+":"+filepath.Dir(node)+":"+filepath.Dir(goBin)+":/usr/bin:/bin")
	env = append(env, "GOMODCACHE="+modCache, "GOCACHE="+goCache, "npm_config_cache="+npmCache, "PLAYWRIGHT_BROWSERS_PATH="+browser)
	ro := append([]string{npmCache, modCache, browser, app, seed, filepath.Dir(filepath.Dir(goBin)), filepath.Join(owned, "bin"), filepath.Join(product, ".git")}, nodeRoots...)
	if _, err := os.Stat(filepath.Join(work, "objects.git")); err == nil {
		ro = append(ro, filepath.Join(work, "objects.git"))
	}
	step := func(name string, args []string, dir string) Result {
		remaining := time.Duration(row.Seconds)*time.Second - time.Since(started)
		if remaining <= 0 {
			return Result{Verdict: "INCOMPLETE", TimedOut: true, Cause: "outer journey deadline"}
		}
		argv := namespace(work, product, ro, args)
		// Only the explicitly keyed Go compilation cache is writable outside the run.
		for i, arg := range argv {
			if arg == "--chdir" {
				argv = append(argv[:i], append([]string{"--bind", goCache, goCache}, argv[i:]...)...)
				break
			}
		}
		return runGroup(argv, dir, env, remaining, filepath.Join(out, name+".log"))
	}
	// npm must run from the plugin directory, with offline settings reaching install hooks.
	npm := step("npm", []string{node, npmCLI, "--prefix", filepath.Join(product, plugin), "ci", "--offline", "--no-audit", "--no-fund"}, product)
	if npm.Verdict != "PASS" {
		npm.Flow = row.Flow
		npm.Label = row.Label
		npm.Cause = "offline npm build: " + npm.Cause
		return npm
	}
	if status, err := git(product, "status", "--porcelain", "--untracked-files=all"); err != nil || status != "" {
		return incomplete("build modified head sources")
	}
	buildFile := filepath.Join(work, "build.json")
	built := step("build", []string{node, filepath.Join(product, plugin, "scripts/build-identity.mjs"), "--repo", product, "--out", buildFile, "--scratch", filepath.Join(work, "build"), "--skip-npm-ci"}, product)
	if built.Verdict != "PASS" {
		built.Flow = row.Flow
		built.Label = row.Label
		built.Cause = "offline product build: " + built.Cause
		return built
	}
	if status, err := git(product, "status", "--porcelain", "--untracked-files=all"); err != nil || status != "" {
		return incomplete("build modified head sources")
	}
	var identity struct {
		PluginDir string `json:"plugin_dir"`
		Autarch   string `json:"autarch_path"`
		SHA       string `json:"autarch_sha256"`
		Source    string `json:"source_sha256"`
	}
	b, e := readOwnedFile(work, buildFile)
	if e != nil {
		return incomplete(e.Error())
	}
	if e = json.Unmarshal(b, &identity); e != nil {
		return incomplete(e.Error())
	}
	binary, e := readOwnedFile(work, identity.Autarch)
	if e != nil {
		return incomplete(e.Error())
	}
	if hash(binary) != identity.SHA {
		return incomplete("built autarch hash differs from build identity")
	}
	pluginDir, e := filepath.EvalSymlinks(identity.PluginDir)
	if e != nil {
		return incomplete(e.Error())
	}
	relative, e := filepath.Rel(work, pluginDir)
	if e != nil || relative == ".." || strings.HasPrefix(relative, "../") {
		return incomplete("built plugin escapes owned workspace")
	}
	report.Artifacts["autarch_sha256"] = hash(binary)
	report.Artifacts["plugin_sha256"] = identity.Source
	// The export needs the exact head's installed dependencies; no helper may fetch them.
	copied := step("plugin-deps", []string{"/bin/cp", "-a", filepath.Join(product, plugin, "node_modules"), identity.PluginDir}, product)
	if copied.Verdict != "PASS" {
		return incomplete("dependency copy failed")
	}
	env = append(env, "HOME_E2E_OUTER_NETNS="+func() string { s, _ := os.Readlink("/proc/self/ns/net"); return s }(), "HOME_E2E_TOOLCHAIN_SEED="+seed)
	evidence := filepath.Join(work, "journey.jsonl")
	result = step("home-pick", []string{node, filepath.Join(product, plugin, "node_modules/tsx/dist/cli.mjs"), filepath.Join(product, plugin, "e2e/harness.ts"), "--mode", "real-bb", "--owned-server", app, "--build", buildFile, "--install", "--run-id", hash([]byte(work))[:24], "--out", evidence, "--scenarios", "pick-click"}, product)
	lines, readErr := readOwnedFile(work, evidence)
	result.Flow = row.Flow
	result.Label = row.Label
	code := 0
	if result.Verdict != "PASS" {
		code = 1
	}
	result.Verdict = journeyVerdict(result.TimedOut, code, lines)
	if readErr != nil {
		result.Verdict = verdict([]string{result.Verdict, "INCOMPLETE"})
		if !result.TimedOut {
			result.Cause = "owned evidence: " + readErr.Error()
		}
	} else {
		if err := writeFresh(filepath.Join(out, "home-pick.jsonl"), lines); err != nil {
			result.Verdict = verdict([]string{result.Verdict, "INCOMPLETE"})
			result.Cause = "evidence write: " + err.Error()
		} else {
			result.Evidence = append(result.Evidence, filepath.Join(out, "home-pick.jsonl"))
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(lines)), "\n") {
		if json.Valid([]byte(line)) {
			result.Assertions = append(result.Assertions, json.RawMessage(line))
		}
	}
	return result
}
