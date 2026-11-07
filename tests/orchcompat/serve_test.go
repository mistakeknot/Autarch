package orchcompat

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
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

type liveServe struct {
	url       string
	token     string
	tokenPath string
	root      string
	cmd       *exec.Cmd
}

func startLive(t *testing.T, args ...string) *liveServe {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tp := filepath.Join(root, "tok", "serve.token")
	all := append([]string{"--addr", "127.0.0.1:0", "--token-file", tp}, args...)
	cmd, url, err := startServeAt(root, root, all...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
	})
	raw, err := os.ReadFile(tp)
	if err != nil {
		t.Fatal(err)
	}
	return &liveServe{url: url, token: strings.TrimSpace(string(raw)), tokenPath: tp, root: root, cmd: cmd}
}

func (s *liveServe) get(t *testing.T, method, path string, hdr map[string]string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(method, s.url+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

func (s *liveServe) auth() map[string]string {
	return map[string]string{"Authorization": "Bearer " + s.token}
}

func mkProjects(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func projectNames(t *testing.T, s *liveServe) []string {
	t.Helper()
	code, _, body := s.get(t, "GET", "/api/projects", s.auth())
	if code != 200 {
		t.Fatalf("/api/projects = %d %s", code, body)
	}
	var list []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, p := range list {
		out = append(out, p.Name)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestServeStartupLineAndToken(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	tp := filepath.Join(root, "nested", "dir", "serve.token")
	cmd := exec.Command(autarchBin, "serve", "--addr", "127.0.0.1:0", "--token-file", tp, "--project-dir", root)
	cmd.Dir = root
	cmd.Env = baseEnv(root)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	n, _ := out.Read(buf)
	line := string(buf[:n])
	cmd.Process.Signal(syscall.SIGTERM)
	werr := cmd.Wait()
	m := regexp.MustCompile(`^autarch serve listening on 127\.0\.0\.1:(\d+) \(token file: (.+)\)\n$`).FindStringSubmatch(line)
	if m == nil || m[1] == "8110" || m[2] != tp {
		t.Fatalf("startup line = %q", line)
	}
	if werr != nil {
		t.Errorf("exit after SIGTERM: %v", werr)
	}
	raw, err := os.ReadFile(tp)
	if err != nil {
		t.Fatal(err)
	}
	tok := strings.TrimSpace(string(raw))
	if _, err := hex.DecodeString(tok); err != nil || len(tok) != 64 || !strings.HasSuffix(string(raw), "\n") {
		t.Errorf("token file = %q, want 64 hex characters and a newline", raw)
	}
	if strings.Contains(line, tok) || strings.Contains(stderr.String(), tok) {
		t.Error("the token was printed")
	}
	if st, _ := os.Stat(tp); st.Mode().Perm() != 0o600 {
		t.Errorf("token file mode = %v, want 0600", st.Mode().Perm())
	}
	if st, _ := os.Stat(filepath.Dir(tp)); st.Mode().Perm() != 0o700 {
		t.Errorf("token directory mode = %v, want 0700", st.Mode().Perm())
	}
}

func TestServeReusesAnExistingToken(t *testing.T) {
	s := startLive(t)
	first := s.token
	s.cmd.Process.Signal(syscall.SIGTERM)
	s.cmd.Wait()
	cmd, url, err := startServeAt(s.root, s.root, "--addr", "127.0.0.1:0", "--token-file", s.tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Signal(syscall.SIGTERM); cmd.Wait() }()
	s2 := &liveServe{url: url, token: first}
	if code, _, _ := s2.get(t, "GET", "/api/projects", s2.auth()); code != 200 {
		t.Errorf("the old token no longer works: %d", code)
	}
	raw, _ := os.ReadFile(s.tokenPath)
	if strings.TrimSpace(string(raw)) != first {
		t.Error("the token file was rewritten")
	}
}

func TestServeStartupRefusals(t *testing.T) {
	good := strings.Repeat("ab", 32) + "\n"
	cases := []struct {
		name   string
		args   func(root string) []string
		prep   func(root string)
		err    string
		stdout bool
	}{
		{"token file readable by group", func(r string) []string { return []string{"--token-file", filepath.Join(r, "t")} },
			func(r string) { os.WriteFile(filepath.Join(r, "t"), []byte(good), 0o640) }, "has mode 0640; refusing (want 0600)", false},
		{"token file not 64 characters", func(r string) []string { return []string{"--token-file", filepath.Join(r, "t")} },
			func(r string) { os.WriteFile(filepath.Join(r, "t"), []byte("abc\n"), 0o600) }, "is not a 64 hex-character token; refusing", false},
		{"token file not hex", func(r string) []string { return []string{"--token-file", filepath.Join(r, "t")} },
			func(r string) { os.WriteFile(filepath.Join(r, "t"), []byte(strings.Repeat("zz", 32)+"\n"), 0o600) }, "is not a 64 hex-character token; refusing", false},
		{"non-loopback address", func(r string) []string { return []string{"--addr", "0.0.0.0:0", "--token-file", filepath.Join(r, "t")} }, nil,
			`refusing to bind non-loopback address "0.0.0.0"; Autarch is local-only by default`, false},
		{"a host name that is not loopback", func(r string) []string {
			return []string{"--addr", "example.com:0", "--token-file", filepath.Join(r, "t")}
		}, nil,
			`refusing to bind non-loopback address "example.com"`, false},
		{"an address with no host", func(r string) []string { return []string{"--addr", ":0", "--token-file", filepath.Join(r, "t")} }, nil,
			`refusing to bind non-loopback address ":0"`, false},
		{"positional argument", func(r string) []string { return []string{"extra"} }, nil, `unknown command "extra" for "autarch serve"`, false},
		{"unknown flag", func(r string) []string { return []string{"--nope"} }, nil, "unknown flag: --nope", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, _ := filepath.EvalSymlinks(t.TempDir())
			if c.prep != nil {
				c.prep(root)
				if st, err := os.Stat(filepath.Join(root, "t")); err == nil {
					os.Chmod(filepath.Join(root, "t"), st.Mode().Perm())
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, autarchBin, append([]string{"serve", "--project-dir", root}, c.args(root)...)...)
			cmd.Dir = root
			cmd.Env = baseEnv(root)
			var so, se bytes.Buffer
			cmd.Stdout, cmd.Stderr = &so, &se
			err := cmd.Run()
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("serve exited without an error: %v (stdout %q)", err, so.String())
			}
			if ee.ExitCode() == 0 || ctx.Err() != nil {
				t.Fatalf("exit %d, ctx %v", ee.ExitCode(), ctx.Err())
			}
			if so.Len() != 0 {
				t.Errorf("stdout = %q, want empty (no startup line)", so.String())
			}
			if !strings.Contains(se.String(), c.err) {
				t.Errorf("stderr %q lacks %q", se.String(), c.err)
			}
		})
	}
}

func TestServeHealthIsOpenAndReportsTheMounts(t *testing.T) {
	s := startLive(t)
	mkProjects(t, filepath.Join(s.root, "projects"), "p1", "p2")
	if err := os.MkdirAll(filepath.Join(s.root, "projects", "p1", ".gurgeh"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.stop(t)
	s = restartWith(t, s, "--project-dir", filepath.Join(s.root, "projects"))
	code, hdr, body := s.get(t, "GET", "/health", nil)
	if code != 200 || !strings.HasPrefix(hdr.Get("Content-Type"), "application/json") {
		t.Fatalf("/health = %d %v", code, hdr)
	}
	var h struct {
		Status   string                 `json:"status"`
		Bigend   string                 `json:"bigend"`
		Gurgeh   map[string]any         `json:"gurgeh"`
		Signals  string                 `json:"signals"`
		Projects int                    `json:"projects"`
		Build    map[string]any         `json:"build"`
		Extra    map[string]interface{} `json:"-"`
	}
	if err := json.Unmarshal([]byte(body), &h); err != nil {
		t.Fatal(err)
	}
	if h.Status != "ok" || h.Bigend != "ok" || h.Signals != "ok" || h.Projects != 2 {
		t.Errorf("health = %+v", h)
	}
	if h.Gurgeh["mounted"] != true || h.Gurgeh["projects"] != float64(1) {
		t.Errorf("gurgeh = %v, want mounted with 1 project", h.Gurgeh)
	}
	if rev, _ := h.Build["revision"].(string); rev == "" {
		t.Errorf("build = %v, want a revision", h.Build)
	}
	if sum, _ := h.Build["exe_sha256"].(string); len(sum) != 64 {
		t.Errorf("build = %v, want a 64-character exe_sha256", h.Build)
	}
	var keys map[string]json.RawMessage
	json.Unmarshal([]byte(body), &keys)
	if len(keys) != 6 {
		t.Errorf("/health has %d keys, want 6: %s", len(keys), body)
	}
}

func (s *liveServe) stop(t *testing.T) {
	t.Helper()
	s.cmd.Process.Signal(syscall.SIGTERM)
	s.cmd.Wait()
}

func restartWith(t *testing.T, s *liveServe, args ...string) *liveServe {
	t.Helper()
	all := append([]string{"--addr", "127.0.0.1:0", "--token-file", s.tokenPath}, args...)
	cmd, url, err := startServeAt(s.root, s.root, all...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Signal(syscall.SIGTERM); cmd.Wait() })
	return &liveServe{url: url, token: s.token, tokenPath: s.tokenPath, root: s.root, cmd: cmd}
}

func TestServeGuardOrderAndEndpoints(t *testing.T) {
	s := startLive(t)
	mkProjects(t, filepath.Join(s.root, "projects"), "p1")
	s = restartWith(t, s, "--project-dir", filepath.Join(s.root, "projects"))

	type want struct {
		code  int
		body  string
		wwwau bool
	}
	cases := []struct {
		name         string
		method, path string
		hdr          map[string]string
		want         want
	}{
		{"projects without a token", "GET", "/api/projects", nil, want{401, "unauthorized\n", true}},
		{"projects with a wrong token", "GET", "/api/projects", map[string]string{"Authorization": "Bearer nope"}, want{401, "unauthorized\n", true}},
		{"projects with a lowercase scheme", "GET", "/api/projects", map[string]string{"Authorization": "bearer " + s.token}, want{401, "unauthorized\n", true}},
		{"projects with the bare token", "GET", "/api/projects", map[string]string{"Authorization": s.token}, want{401, "unauthorized\n", true}},
		{"an unknown path still needs the token", "GET", "/nope", nil, want{401, "unauthorized\n", true}},
		{"an unknown path with the token", "GET", "/nope", s.auth(), want{404, "404 page not found\n", false}},
		{"health with a foreign origin", "GET", "/health", map[string]string{"Origin": "http://evil.example"}, want{403, "forbidden origin\n", false}},
		{"projects with a foreign origin and the token", "GET", "/api/projects", merge(s.auth(), map[string]string{"Origin": "http://evil.example"}), want{403, "forbidden origin\n", false}},
		{"projects with a foreign host and the token", "GET", "/api/projects", merge(s.auth(), map[string]string{"Host": "evil.example"}), want{403, "forbidden host\n", false}},
		{"health with a foreign host", "GET", "/health", map[string]string{"Host": "evil.example"}, want{403, "forbidden host\n", false}},
		{"origin is checked before host", "GET", "/health", map[string]string{"Host": "evil.example", "Origin": "http://evil.example"}, want{403, "forbidden origin\n", false}},
		{"host is checked before the token", "GET", "/api/projects", map[string]string{"Host": "evil.example"}, want{403, "forbidden host\n", false}},
		{"localhost is an accepted host", "GET", "/api/projects", merge(s.auth(), map[string]string{"Host": "localhost:9"}), want{200, "", false}},
		{"projects only answers GET", "POST", "/api/projects", s.auth(), want{405, "", false}},
		{"health only answers GET", "POST", "/health", nil, want{405, "", false}},
		{"gurgeh of a project with no .gurgeh directory", "GET", "/gurgeh/p1/health", s.auth(), want{404, "404 page not found\n", false}},
		{"gurgeh of an unknown project", "GET", "/gurgeh/zzz/health", s.auth(), want{404, "404 page not found\n", false}},
		{"gurgeh needs the token", "GET", "/gurgeh/p1/health", nil, want{401, "unauthorized\n", true}},
		{"signals health", "GET", "/signals/health", s.auth(), want{200, "", false}},
		{"signals needs the token", "GET", "/signals/health", nil, want{401, "unauthorized\n", true}},
		{"bigend health", "GET", "/bigend/health", s.auth(), want{200, "", false}},
		{"bigend needs the token", "GET", "/bigend/health", nil, want{401, "unauthorized\n", true}},
		{"bigend projects", "GET", "/bigend/api/projects", s.auth(), want{200, "", false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, hdr, body := s.get(t, c.method, c.path, c.hdr)
			if code != c.want.code {
				t.Fatalf("status = %d (%q), want %d", code, body, c.want.code)
			}
			if c.want.body != "" && body != c.want.body {
				t.Errorf("body = %q, want %q", body, c.want.body)
			}
			if got := hdr.Get("WWW-Authenticate") == "Bearer"; got != c.want.wwwau {
				t.Errorf("WWW-Authenticate Bearer = %v, want %v", got, c.want.wwwau)
			}
		})
	}
}

func merge(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func TestServeProjectsListing(t *testing.T) {
	s := startLive(t)
	dir := filepath.Join(s.root, "projects")
	mkProjects(t, dir, "zeta", "alpha", "mid")
	if err := os.WriteFile(filepath.Join(dir, "afile"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s = restartWith(t, s, "--project-dir", dir)
	code, hdr, body := s.get(t, "GET", "/api/projects", s.auth())
	if code != 200 || !strings.HasPrefix(hdr.Get("Content-Type"), "application/json") {
		t.Fatalf("%d %v", code, hdr)
	}
	var list []struct {
		Name string `json:"name"`
		Root string `json:"root"`
		Dev  uint64 `json:"dev"`
		Ino  uint64 `json:"ino"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].Name != "alpha" || list[1].Name != "mid" || list[2].Name != "zeta" {
		t.Fatalf("projects = %+v, want alpha, mid, zeta sorted with the plain file left out", list)
	}
	for _, p := range list {
		if p.Root != filepath.Join(dir, p.Name) || p.Dev == 0 || p.Ino == 0 {
			t.Errorf("project %+v, want root %s and nonzero dev and ino", p, filepath.Join(dir, p.Name))
		}
	}
}

func TestServeProjectsListingWithNoProjectsIsAnEmptyArray(t *testing.T) {
	s := startLive(t, "--project-dir", t.TempDir())
	_, _, body := s.get(t, "GET", "/api/projects", s.auth())
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("body = %q, want []", body)
	}
}

func TestServeMountsGurgehForProjectsWithAGurgehDirectory(t *testing.T) {
	s := startLive(t)
	dir := filepath.Join(s.root, "projects")
	mkProjects(t, dir, "p1", "p2/.gurgeh")
	s = restartWith(t, s, "--project-dir", dir)
	if code, _, body := s.get(t, "GET", "/gurgeh/p2/health", s.auth()); code != 200 {
		t.Errorf("/gurgeh/p2/health = %d %q", code, body)
	}
	if code, _, _ := s.get(t, "GET", "/gurgeh/p1/health", s.auth()); code != 404 {
		t.Errorf("/gurgeh/p1/health = %d, want 404", code)
	}
	if code, _, _ := s.get(t, "GET", "/gurgeh/p2/api/specs", s.auth()); code != 200 {
		t.Errorf("/gurgeh/p2/api/specs = %d, want 200", code)
	}
}

func TestServeAllowOrigin(t *testing.T) {
	s := startLive(t, "--project-dir", t.TempDir(), "--allow-origin", "http://app.example/")
	ok := merge(s.auth(), map[string]string{"Origin": "http://app.example"})
	if code, _, _ := s.get(t, "GET", "/api/projects", ok); code != 200 {
		t.Errorf("allowed origin = %d, want 200", code)
	}
	ok["Origin"] = "http://app.example/"
	if code, _, _ := s.get(t, "GET", "/api/projects", ok); code != 200 {
		t.Errorf("allowed origin with a trailing slash = %d, want 200", code)
	}
	other := merge(s.auth(), map[string]string{"Origin": "http://other.example"})
	if code, _, _ := s.get(t, "GET", "/api/projects", other); code != 403 {
		t.Errorf("other origin = %d, want 403", code)
	}
}

func writeConfig(t *testing.T, path, scanRoot string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[discovery]\nscan_roots = [\""+scanRoot+"\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestServeDefaultScanRootIsProjectsUnderHome(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	mkProjects(t, filepath.Join(root, "projects"), "from-home-2", "from-home-1")
	mkProjects(t, filepath.Join(root, "elsewhere"), "not-listed")
	s := startNoDirs(t, root, root)
	if got := projectNames(t, s); !sameStrings(got, []string{"from-home-1", "from-home-2"}) {
		t.Errorf("projects = %v", got)
	}
}

func startNoDirs(t *testing.T, cwd, home string) *liveServe {
	t.Helper()
	tp := filepath.Join(home, "serve.token")
	cmd, url, err := startServeAt(cwd, home, "--addr", "127.0.0.1:0", "--token-file", tp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Signal(syscall.SIGTERM); cmd.Wait() })
	raw, err := os.ReadFile(tp)
	if err != nil {
		t.Fatal(err)
	}
	return &liveServe{url: url, token: strings.TrimSpace(string(raw)), tokenPath: tp, root: home, cmd: cmd}
}

func TestServeConfigCandidateOrder(t *testing.T) {
	type layout struct{ bigend, vauxhall, local bool }
	cases := []struct {
		name string
		l    layout
		want string
	}{
		{"bigend config wins over the others", layout{true, true, true}, "from-bigend"},
		{"vauxhall config is the legacy fallback", layout{false, true, true}, "from-vauxhall"},
		{"config.toml in the working directory is last", layout{false, false, true}, "from-local"},
		{"no config falls back to projects under home", layout{false, false, false}, "from-default"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, _ := filepath.EvalSymlinks(t.TempDir())
			home := filepath.Join(root, "home")
			cwd := filepath.Join(root, "cwd")
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatal(err)
			}
			mkProjects(t, filepath.Join(root, "bigend-roots"), "from-bigend")
			mkProjects(t, filepath.Join(root, "vauxhall-roots"), "from-vauxhall")
			mkProjects(t, filepath.Join(root, "local-roots"), "from-local")
			mkProjects(t, filepath.Join(home, "projects"), "from-default")
			if c.l.bigend {
				writeConfig(t, filepath.Join(home, ".config", "bigend", "config.toml"), filepath.Join(root, "bigend-roots"))
			}
			if c.l.vauxhall {
				writeConfig(t, filepath.Join(home, ".config", "vauxhall", "config.toml"), filepath.Join(root, "vauxhall-roots"))
			}
			if c.l.local {
				writeConfig(t, filepath.Join(cwd, "config.toml"), filepath.Join(root, "local-roots"))
			}
			s := startNoDirs(t, cwd, home)
			if got := projectNames(t, s); !sameStrings(got, []string{c.want}) {
				t.Errorf("projects = %v, want [%s]", got, c.want)
			}
		})
	}
}

func TestServeConfigScanRootsExpandHome(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	home := filepath.Join(root, "home")
	mkProjects(t, filepath.Join(home, "code"), "tilde-project")
	writeConfig(t, filepath.Join(home, ".config", "bigend", "config.toml"), "~/code")
	s := startNoDirs(t, root, home)
	if got := projectNames(t, s); !sameStrings(got, []string{"tilde-project"}) {
		t.Errorf("projects = %v", got)
	}
}

func TestServeProjectDirOverridesTheConfig(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	home := filepath.Join(root, "home")
	mkProjects(t, filepath.Join(root, "cfg"), "from-config")
	mkProjects(t, filepath.Join(root, "flag-a"), "from-flag-a")
	mkProjects(t, filepath.Join(root, "flag-b"), "from-flag-b")
	writeConfig(t, filepath.Join(home, ".config", "bigend", "config.toml"), filepath.Join(root, "cfg"))
	tp := filepath.Join(home, "serve.token")
	cmd, url, err := startServeAt(root, home, "--addr", "127.0.0.1:0", "--token-file", tp,
		"--project-dir", filepath.Join(root, "flag-a"), "--project-dir", filepath.Join(root, "flag-b"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Signal(syscall.SIGTERM); cmd.Wait() })
	raw, _ := os.ReadFile(tp)
	s := &liveServe{url: url, token: strings.TrimSpace(string(raw))}
	if got := projectNames(t, s); !sameStrings(got, []string{"from-flag-a", "from-flag-b"}) {
		t.Errorf("projects = %v", got)
	}
}

func TestServeBadConfigIsFatal(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	home := filepath.Join(root, "home")
	cfg := filepath.Join(home, ".config", "bigend", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("[discovery\nscan_roots = \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, autarchBin, "serve", "--addr", "127.0.0.1:0", "--token-file", filepath.Join(home, "t"))
	cmd.Dir = root
	cmd.Env = baseEnv(home)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() == 0 {
		t.Fatalf("serve did not fail: %v", err)
	}
	if !strings.Contains(se.String(), "load bigend discovery defaults:") || so.Len() != 0 {
		t.Errorf("stdout %q stderr %q", so.String(), se.String())
	}
	if _, err := os.Stat(filepath.Join(home, "t")); err == nil {
		t.Error("a token file was created although the config failed to load")
	}
}

func TestServeDefaultTokenFileIsUnderHome(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	cmd, _, err := startServeAt(root, root, "--addr", "127.0.0.1:0", "--project-dir", root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Signal(syscall.SIGTERM); cmd.Wait() }()
	st, err := os.Stat(filepath.Join(root, ".autarch", "serve.token"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("default token file: %v %v", st, err)
	}
}

func TestServeDefaultAddressIsLoopback8110(t *testing.T) {
	out, err := exec.Command(autarchBin, "serve", "--help").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`--addr string`, `(default "127.0.0.1:8110")`, `--token-file string`, `--allow-origin stringArray`, `--project-dir stringArray`} {
		if !strings.Contains(string(out), w) {
			t.Errorf("serve --help lacks %q:\n%s", w, out)
		}
	}
}
