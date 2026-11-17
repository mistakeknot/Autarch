//go:build linux

// Source of requirements: bead mk-05q0j.3.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var contains = strings.Contains
var stopRequested = make(chan struct{})

// Read a generated input only after resolving it inside the owned workspace.
// Non-blocking open plus the regular-file check also rejects FIFO evidence.
func readOwnedFile(root, name string) ([]byte, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	name, err = filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(name)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, fmt.Errorf("owned input escapes workspace")
	}
	fd, err := syscall.Open(real, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), real)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("owned input is not a regular file")
	}
	return io.ReadAll(f)
}

type Row struct {
	Glob        string `json:"glob"`
	Flow        string `json:"flow"`
	Coverage    string `json:"coverage"`
	Implemented bool   `json:"implemented"`
	Label       string `json:"label"`
	Seconds     int    `json:"deadline_seconds"`
	Behavior    string `json:"behavior"`
}
type Mapping struct {
	Path     string   `json:"path"`
	Coverage string   `json:"coverage"`
	Flows    []string `json:"flows"`
}

func verdict(values []string) string {
	if len(values) == 0 {
		return "NO-FLOW"
	}
	for _, v := range []string{"FAIL", "INCOMPLETE", "VERIFIER-CHANGED", "NO-FLOW"} {
		for _, have := range values {
			if have == v {
				return v
			}
		}
	}
	return "PASS"
}

// The map uses git's :(glob) pathspec subset: *, ** and ?. Unsupported syntax is refused.
func matches(glob, name string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(glob[i : i+1]))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String()).MatchString(name)
}
func ignored(name string) bool {
	return (strings.HasPrefix(name, "docs/") && strings.HasSuffix(name, ".md")) || (!strings.Contains(name, "/") && strings.HasSuffix(name, ".md") && name != "AGENTS.md" && name != "CLAUDE.md" && name != "CODING_STANDARDS.md")
}
func selectFlows(names []string, rows []Row, extra []string) ([]Mapping, []string) {
	paths := []Mapping{}
	flows := []string{}
	add := func(v string) {
		for _, s := range flows {
			if s == v {
				return
			}
		}
		flows = append(flows, v)
	}
	for _, name := range names {
		m := Mapping{Path: name, Coverage: "NO-FLOW", Flows: []string{}}
		if ignored(name) {
			m.Coverage = "ignored"
		} else {
			for _, r := range rows {
				if matches(r.Glob, name) {
					m.Flows = append(m.Flows, r.Flow)
					add(r.Flow)
					if r.Coverage == "full" {
						m.Coverage = "full"
					}
				}
			}
		}
		paths = append(paths, m)
	}
	for _, v := range extra {
		if v != "" {
			add(v)
		}
	}
	return paths, flows
}
func safePath(name, checkout string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	if abs == "/" {
		return "", fmt.Errorf("refusing filesystem root")
	}
	for _, component := range strings.Split(abs, string(filepath.Separator)) {
		if component == ".bb-machines" {
			return "", fmt.Errorf("refusing live machine path")
		}
	}
	for p := abs; p != "/"; p = filepath.Dir(p) {
		if _, e := os.Lstat(filepath.Join(p, ".git")); e == nil {
			return "", fmt.Errorf("refusing path within a checkout")
		}
		st, e := os.Lstat(p)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return "", e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing symlink: %s", p)
		}
	}
	roots := []string{checkout, filepath.Join(os.Getenv("HOME"), ".bb"), filepath.Join(os.Getenv("HOME"), ".bb-machines"), filepath.Join(os.Getenv("HOME"), ".local", "share", "autarch"), filepath.Join(os.Getenv("HOME"), ".autarch")}
	// Ambient overrides are consulted only to deny destinations, never to launch children.
	if live := os.Getenv("AUTARCH_HOME_DB"); filepath.IsAbs(live) {
		roots = append(roots, filepath.Dir(live))
	}
	for _, key := range []string{"HOME_BB_DATA", "BB_DATA_DIR"} {
		if live := os.Getenv(key); filepath.IsAbs(live) {
			roots = append(roots, live)
		}
	}
	for _, root := range roots {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		if root != "" && (abs == root || strings.HasPrefix(abs, root+"/") || strings.HasPrefix(root, abs+"/")) {
			return "", fmt.Errorf("refusing checkout or live Home path")
		}
	}
	return abs, nil
}
func childEnv(root, path string) []string {
	return []string{"PATH=" + path, "HOME=" + root + "/home", "XDG_STATE_HOME=" + root + "/state", "XDG_DATA_HOME=" + root + "/data", "XDG_CONFIG_HOME=" + root + "/config", "XDG_CACHE_HOME=" + root + "/cache", "TMPDIR=" + root + "/tmp", "LANG=C.UTF-8", "AUTARCH_HOME_DB=" + root + "/home.db", "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-mod=readonly -p=4", "GOENV=off", "GOTOOLCHAIN=local", "GOWORK=off", "GOMAXPROCS=4", "npm_config_offline=true", "npm_config_audit=false", "npm_config_fund=false", "npm_config_jobs=4", "npm_config_nodedir=/usr", "MAKEFLAGS=-j4", "UV_THREADPOOL_SIZE=4"}
}

