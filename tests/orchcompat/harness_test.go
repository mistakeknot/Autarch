package orchcompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	autarchBin string
	testBin    string
	shared     struct {
		root     string
		projects string
		alpha    string
		beta     string
		url      string
		token    string
	}
)

func TestMain(m *testing.M) {
	if os.Getenv("ORCHCOMPAT_FAKEBB") == "1" {
		os.Exit(fakeBBMain(os.Args[1:]))
	}
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	root, err := os.MkdirTemp("", "orchcompat-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(root)
	if root, err = filepath.EvalSymlinks(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if testBin, err = os.Executable(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	autarchBin = os.Getenv("ORCHCOMPAT_AUTARCH")
	if autarchBin == "" {
		autarchBin = filepath.Join(root, "autarch")
		build := exec.Command("go", "build", "-o", autarchBin, "github.com/mistakeknot/autarch/cmd/autarch")
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "build autarch:", err)
			return 1
		}
	}
	shared.root = root
	shared.projects = filepath.Join(root, "projects")
	shared.alpha = filepath.Join(shared.projects, "alpha")
	shared.beta = filepath.Join(shared.projects, "beta")
	for _, d := range []string{shared.alpha, shared.beta} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	shared.token = filepath.Join(root, "serve.token")
	srv, url, err := startServe(root, "--addr", "127.0.0.1:0", "--project-dir", shared.projects, "--token-file", shared.token)
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	shared.url = url
	code := m.Run()
	srv.Process.Signal(syscall.SIGTERM)
	srv.Wait()
	return code
}

var listenRe = regexp.MustCompile(`^autarch serve listening on (127\.0\.0\.1:\d+) \(token file: (.+)\)$`)

func baseEnv(home string, extra ...string) []string {
	env := []string{"HOME=" + home, "PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C", "GIT_CONFIG_NOSYSTEM=1"}
	if v := os.Getenv("GOCACHE"); v != "" {
		env = append(env, "GOCACHE="+v)
	}
	return append(env, extra...)
}

func startServe(home string, args ...string) (*exec.Cmd, string, error) {
	return startServeAt(home, home, args...)
}

func startServeAt(dir, home string, args ...string) (*exec.Cmd, string, error) {
	cmd := exec.Command(autarchBin, append([]string{"serve"}, args...)...)
	cmd.Dir = dir
	cmd.Env = baseEnv(home)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	line := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(out)
		if sc.Scan() {
			line <- sc.Text()
		}
		close(line)
		for sc.Scan() {
		}
	}()
	select {
	case l, ok := <-line:
		if m := listenRe.FindStringSubmatch(l); ok && m != nil {
			return cmd, "http://" + m[1], nil
		}
		cmd.Process.Kill()
		cmd.Wait()
		return nil, "", fmt.Errorf("unexpected first line %q", l)
	case <-time.After(20 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		return nil, "", fmt.Errorf("serve did not report a listen address")
	}
}

const subprocessDeadline = 60 * time.Second

var httpClient = &http.Client{Timeout: 20 * time.Second}

func helpOutput(t *testing.T, args ...string) string {
	t.Helper()
	home, err := os.MkdirTemp("", "orchcompat-help-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	ctx, cancel := context.WithTimeout(context.Background(), subprocessDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, autarchBin, args...)
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = home
	cmd.Env = baseEnv(home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("autarch %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func closedLoopbackAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

type result struct {
	Code   int
	Stdout string
	Stderr string
}

type env struct {
	t      *testing.T
	root   string
	home   string
	run    string
	state  string
	tmp    string
	bin    string
	fbdir  string
	thread string
	cwd    string
	vars   map[string]string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, root: root, home: filepath.Join(root, "home"), run: filepath.Join(root, "run"), state: filepath.Join(root, "state"),
		tmp: filepath.Join(root, "tmp"), bin: filepath.Join(root, "bin"), fbdir: filepath.Join(root, "fakebb"), thread: "thr_a", vars: map[string]string{}}
	for _, d := range []string{e.home, e.run, e.state, e.tmp, e.bin, e.fbdir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	wrapper := fmt.Sprintf("#!/bin/sh\nORCHCOMPAT_FAKEBB=1 GORACE=atexit_sleep_ms=0 exec %q \"$@\"\n", testBin)
	if err := os.WriteFile(filepath.Join(e.bin, "bb"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	e.update(func(s *fbState) {
		s.Projects = []fbProject{{ID: "P1", Name: "P1", Prefix: "P1"}}
		s.Bindings["P1"] = fbBinding{Home: "alpha", State: "confirmed"}
	})
	return e
}

func (e *env) update(fn func(*fbState)) {
	e.t.Helper()
	s, err := fbLoad(e.fbdir)
	if err != nil {
		e.t.Fatal(err)
	}
	fn(s)
	if err := fbSave(e.fbdir, s); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) st() *fbState {
	e.t.Helper()
	s, err := fbLoad(e.fbdir)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *env) calls() []fbCall {
	e.t.Helper()
	_, cp, _ := fbPaths(e.fbdir)
	raw, err := os.ReadFile(cp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		e.t.Fatal(err)
	}
	var out []fbCall
	for _, l := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		if len(l) == 0 {
			continue
		}
		var c fbCall
		if err := json.Unmarshal(l, &c); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

func (e *env) callsWith(prefix ...string) []fbCall {
	var out []fbCall
	for _, c := range e.calls() {
		if len(c.Args) >= len(prefix) && strings.Join(c.Args[:len(prefix)], " ") == strings.Join(prefix, " ") {
			out = append(out, c)
		}
	}
	return out
}

func (e *env) environ() []string {
	env := baseEnv(e.home,
		"PATH="+e.bin+":/usr/local/bin:/usr/bin:/bin",
		"XDG_RUNTIME_DIR="+e.run,
		"XDG_STATE_HOME="+e.state,
		"TMPDIR="+e.tmp,
		"ORCHCOMPAT_FAKEBB_DIR="+e.fbdir,
		"AUTARCH_SERVE_URL="+shared.url,
		"AUTARCH_SERVE_TOKEN_FILE="+shared.token,
		"AUTARCH_TASKS_PROJECT=P1",
		"GIT_CEILING_DIRECTORIES="+filepath.Dir(shared.root),
	)
	if e.thread != "" {
		env = append(env, "BB_THREAD_ID="+e.thread)
	}
	for k, v := range e.vars {
		env = append(env, k+"="+v)
	}
	return env
}

func (e *env) autarch(args ...string) result {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), subprocessDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, autarchBin, args...)
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = e.root
	if e.cwd != "" {
		cmd.Dir = e.cwd
	}
	cmd.Env = e.environ()
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	r := result{Stdout: so.String(), Stderr: se.String()}
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok || ctx.Err() != nil {
			e.t.Fatalf("autarch %v: %v (deadline %v)", args, err, subprocessDeadline)
		}
		r.Code = ee.ExitCode()
	}
	return r
}

func (e *env) writeFile(name, content string) string {
	e.t.Helper()
	p := filepath.Join(e.root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func askJSON(extra map[string]any) string {
	m := map[string]any{
		"question": "Ship it?",
		"options": []any{
			map[string]any{"id": "yes", "label": "Yes", "kind": "ruling-only"},
			map[string]any{"id": "no", "label": "No", "kind": "ruling-only"},
		},
		"project":      "alpha",
		"project_root": shared.alpha,
	}
	for k, v := range extra {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

type filed struct {
	Card    string `json:"card"`
	Request string `json:"request"`
	Project string `json:"project"`
	Replay  bool   `json:"replay"`
	CardKey string `json:"card_key"`
}

func parseFiled(t *testing.T, r result) filed {
	t.Helper()
	var f filed
	if err := json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &f); err != nil {
		t.Fatalf("stdout is not one JSON object: %q (%v)", r.Stdout, err)
	}
	return f
}
