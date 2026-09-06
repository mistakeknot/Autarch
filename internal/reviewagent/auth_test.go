package reviewagent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/review"
)

func TestAuthEventsNeverBecomeReviewRecords(t *testing.T) {
	store, _ := review.Open(t.TempDir())
	e := New(store)
	c := &conversation{project: t.TempDir(), session: "session", in: &inputBuffer{}}
	for _, raw := range []string{
		`{"type":"auth_state","data":{"runtimeId":"runtime","operation":{"id":"op","status":"pending","events":[{"type":"auth_url","url":"https://example.test/?code=PRIVATE"}]}}}`,
		`{"type":"response","command":"auth_response","id":"auth-private","success":false,"error":"PRIVATE"}`,
	} {
		var event map[string]json.RawMessage
		_ = json.Unmarshal([]byte(raw), &event)
		e.event(c, event)
	}
	state, _ := json.Marshal(store.Snapshot())
	if strings.Contains(string(state), "PRIVATE") || len(store.Snapshot().Turns) != 0 {
		t.Fatal("authentication leaked into durable review records")
	}
	if c.auth == nil || c.auth.RuntimeID != "runtime" {
		t.Fatal("transient authentication lost")
	}
}

func TestCredentialSandboxRejectsProjectStorage(t *testing.T) {
	if _, err := credentialProfile("/project", "/project/auth.json", "/project/auth.json.lock"); err == nil {
		t.Fatal("credential grant allows project writes")
	}
	profile, err := credentialProfile("/project", "/credentials/auth.json", "/credentials/auth.json.lock")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(profile, `(literal "/credentials/auth.json")`) || strings.Contains(profile, `(subpath "/credentials")`) {
		t.Fatal("credential grant is broader than the file and lock")
	}
}

func TestCredentialSandboxAllowsLockButDeniesProjectWrites(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	credentials, _ := filepath.EvalSymlinks(t.TempDir())
	path := filepath.Join(credentials, "auth.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	grant, err := credentialProfile(root, path, path+".lock")
	if err != nil {
		t.Fatal(err)
	}
	profile := "(version 1)\n(allow default)\n(deny file-write*)\n" + grant
	allowed := exec.Command("/usr/bin/sandbox-exec", "-p", profile, "/bin/sh", "-c", `mkdir "$1.lock" && printf '{}' > "$1" && rmdir "$1.lock"`, "fixture", path)
	if data, err := allowed.CombinedOutput(); err != nil {
		t.Fatalf("credential lock denied: %v %s", err, data)
	}
	denied := exec.Command("/usr/bin/sandbox-exec", "-p", profile, "/usr/bin/touch", filepath.Join(root, "denied"))
	if err := denied.Run(); err == nil {
		t.Fatal("project write succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, "denied")); !os.IsNotExist(err) {
		t.Fatal("project changed")
	}
}

func TestCredentialPreflightBoundsEscapedPipeWait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "flere")
	pidfile := filepath.Join(dir, "pid")
	script := "#!/bin/sh\nsleep 30 &\nprintf '%s' $! > " + strconv.Quote(pidfile) + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	defer func() {
		data, _ := os.ReadFile(pidfile)
		pid, _ := strconv.Atoi(string(data))
		if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}()
	start := time.Now()
	_, err := prepareCredentials(bin, t.TempDir())
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatal("credential preflight retained an escaped stdout pipe")
	}
}

func TestLiveFlereProviderControlPlaneWithIsolatedCredentials(t *testing.T) {
	if os.Getenv("AUTARCH_LIVE_FLERE_AUTH") != "1" || runtime.GOOS != "darwin" {
		t.Skip("explicit local RPC smoke; no provider network call")
	}
	t.Setenv("FLERE_CODING_AGENT_DIR", t.TempDir())
	t.Setenv("PI_OFFLINE", "1")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	dir, err := os.MkdirTemp("/tmp", "a-auth-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store, _ := review.Open(dir)
	e := New(store)
	defer func() {
		e.mu.Lock()
		c := e.runtimes[root]
		e.mu.Unlock()
		if c != nil {
			_ = c.in.Close()
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM)
			<-c.done
		}
	}()
	call := func(method string, a *review.AuthRequest) *review.AuthState {
		t.Helper()
		r := e.Auth(review.Request{Project: root, Method: method, Auth: a})
		if r.Error != "" {
			for _, turn := range store.Snapshot().Turns {
				t.Log(turn.Kind, turn.Text)
			}
			t.Fatal(method, r.Error)
		}
		return r.Auth
	}
	state := call("auth.providers", nil)
	if state == nil || len(state.Providers) < 3 {
		t.Fatal("provider registry missing")
	}
	state = call("auth.login", &review.AuthRequest{RuntimeID: state.RuntimeID, Provider: "openai", AuthType: "api_key"})
	deadline := time.Now().Add(10 * time.Second)
	for state.Operation == nil || state.Operation.Prompt == nil {
		if time.Now().After(deadline) {
			t.Fatal("login prompt missing")
		}
		time.Sleep(30 * time.Millisecond)
		state = call("auth.status", nil)
	}
	op := state.Operation
	a := &review.AuthRequest{RuntimeID: state.RuntimeID, OperationID: op.ID, PromptID: op.Prompt.ID, Value: "AUTARCH-SYNTHETIC-KEY"}
	state = call("auth.respond", a)
	for state.Operation == nil || state.Operation.Status == "pending" {
		if time.Now().After(deadline) {
			t.Fatal("login did not settle")
		}
		time.Sleep(30 * time.Millisecond)
		state = call("auth.status", nil)
	}
	if state.Operation.Status != "connected" {
		t.Fatalf("connection failed: %s", state.Operation.ErrorCode)
	}
	if r := e.Auth(review.Request{Project: root, Method: "auth.respond", Auth: a}); r.Error == "" {
		t.Fatal("stale response accepted")
	}
	call("auth.logout", &review.AuthRequest{RuntimeID: state.RuntimeID, Provider: "openai"})
	data, _ := json.Marshal(store.Snapshot())
	if strings.Contains(string(data), "AUTARCH-SYNTHETIC-KEY") {
		t.Fatal("secret entered review records")
	}
	_ = filepath.WalkDir(store.Dir(), func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr == nil && strings.Contains(string(data), "AUTARCH-SYNTHETIC-KEY") {
				t.Error("secret entered runtime or evidence file")
			}
		}
		return err
	})
	t.Log("Real Flere RPC registry, masked-key prompt, shared-store login/logout and stale-response rejection passed with isolated synthetic credentials")
}