type Result struct {
	Flow       string            `json:"flow"`
	Verdict    string            `json:"verdict"`
	Label      string            `json:"label"`
	Wall       float64           `json:"wall_seconds"`
	TimedOut   bool              `json:"timed_out"`
	Command    []string          `json:"command"`
	Evidence   []string          `json:"evidence"`
	Assertions []json.RawMessage `json:"assertions"`
	Cause      string            `json:"cause,omitempty"`
}

func runGroup(argv []string, dir string, env []string, deadline time.Duration, log string) Result {
	started := time.Now()
	r := Result{Command: argv, Evidence: []string{}, Assertions: []json.RawMessage{}, Verdict: "INCOMPLETE"}
	f, err := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		r.Cause = err.Error()
		return r
	}
	defer f.Close()
	r.Evidence = append(r.Evidence, log)
	command := append([]string{"-n", "19", "/usr/bin/ionice", "-c3"}, argv...)
	cmd := exec.Command("/usr/bin/nice", command...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = f
	cmd.Stderr = f
	err, r.TimedOut = waitGroup(cmd, deadline)
	r.Wall = time.Since(started).Seconds()
	if err == nil && !r.TimedOut {
		r.Verdict = "PASS"
	} else {
		r.Cause = fmt.Sprint(err)
		if r.TimedOut {
			r.Cause = "outer deadline"
		}
	}
	return r
}

// Metadata helpers and journeys share the same process lifetime contract.
func checkLaunch(argv []string) error {
	refuse := func() error { return fmt.Errorf("launch policy: refusing live or opaque shell command") }
	if len(argv) == 0 {
		return refuse()
	}
	for _, arg := range argv {
		if filepath.Base(arg) == "home-ui.sh" {
			return refuse()
		}
	}
	switch filepath.Base(argv[0]) {
	case "bb", "sh", "bash", "dash", "zsh", "ksh", "fish", "busybox":
		return refuse()
	case "nice":
		if len(argv) < 4 || argv[1] != "-n" || argv[2] != "19" {
			return refuse()
		}
		return checkLaunch(argv[3:])
	case "ionice":
		if len(argv) < 3 || argv[1] != "-c3" {
			return refuse()
		}
		return checkLaunch(argv[2:])
	case "bwrap":
		for i, arg := range argv[1:] {
			if arg == "--" {
				return checkLaunch(argv[i+2:])
			}
		}
		return refuse()
	case "env":
		for i := 1; i < len(argv); i++ {
			if argv[i] == "-i" || argv[i] == "--ignore-environment" || strings.Contains(argv[i], "=") {
				continue
			}
			if strings.HasPrefix(argv[i], "-") {
				return refuse()
			}
			return checkLaunch(argv[i:])
		}
	}
	return nil
}

func waitGroup(cmd *exec.Cmd, deadline time.Duration) (error, bool) {
	if len(cmd.Args) == 0 || filepath.Base(cmd.Path) != filepath.Base(cmd.Args[0]) {
		return fmt.Errorf("launch policy: executable differs from argv"), false
	}
	if err := checkLaunch(cmd.Args); err != nil {
		return err, false
	}
	select {
	case <-stopRequested:
		return fmt.Errorf("interrupted"), false
	default:
	}
	if deadline <= 0 {
		return fmt.Errorf("outer deadline"), true
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err, false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	var err error
	timedOut := false
	select {
	case err = <-done:
	case <-stopRequested:
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = fmt.Errorf("interrupted: %v", <-done)
	case <-timer.C:
		timedOut = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = <-done
	}
	// A successful parent may have left descendants; never leave its process group behind.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return err, timedOut
}
func classifyHarness(code int, lines []byte) string {
	seen := false
	states := []string{}
	for _, line := range bytes.Split(lines, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r struct {
			Scenario string `json:"scenario"`
			Pass     *bool  `json:"pass"`
		}
		if json.Unmarshal(line, &r) != nil || r.Pass == nil || r.Scenario != "pick-click" {
			states = append(states, "INCOMPLETE")
			continue
		}
		seen = true
		if !*r.Pass {
			states = append(states, "FAIL")
		}
	}
	if !seen || code != 0 {
		states = append(states, "INCOMPLETE")
	}
	if seen && len(states) == 0 {
		states = append(states, "PASS")
	}
	return verdict(states)
}

func journeyVerdict(timedOut bool, code int, lines []byte) string {
	observed := classifyHarness(code, lines)
	if observed == "FAIL" {
		return "FAIL"
	}
	if timedOut {
		return "INCOMPLETE"
	}
	return observed
}
