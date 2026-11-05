//go:build unix && homerunlive

package homerun

// Live tests for the as-mk launcher. They drive the REAL CLISystemd, ExecRunner, FSCgroup and RootLogs
// against the system manager of the machine they run on, so they must run as root on a throwaway Linux
// machine (a VM) with systemd as PID 1 and a service account. They are never part of the normal suite:
//
//	go test -c -tags homerunlive -o live.test ./internal/homerun
//
// Setup the machine must provide: an account named runner, a group named hrlog, and (for the sudo and
// docker checks) a NOPASSWD sudo rule for runner. Every observation is written to $HR_OUT/obs.txt as
// "OBS <test> <key> = <value>" so a reviewer can read what the machine did, not only pass or fail.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	liveUser  = "runner"
	liveGroup = "hrlog"
	liveBase  = "/srv/hr"
)

var (
	obsMu  sync.Mutex
	liveN  int
	liveCg = FSCgroup{}
)

func obs(t *testing.T, key string, val any) {
	t.Helper()
	line := fmt.Sprintf("OBS %s %s = %v", t.Name(), key, val)
	t.Log(line)
	if out := os.Getenv("HR_OUT"); out != "" {
		obsMu.Lock()
		defer obsMu.Unlock()
		if f, err := os.OpenFile(filepath.Join(out, "obs.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintln(f, line)
			f.Close()
		}
	}
}

type live struct {
	t     *testing.T
	cfg   LaunchConfig
	lim   Limits
	store *Store
	sys   CLISystemd
	out   FileOutput
	cas   *CAS
	l     *Launcher
	uid   uint32
	gid   uint32
}

func newLive(t *testing.T) *live {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("live tests must run as root on a throwaway machine")
	}
	u, err := user.Lookup(liveUser)
	if err != nil {
		t.Fatalf("service account %q: %v", liveUser, err)
	}
	uid, _ := strconv.ParseUint(u.Uid, 10, 32)
	gid, _ := strconv.ParseUint(u.Gid, 10, 32)
	cfg := LaunchConfig{User: liveUser, Bash: "/bin/bash", CASRoot: liveBase + "/cas", LogDir: liveBase + "/log", WritesRoot: liveBase + "/w"}
	for _, d := range []string{liveBase, cfg.CASRoot, cfg.LogDir, cfg.WritesRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	lim := DefaultLimits()
	lim.MaxExecutionsPerDay, lim.MaxActiveAsMk, lim.ApplyInterval = 10000, 1000, time.Second
	store, err := Open(filepath.Join(t.TempDir(), "state.db"), lim)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	sys := CLISystemd{Runner: ExecRunner{}}
	lv := &live{t: t, cfg: cfg, lim: lim, store: store, sys: sys, out: FileOutput{Config: cfg}, uid: uint32(uid), gid: uint32(gid),
		cas: &CAS{Root: cfg.CASRoot, OwnerUID: 0}}
	lv.l = &Launcher{Config: cfg, Limits: lim, Ledger: store, System: sys, Logs: RootLogs{Group: liveGroup}}
	return lv
}

// consume issues a nonce and consumes a check envelope for script (stored in the CAS) with the given runtime.
func (lv *live) consume(script string, runtime time.Duration, flags []string) (*Attempt, *Envelope) {
	lv.t.Helper()
	liveN++
	script = fmt.Sprintf("%s\n# live %s %d %d\n", script, lv.t.Name(), liveN, time.Now().UnixNano())
	sha, err := lv.cas.Put([]byte(script), "")
	if err != nil {
		lv.t.Fatal(err)
	}
	now := time.Now()
	n, err := lv.store.IssueNonce(bg, testHost, 4*time.Minute, now)
	if err != nil {
		lv.t.Fatal(err)
	}
	e := testEnvelope(lv.t, now)
	e.CardID, e.Nonce, e.ScriptSHA256, e.RuntimeMaxS = fmt.Sprintf("LIVE-%d", liveN), n.Value, sha, int64(runtime/time.Second)
	if flags != nil {
		e.Flags = flags
	}
	if err := e.Validate(lv.lim); err != nil {
		lv.t.Fatal(err)
	}
	dg, _ := e.Digest()
	a, err := lv.store.Consume(bg, ConsumeRequest{Env: e, EnvelopeDigest: dg,
		Signer: Signer{Principal: principal, KeyType: "ED25519-CERT", Fingerprint: "SHA256:test"}, Host: testHost, Now: now})
	if err != nil {
		lv.t.Fatal(err)
	}
	return a, e
}

func (lv *live) launch(script string, runtime time.Duration, writes ...string) (*Attempt, StartInfo, error) {
	lv.t.Helper()
	a, e := lv.consume(script, runtime, nil)
	info, err := lv.l.Launch(bg, a, e, writes)
	if err == nil {
		lv.t.Cleanup(func() { // never leave a unit behind
			_, _ = (ExecRunner{}).Run(bg, "systemctl", "stop", "--", info.Unit)
			_, _ = (ExecRunner{}).Run(bg, "systemctl", "reset-failed", "--", info.Unit)
		})
	}
	return a, info, err
}

func (lv *live) show(unit string) UnitProps {
	lv.t.Helper()
	p, err := lv.sys.Show(bg, unit)
	if err != nil {
		lv.t.Fatal(err)
	}
	return p
}

// sctl returns "systemctl show -p <props> unit" output, one KEY=VALUE per line.
func sctl(t *testing.T, unit string, props ...string) string {
	t.Helper()
	out, err := (ExecRunner{}).Run(bg, "systemctl", "show", "--property="+strings.Join(props, ","), "--", unit)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// recoverOnce runs the production recovery over the store and returns the last record of the attempt.
func (lv *live) recoverOnce(a *Attempt) Record {
	lv.t.Helper()
	if _, err := lv.store.RecoverUnits(bg, nil, UnitLookupFor(lv.sys, lv.out, liveCg), time.Now()); err != nil {
		lv.t.Fatalf("RecoverUnits: %v", err)
	}
	recs, err := lv.store.Records(bg)
	if err != nil {
		lv.t.Fatal(err)
	}
	var last Record
	for _, r := range recs {
		if r.Attempt == a.ID {
			last = r
		}
	}
	return last
}

// waitEnded polls recovery until the attempt is no longer STARTED, or the deadline passes.
func (lv *live) waitEnded(a *Attempt, within time.Duration) (Record, time.Duration) {
	lv.t.Helper()
	start := time.Now()
	for {
		r := lv.recoverOnce(a)
		if r.Kind != KindStarted {
			return r, time.Since(start)
		}
		if time.Since(start) > within {
			return r, time.Since(start)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// waitProps polls Show until ok(p) or the deadline; it returns the last answer.
func (lv *live) waitProps(unit string, within time.Duration, ok func(UnitProps) bool) (UnitProps, bool) {
	lv.t.Helper()
	end := time.Now().Add(within)
	for {
		p := lv.show(unit)
		if ok(p) {
			return p, true
		}
		if time.Now().After(end) {
			return p, false
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func readLog(t *testing.T, lv *live, a *Attempt) string {
	t.Helper()
	b, err := os.ReadFile(lv.cfg.LogPath(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exitStr(p *int) string {
	if p == nil {
		return "nil"
	}
	return strconv.Itoa(*p)
}

func cgDir(cg string) string { return filepath.Join("/sys/fs/cgroup", cg) }

func asRunner(t *testing.T, lv *live, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = []string{"PATH=" + UnitPATH}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: lv.uid, Gid: lv.gid}}
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	err := cmd.Run()
	return strings.TrimSpace(b.String()), err
}

// ---- environment facts -------------------------------------------------------------------------------

func TestLiveEnvironment(t *testing.T) {
	lv := newLive(t)
	for _, c := range [][]string{{"systemctl", "--version"}, {"stat", "-fc", "%T", "/sys/fs/cgroup"}, {"cat", "/proc/sys/kernel/yama/ptrace_scope"}, {"id", liveUser}} {
		out, err := (ExecRunner{}).Run(bg, c[0], c[1:]...)
		obs(t, strings.Join(c, " "), strings.ReplaceAll(strings.TrimSpace(string(out)), "\n", " | ")+fmt.Sprint(" err=", err))
	}
	_ = lv
}

// ---- A1/A2/A7: clean exit, pruned cgroup, ENDED --------------------------------------------------------

func TestLiveCleanExitEnds(t *testing.T) {
	lv := newLive(t)
	a, info, err := lv.launch("echo hello-from-unit; id -un; exit 0", 60*time.Second)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	obs(t, "invocation_id", info.InvocationID)
	// Wait for the main process to be done: the unit settles to active/exited (RemainAfterExit).
	p, ok := lv.waitProps(info.Unit, 30*time.Second, func(p UnitProps) bool { c, _ := p.settled(); return c })
	obs(t, "settled", fmt.Sprintf("%v active=%s sub=%s result=%s status=%d", ok, p.ActiveState, p.SubState, p.Result, p.ExitStatus))
	obs(t, "ControlGroup", fmt.Sprintf("%q", p.ControlGroup))
	obs(t, "TasksCurrent", fmt.Sprintf("known=%v n=%d raw=%q", p.TasksKnown, p.Tasks, sctl(t, info.Unit, "TasksCurrent")))
	if p.ControlGroup != "" {
		_, serr := os.Lstat(cgDir(p.ControlGroup))
		obs(t, "cgroup dir exists after clean exit", serr == nil)
		if serr == nil {
			b, _ := os.ReadFile(filepath.Join(cgDir(p.ControlGroup), "cgroup.procs"))
			obs(t, "cgroup.procs after clean exit", fmt.Sprintf("%q", string(b)))
		}
	}
	if !ok || p.ActiveState != "active" || p.SubState != "exited" {
		t.Errorf("a clean exit with RemainAfterExit should be active/exited, got %s/%s", p.ActiveState, p.SubState)
	}
	r, took := lv.waitEnded(a, 30*time.Second)
	obs(t, "recovery after clean exit", fmt.Sprintf("kind=%s result=%s status=%s took=%v", r.Kind, r.Result, exitStr(r.ExitStatus), took))
	if r.Kind != KindEnded || r.Result != ResultSuccess {
		t.Fatalf("clean exit: last record %s result %q note %q, want ENDED success", r.Kind, r.Result, r.Note)
	}
	log := readLog(t, lv, a)
	if !strings.Contains(log, "hello-from-unit") || !strings.Contains(log, liveUser) {
		t.Errorf("log %q misses the script output or the user", log)
	}
}

// ---- A3: background child survives the main process; not ended; killed at RuntimeMaxSec ----------------

func TestLiveBackgroundChildNotEndedUntilRuntimeMax(t *testing.T) {
	lv := newLive(t)
	a, info, err := lv.launch("sleep 300 </dev/null >/dev/null 2>&1 &\necho parent-done\nexit 0", 15*time.Second)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	started := time.Now()
	// The parent exits at once. For the next 6 s the unit must not be ENDED.
	time.Sleep(6 * time.Second)
	p := lv.show(info.Unit)
	obs(t, "6s after parent exit", fmt.Sprintf("active=%s sub=%s result=%s tasks(known=%v)=%d cg=%q", p.ActiveState, p.SubState, p.Result, p.TasksKnown, p.Tasks, p.ControlGroup))
	if p.ControlGroup != "" {
		b, _ := os.ReadFile(filepath.Join(cgDir(p.ControlGroup), "cgroup.procs"))
		obs(t, "cgroup.procs while child runs", strings.ReplaceAll(strings.TrimSpace(string(b)), "\n", ","))
	}
	r := lv.recoverOnce(a)
	obs(t, "recovery while child runs", fmt.Sprintf("kind=%s note=%q", r.Kind, r.Note))
	if r.Kind != KindStarted {
		t.Fatalf("a unit whose background child still runs was recorded as %s (%q)", r.Kind, r.Note)
	}
	// RuntimeMaxSec=15 kills the whole cgroup.
	r, _ = lv.waitEnded(a, 60*time.Second)
	took := time.Since(started)
	p = lv.show(info.Unit)
	obs(t, "after RuntimeMaxSec", fmt.Sprintf("kind=%s result=%s status=%s active=%s sub=%s sysResult=%s elapsed=%v", r.Kind, r.Result, exitStr(r.ExitStatus), p.ActiveState, p.SubState, p.Result, took.Round(time.Second)))
	if r.Kind != KindEnded || r.Result != ResultTimeout {
		t.Fatalf("after RuntimeMaxSec: %s result %q note %q, want ENDED timeout", r.Kind, r.Result, r.Note)
	}
	if took < 14*time.Second {
		t.Errorf("ended after %v, before the 15s runtime limit", took)
	}
	out, _ := (ExecRunner{}).Run(bg, "pgrep", "-f", "sleep 300")
	if len(bytes.TrimSpace(out)) > 0 {
		t.Errorf("the background child survived the timeout: %s", out)
	}
}

// ---- A7: failed and timed-out main process -------------------------------------------------------------

func TestLiveFailedExit(t *testing.T) {
	lv := newLive(t)
	a, _, err := lv.launch("echo before-fail; exit 3", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := lv.waitEnded(a, 30*time.Second)
	obs(t, "failed exit", fmt.Sprintf("kind=%s result=%s status=%s", r.Kind, r.Result, exitStr(r.ExitStatus)))
	if r.Kind != KindEnded || r.Result != ResultFailed || r.ExitStatus == nil || *r.ExitStatus != 3 {
		t.Fatalf("exit 3: %+v", r)
	}
}

func TestLiveMainProcessTimeout(t *testing.T) {
	lv := newLive(t)
	a, info, err := lv.launch("sleep 300", 8*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := lv.waitEnded(a, 60*time.Second)
	p := lv.show(info.Unit)
	obs(t, "timeout timestamps", fmt.Sprintf("exited=%v started=%v now=%v", p.ExitedAt.Unix(), p.StartedAt.Unix(), time.Now().Unix()))
	obs(t, "timeout", fmt.Sprintf("kind=%s result=%s status=%s active=%s sub=%s sysResult=%s", r.Kind, r.Result, exitStr(r.ExitStatus), p.ActiveState, p.SubState, p.Result))
	if r.Kind != KindEnded || r.Result != ResultTimeout {
		t.Fatalf("timeout: %+v", r)
	}
}

func TestLiveKilledBySignal(t *testing.T) {
	lv := newLive(t)
	a, info, err := lv.launch("kill -9 $$", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := lv.waitEnded(a, 30*time.Second)
	p := lv.show(info.Unit)
	obs(t, "signal", fmt.Sprintf("kind=%s result=%s status=%s active=%s sub=%s sysResult=%s", r.Kind, r.Result, exitStr(r.ExitStatus), p.ActiveState, p.SubState, p.Result))
	if r.Kind != KindEnded || r.Result != ResultSignal {
		t.Fatalf("signal: %+v", r)
	}
}

// ---- A6: log file owner, mode, no truncation, UMask ----------------------------------------------------

func TestLiveLogOwnerModeAndNoTruncation(t *testing.T) {
	lv := newLive(t)
	// Put pre-existing bytes in the precreated log, as a hostile or earlier writer might: systemd must append.
	lv.l.Logs = prefilledLogs{RootLogs{Group: liveGroup}, "PRE-EXISTING\n"}
	a, _, err := lv.launch("echo from-script; umask; touch /tmp/umask-probe; stat -c 'tmpfile=%a' /tmp/umask-probe", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := lv.waitEnded(a, 30*time.Second)
	if r.Kind != KindEnded {
		t.Fatalf("record %+v", r)
	}
	fi, err := os.Stat(lv.cfg.LogPath(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	g, _ := user.LookupGroup(liveGroup)
	obs(t, "log owner:group mode", fmt.Sprintf("uid=%d gid=%d (hrlog gid=%s) mode=%04o", st.Uid, st.Gid, g.Gid, fi.Mode().Perm()))
	log := readLog(t, lv, a)
	obs(t, "log content", strings.ReplaceAll(log, "\n", "\\n"))
	if st.Uid != 0 || strconv.Itoa(int(st.Gid)) != g.Gid || fi.Mode().Perm() != 0o640 {
		t.Errorf("log is uid %d gid %d mode %04o, want root:%s 0640", st.Uid, st.Gid, fi.Mode().Perm(), liveGroup)
	}
	// Observed, not asserted as a requirement: StandardOutput=file: truncates the precreated file.
	obs(t, "precreated content survived", strings.HasPrefix(log, "PRE-EXISTING"))
	if !strings.Contains(log, "from-script") {
		t.Errorf("log %q: the script output did not reach the precreated root-owned file", log)
	}
	if !strings.Contains(log, "0027") || !strings.Contains(log, "tmpfile=640") {
		t.Errorf("UMask=0027 not in effect inside the unit: %q", log)
	}
}

type prefilledLogs struct {
	RootLogs
	content string
}

func (p prefilledLogs) Create(path string) error {
	if err := p.RootLogs.Create(path); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(p.content), 0o640)
}

// If the log is not precreated, what does systemd make of it? (Why the launcher precreates it.)
func TestLiveLogCreatedBySystemdIfAbsent(t *testing.T) {
	lv := newLive(t)
	lv.l.Logs = noopLogs{}
	a, _, err := lv.launch("echo from-script", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	lv.waitEnded(a, 30*time.Second)
	fi, err := os.Stat(lv.cfg.LogPath(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	obs(t, "log made by systemd", fmt.Sprintf("uid=%d gid=%d mode=%04o", st.Uid, st.Gid, fi.Mode().Perm()))
}

type noopLogs struct{}

func (noopLogs) Create(string) error { return nil }
func (noopLogs) Remove(string) error { return nil }

// ---- hardening actually applied -------------------------------------------------------------------------

func TestLiveHardeningApplied(t *testing.T) {
	lv := newLive(t)
	// A docker socket reachable by every account, to prove the unit (not the file mode) is what blocks it.
	_ = os.Remove("/run/docker.sock")
	ln, err := net.Listen("unix", "/run/docker.sock")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove("/run/docker.sock")
	defer ln.Close()
	_ = os.Chmod("/run/docker.sock", 0o666)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	w := liveBase + "/w/hard"
	if err := os.MkdirAll(w, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chown(w, int(lv.uid), int(lv.gid))
	script := `
probe() { local n="$1"; shift; if "$@" >/dev/null 2>&1; then echo "PROBE $n=allowed"; else echo "PROBE $n=denied"; fi; }
probe sudo sudo -n true
probe docker_sock python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect("/run/docker.sock")'
probe write_etc touch /etc/hr-probe
probe write_usr touch /usr/hr-probe
probe write_declared touch ` + w + `/ok
probe write_other_home touch /home/runner/hr-probe
probe exec_from_tmp sh -c 'cp /usr/bin/true /tmp/t && /tmp/t'
probe exec_from_declared_write sh -c 'cp /usr/bin/true ` + w + `/t && ` + w + `/t'
probe setuid_su su -c true root
grep -E '^(Cap(Inh|Prm|Eff|Bnd|Amb)|NoNewPrivs|Seccomp):' /proc/self/status | tr '\t' ' ' | sed 's/^/STATUS /'
echo "PROBE tmp_private=$(ls /tmp | wc -l)"
echo "PROBE uid=$(id -u) euid=$(id -u)"
echo "PROBE home_runner_umask=$(umask)"
`
	a, info, err := lv.launch(script, 60*time.Second, w)
	if err != nil {
		t.Fatal(err)
	}
	// While the unit exists, read the properties systemd applied.
	props := sctl(t, info.Unit, "User", "NoNewPrivileges", "RestrictSUIDSGID", "CapabilityBoundingSet", "InaccessiblePaths", "PrivateTmp",
		"ProtectSystem", "NoExecPaths", "ExecPaths", "ReadWritePaths", "RuntimeMaxUSec", "KillMode", "TasksAccounting", "ExitType",
		"RemainAfterExit", "UMask", "ProtectHome", "StandardOutput", "StandardError", "Type", "ExecMainStatus")
	obs(t, "systemctl show", strings.ReplaceAll(props, "\n", " ; "))
	want := map[string]string{"User": liveUser, "NoNewPrivileges": "yes", "RestrictSUIDSGID": "yes", "PrivateTmp": "yes", "ProtectSystem": "strict", "ProtectHome": "read-only",
		"KillMode": "control-group", "TasksAccounting": "yes", "ExitType": "cgroup", "RemainAfterExit": "yes", "UMask": "0027", "RuntimeMaxUSec": "1min", "Type": "exec"}
	got := map[string]string{}
	for _, l := range strings.Split(props, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			got[k] = v
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if got["CapabilityBoundingSet"] != "" {
		t.Errorf("CapabilityBoundingSet = %q, want empty", got["CapabilityBoundingSet"])
	}
	if got["StandardOutput"] != "file" {
		t.Errorf("StandardOutput = %q", got["StandardOutput"])
	}
	r, _ := lv.waitEnded(a, 60*time.Second)
	log := readLog(t, lv, a)
	obs(t, "probe log", strings.ReplaceAll(log, "\n", " | "))
	if r.Kind != KindEnded || r.Result != ResultSuccess {
		t.Fatalf("hardening probe run: %+v", r)
	}
	for _, n := range []string{"sudo", "docker_sock", "write_etc", "write_usr", "write_other_home", "exec_from_tmp", "exec_from_declared_write", "setuid_su"} {
		if !strings.Contains(log, "PROBE "+n+"=denied") {
			t.Errorf("%s was not denied inside the unit", n)
		}
	}
	if !strings.Contains(log, "PROBE write_declared=allowed") {
		t.Errorf("a path in ReadWritePaths was not writable")
	}
	// The same probes outside any unit: the controls must be the unit's doing.
	if out, err := asRunner(t, lv, "sudo", "-n", "true"); err != nil {
		t.Errorf("control: sudo -n true as %s outside a unit failed (%v %s): the sudo probe proves nothing", liveUser, err, out)
	}
	if out, err := asRunner(t, lv, "python3", "-c", `import socket;s=socket.socket(socket.AF_UNIX);s.connect("/run/docker.sock")`); err != nil {
		t.Errorf("control: docker socket connect outside a unit failed (%v %s)", err, out)
	}
	// systemd's own exposure score for the unit, kept as evidence.
	sec, _ := (ExecRunner{}).Run(bg, "systemd-analyze", "security", "--no-pager", info.Unit)
	if out := os.Getenv("HR_OUT"); out != "" {
		_ = os.WriteFile(filepath.Join(out, "systemd-analyze-security.txt"), sec, 0o644)
	}
	lines := strings.Split(strings.TrimSpace(string(sec)), "\n")
	obs(t, "systemd-analyze security (last line)", lines[len(lines)-1])
}

// ---- A5: ReadWritePaths, symlinks and the swap window --------------------------------------------------

func TestLiveWritesResolveSymlinkedRoot(t *testing.T) {
	lv := newLive(t)
	real := liveBase + "/wreal"
	link := liveBase + "/wlink"
	_ = os.RemoveAll(real)
	_ = os.Remove(link)
	if err := os.MkdirAll(real+"/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chown(real+"/sub", int(lv.uid), int(lv.gid))
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	lv.cfg.WritesRoot = link
	lv.l.Config = lv.cfg
	a, info, err := lv.launch("touch "+real+"/sub/via-real && echo wrote-real", 60*time.Second, link+"/sub")
	if err != nil {
		t.Fatalf("Launch through a symlinked WritesRoot: %v", err)
	}
	rw := sctl(t, info.Unit, "ReadWritePaths")
	obs(t, "ReadWritePaths given to systemd", rw)
	r, _ := lv.waitEnded(a, 30*time.Second)
	log := readLog(t, lv, a)
	obs(t, "result", fmt.Sprintf("%s %s log=%q", r.Kind, r.Result, log))
	if r.Result != ResultSuccess || !strings.Contains(log, "wrote-real") || !strings.Contains(rw, real+"/sub") || strings.Contains(rw, link) {
		t.Errorf("symlinked root: result %q, log %q, ReadWritePaths %q", r.Result, log, rw)
	}
	// A symlink component below the root is refused before any intent.
	_ = os.Symlink("/etc", real+"/evil")
	if _, _, err := lv.launch("true", 10*time.Second, link+"/evil"); !errors.Is(err, ErrLaunchRefused) {
		t.Errorf("symlink component: %v, want ErrLaunchRefused", err)
	}
}

func TestLiveTrustedChainOnRealTree(t *testing.T) {
	lv := newLive(t)
	good := liveBase + "/w"
	obs(t, "TrustedRoot(root-owned 0755 chain)", TrustedRoot(lv.uid)(good))
	if err := TrustedRoot(lv.uid)(good); err != nil {
		t.Errorf("the production chain was refused: %v", err)
	}
	bad := liveBase + "/w/gw"
	_ = os.MkdirAll(bad, 0o775)
	_ = os.Chmod(bad, 0o775)
	err := TrustedRoot(lv.uid)(bad)
	obs(t, "TrustedRoot(group-writable)", err)
	if err == nil {
		t.Errorf("a group-writable root was accepted")
	}
	// The runner-owned tree: what production /home/<user>/... looks like.
	own := liveBase + "/w/owned"
	_ = os.MkdirAll(own, 0o755)
	_ = os.Chown(own, int(lv.uid), int(lv.gid))
	err = TrustedRoot(lv.uid)(own)
	obs(t, "TrustedRoot(service-user-owned 0755 root)", err)
	// Can the service user (who is also the agent's identity in production) swap it for a symlink?
	out, serr := asRunner(t, lv, "sh", "-c", "mv "+own+" "+own+".moved && ln -s /etc "+own)
	obs(t, "service user renames its own trusted root", fmt.Sprintf("err=%v out=%q", serr, out))
	_ = os.RemoveAll(own + ".moved")
	_ = os.Remove(own)
	// The root-owned parent of a root-owned child cannot be renamed by the service user.
	out, serr = asRunner(t, lv, "sh", "-c", "mv "+liveBase+"/w "+liveBase+"/w.moved")
	obs(t, "service user renames the root-owned writes root", fmt.Sprintf("err=%v out=%q", serr, out))
	if serr == nil {
		t.Errorf("service user could rename a root-owned directory")
		_ = os.Rename(liveBase+"/w.moved", liveBase+"/w")
	}
}

// swapSystemd swaps the writable path for a symlink after the launcher's last check and before the real
// systemd-run, which is the narrowest swap the launcher cannot see.
type swapSystemd struct {
	Systemd
	swap func()
}

func (s swapSystemd) Start(ctx context.Context, args []string) error {
	s.swap()
	return s.Systemd.Start(ctx, args)
}

func TestLiveSwapWindow(t *testing.T) {
	// Two targets for the agent-made symlink. A root-owned protected directory must stay unwritable (the
	// service user has no permission there). A service-user-owned one is the residual window: ReadWritePaths
	// follows the symlink, so the unit's own read-only mounts no longer protect that directory; only file
	// permissions do. That second case is recorded, not asserted.
	for _, svcOwned := range []bool{false, true} {
		name := "root-owned-target"
		if svcOwned {
			name = "service-owned-target"
		}
		t.Run(name, func(t *testing.T) {
			lv := newLive(t)
			base := liveBase + "/w/swapdir-" + name
			good := base + "/work"
			prot := liveBase + "/protected-" + name
			_ = os.RemoveAll(base)
			_ = os.RemoveAll(prot)
			for _, d := range []string{good, prot} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			// base is owned by the service user: the production shape (/home/<user> belongs to the user the agent runs as).
			_ = os.Chown(base, int(lv.uid), int(lv.gid))
			_ = os.Chown(good, int(lv.uid), int(lv.gid))
			if svcOwned {
				_ = os.Chown(prot, int(lv.uid), int(lv.gid))
			}
			script := "touch " + good + "/pwned && echo wrote-into-good; ls -ld " + good + " >&2"
			// The swap happens after the launcher's identity check (inside Start). Nothing sees it.
			lv.l.System = swapSystemd{lv.sys, func() {
				out, err := asRunner(t, lv, "sh", "-c", "mv "+good+" "+good+".real && ln -s "+prot+" "+good)
				obs(t, name+" agent swaps the writable dir for a symlink", fmt.Sprintf("err=%v out=%q", err, out))
			}}
			a, info, err := lv.launch(script, 60*time.Second, good)
			obs(t, name+" Launch result after the swap", err)
			if err != nil {
				t.Logf("launch refused: %v", err)
				return
			}
			rw := sctl(t, info.Unit, "ReadWritePaths")
			lv.waitEnded(a, 30*time.Second)
			_, perr := os.Lstat(prot + "/pwned")
			obs(t, name+" unit given", rw)
			obs(t, name+" file appeared in the protected directory", perr == nil)
			obs(t, name+" log", strings.ReplaceAll(readLog(t, lv, a), "\n", " | "))
			if perr == nil && !svcOwned {
				t.Errorf("SWAP WINDOW OPEN: the unit followed an agent-made symlink and wrote into root-owned %s", prot)
			}
		})
	}
}

// ---- A4 (cgroup path) and cgroup removal after the unit is stopped ---------------------------------------

func TestLiveCgroupPathAndRemoval(t *testing.T) {
	lv := newLive(t)
	a, info, err := lv.launch("sleep 20", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := lv.waitProps(info.Unit, 10*time.Second, func(p UnitProps) bool { return p.ControlGroup != "" })
	dir := cgDir(p.ControlGroup)
	_, serr := os.Lstat(dir)
	pop, perr := liveCg.Populated(p.ControlGroup)
	obs(t, "while running", fmt.Sprintf("ControlGroup=%q dir exists=%v Populated=%v err=%v tasks=%d known=%v", p.ControlGroup, serr == nil, pop, perr, p.Tasks, p.TasksKnown))
	if serr != nil || !pop || perr != nil {
		t.Errorf("cgroup %q: exists=%v populated=%v err=%v while the unit runs", dir, serr == nil, pop, perr)
	}
	if out, err := (ExecRunner{}).Run(bg, "systemctl", "stop", "--", info.Unit); err != nil {
		t.Fatalf("stop: %v %s", err, out)
	}
	p2 := lv.show(info.Unit)
	_, serr2 := os.Lstat(dir)
	pop2, perr2 := liveCg.Populated(p.ControlGroup)
	obs(t, "after systemctl stop", fmt.Sprintf("load=%s active=%s sub=%s result=%s cg=%q dir exists=%v Populated=%v err=%v", p2.LoadState, p2.ActiveState, p2.SubState, p2.Result, p2.ControlGroup, serr2 == nil, pop2, perr2))
	_ = a
	if pop2 || perr2 != nil {
		t.Errorf("after stop: populated=%v err=%v", pop2, perr2)
	}
}

// ---- A7: the unit stays readable after exit (RemainAfterExit) and is gone only when reset ---------------

func TestLiveUnitStaysAfterExit(t *testing.T) {
	lv := newLive(t)
	a, info, err := lv.launch("exit 0", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	lv.waitEnded(a, 30*time.Second)
	time.Sleep(10 * time.Second)
	p := lv.show(info.Unit)
	obs(t, "10s after end", fmt.Sprintf("load=%s active=%s sub=%s", p.LoadState, p.ActiveState, p.SubState))
	if p.LoadState != "loaded" || p.ActiveState != "active" || p.SubState != "exited" {
		t.Errorf("unit did not stay readable: %+v", p)
	}
	// A second recovery pass must not change the ENDED record's meaning.
	r := lv.recoverOnce(a)
	if r.Kind != KindEnded {
		t.Errorf("after a second pass: %s", r.Kind)
	}
	if out, err := (ExecRunner{}).Run(bg, "systemctl", "stop", "--", info.Unit); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	p = lv.show(info.Unit)
	obs(t, "after stop", fmt.Sprintf("load=%s active=%s", p.LoadState, p.ActiveState))
}

var _ = fs.ModeDir

// TestLiveDiagRunArgs prints the exact systemd-run command line and the unfiltered systemd-run output, so a
// refusal that ExecRunner's trimmed stderr hides can be read.
func TestLiveDiagRunArgs(t *testing.T) {
	lv := newLive(t)
	a, e := lv.consume("exit 0", 30*time.Second, nil)
	args, err := BuildRunArgs(lv.cfg, lv.lim, a, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	obs(t, "args", strings.Join(args, " \\\n   "))
	var clean []string
	for _, x := range args {
		if x != "--quiet" {
			clean = append(clean, x)
		}
	}
	cmd := exec.Command("systemd-run", clean...)
	out, err := cmd.CombinedOutput()
	obs(t, "systemd-run without --quiet", fmt.Sprintf("err=%v out=%q", err, out))
	jo, _ := exec.Command("journalctl", "--no-pager", "-o", "cat", "-u", UnitName(a.ID)).CombinedOutput()
	obs(t, "journal", strings.ReplaceAll(string(jo), "\n", " | "))
	st, _ := exec.Command("systemctl", "status", "--no-pager", "-l", UnitName(a.ID)).CombinedOutput()
	obs(t, "status", strings.ReplaceAll(string(st), "\n", " | "))
	ls, _ := exec.Command("ls", "-ld", "/srv/hr", "/srv/hr/cas", args[len(args)-1]).CombinedOutput()
	obs(t, "ls", strings.ReplaceAll(string(ls), "\n", " | "))
	_, _ = (ExecRunner{}).Run(bg, "systemctl", "stop", "--", UnitName(a.ID))
}

// TestLiveDiagSandbox compares sandbox property sets. The probe writes to directories the service user owns
// (so file mode cannot be the reason for a denial) and runs a binary copied into /tmp, the home directory and a
// ReadWritePaths directory. Wanted: home write denied, rw-path write allowed, no exec from tmp/home/rw-path.
func TestLiveDiagSandbox(t *testing.T) {
	lv := newLive(t)
	rw := liveBase + "/w/hard"
	if err := os.MkdirAll(rw, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chown(rw, int(lv.uid), int(lv.gid))
	a, e := lv.consume("exit 0", 30*time.Second, nil)
	args, err := BuildRunArgs(lv.cfg, lv.lim, a, e, []string{rw})
	if err != nil {
		t.Fatal(err)
	}
	var props []string
	for _, x := range args {
		if strings.HasPrefix(x, "--property=") && !strings.HasPrefix(x, "--property=StandardOutput=") && !strings.HasPrefix(x, "--property=StandardError=") &&
			!strings.HasPrefix(x, "--property=RuntimeMaxSec=") && !strings.HasPrefix(x, "--property=ExitType=") && !strings.HasPrefix(x, "--property=RemainAfterExit=") {
			props = append(props, strings.TrimPrefix(x, "--property="))
		}
	}
	probe := `h=/home/runner; r=` + rw + `
chk() { if "$@" >/dev/null 2>&1; then echo -n "allowed "; else echo -n "denied "; fi; }
echo -n "Whome="; chk touch $h/w
echo -n "Wrw="; chk touch $r/w
echo -n "Xtmp="; cp /usr/bin/true /tmp/t 2>/dev/null; chk /tmp/t
echo -n "Xhome="; cp /usr/bin/true $h/t 2>/dev/null; chk $h/t
echo -n "Xrw="; cp /usr/bin/true $r/t 2>/dev/null; chk $r/t
echo -n "Xusr="; chk /usr/bin/true
echo -n "Wtmp="; chk touch /tmp/w
echo
`
	run := func(label string, ps []string) {
		a := []string{"--wait", "--pipe", "--collect", "--quiet", "--no-ask-password"}
		for _, p := range ps {
			a = append(a, "--property="+p)
		}
		a = append(a, "--", "/bin/sh", "-c", probe)
		out, err := exec.Command("systemd-run", a...).CombinedOutput()
		_ = os.Remove(rw + "/w")
		_ = os.Remove(rw + "/t")
		_ = os.Remove("/home/" + liveUser + "/w")
		_ = os.Remove("/home/" + liveUser + "/t")
		obs(t, label, fmt.Sprintf("%s err=%v", strings.ReplaceAll(strings.TrimSpace(string(out)), "\n", " "), err))
	}
	run("production props (current)", props)
	without := func(prefix string, extra ...string) []string {
		var o []string
		for _, p := range props {
			if !strings.HasPrefix(p, prefix) {
				o = append(o, p)
			}
		}
		return append(o, extra...)
	}
	run("C1 ProtectHome=read-only + NoExecPaths=/ /tmp /var/tmp RW", without("NoExecPaths=", "ProtectHome=read-only", "NoExecPaths=/ /tmp /var/tmp "+rw))
	run("C2 as C1 but NoExecPaths also given as a second property line", without("NoExecPaths=", "ProtectHome=read-only", "NoExecPaths=/ /tmp /var/tmp", "NoExecPaths="+rw))
	// mountinfo seen by the unit, current props
	a2 := []string{"--wait", "--pipe", "--collect", "--quiet"}
	for _, p := range props {
		a2 = append(a2, "--property="+p)
	}
	a2 = append(a2, "--", "/bin/sh", "-c", "grep -vE ' /(sys|dev|proc|run)' /proc/self/mountinfo | cut -d' ' -f5-6; findmnt -n /home || echo home-not-a-mount")
	out, err := exec.Command("systemd-run", a2...).CombinedOutput()
	obs(t, "mountinfo in the unit", strings.ReplaceAll(string(out), "\n", " | ")+fmt.Sprint(err))
}
