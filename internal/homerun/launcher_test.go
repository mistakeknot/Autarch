//go:build unix

package homerun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeSystemd records every call and never starts anything.
type fakeSystemd struct {
	log       *[]string
	startErr  error
	showErr   error
	startArgs [][]string
	units     map[string]UnitProps // what Show answers, by unit
	onStart   func(args []string)
	invFor    string // InvocationID returned for a started unit
}

func (f *fakeSystemd) Start(_ context.Context, args []string) error {
	*f.log = append(*f.log, "systemd-run")
	f.startArgs = append(f.startArgs, args)
	if f.onStart != nil {
		f.onStart(args)
	}
	if f.startErr != nil {
		return f.startErr
	}
	unit := strings.TrimPrefix(args[0], "--unit=")
	if f.units == nil {
		f.units = map[string]UnitProps{}
	}
	f.units[unit] = UnitProps{LoadState: "loaded", ActiveState: "active", SubState: "running", InvocationID: f.invFor}
	return nil
}

func (f *fakeSystemd) Show(_ context.Context, unit string) (UnitProps, error) {
	*f.log = append(*f.log, "show")
	if f.showErr != nil {
		return UnitProps{}, f.showErr
	}
	if p, ok := f.units[unit]; ok {
		return p, nil
	}
	return UnitProps{LoadState: "not-found", ActiveState: "inactive", SubState: "dead"}, nil
}

// loggingLedger wraps the real Store, logs each step and can fail RecordStarted.
type loggingLedger struct {
	s          *Store
	log        *[]string
	startedErr error
}

func (l loggingLedger) RecordLaunchIntent(ctx context.Context, a string, at time.Time) (string, error) {
	*l.log = append(*l.log, "intent")
	return l.s.RecordLaunchIntent(ctx, a, at)
}

func (l loggingLedger) RecordStarted(ctx context.Context, a string, i StartInfo, at time.Time) error {
	*l.log = append(*l.log, "started")
	if l.startedErr != nil {
		return l.startedErr
	}
	return l.s.RecordStarted(ctx, a, i, at)
}

const (
	goodInv = "0123456789abcdef0123456789abcdef"
	testCAS = "/srv/cas"
)

func testCfg() LaunchConfig {
	return LaunchConfig{User: "runner", CASRoot: testCAS, LogDir: "/srv/log", WritesRoot: "/home/runner"}
}

type rig struct {
	s   *Store
	l   *Launcher
	sys *fakeSystemd
	led loggingLedger
	log *[]string
	att *Attempt
	env *Envelope
	now time.Time
}

func newRig(t *testing.T, op string) *rig {
	t.Helper()
	s, _ := newStore(t)
	now := time.Now()
	// An apply or recover needs a recorded check first (slice 2's gate, tested there); the launcher only reads
	// the op, so consume a check and relabel it.
	att, env, err := consumeFor(t, s, now, "AUT-1", OpCheck)
	if err != nil {
		t.Fatal(err)
	}
	att.Op, env.Op = op, op
	log := &[]string{}
	sys := &fakeSystemd{log: log, invFor: goodInv}
	led := loggingLedger{s: s, log: log}
	return &rig{s: s, sys: sys, led: led, log: log, att: att, env: env, now: now,
		l: &Launcher{Config: testCfg(), Limits: DefaultLimits(), Ledger: &led, System: sys, Now: func() time.Time { return now }}}
}

func TestBuildRunArgsGolden(t *testing.T) {
	r := newRig(t, OpCheck)
	r.env.Flags = []string{"--check", "--verbose"}
	got, err := BuildRunArgs(testCfg(), DefaultLimits(), r.att, r.env, []string{"/home/runner/work", "/home/runner/logs"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--unit=" + UnitName(r.att.ID),
		"--description=Citadel script run " + r.att.ID,
		"--quiet",
		"--no-ask-password",
		"--service-type=exec",
		"--property=User=runner",
		"--property=NoNewPrivileges=yes",
		"--property=RestrictSUIDSGID=yes",
		"--property=CapabilityBoundingSet=",
		"--property=InaccessiblePaths=/run/docker.sock /var/run/docker.sock /run/containerd",
		"--property=PrivateTmp=yes",
		"--property=ProtectSystem=strict",
		"--property=NoExecPaths=/",
		"--property=ExecPaths=/usr /bin /sbin /lib /lib64 /srv/cas",
		"--property=ReadWritePaths=/home/runner/work /home/runner/logs",
		"--property=RuntimeMaxSec=600",
		"--property=KillMode=control-group",
		"--property=TasksAccounting=yes",
		"--property=ExitType=cgroup",
		"--property=RemainAfterExit=yes",
		"--property=UMask=0027",
		"--property=StandardOutput=file:/srv/log/" + r.att.ID + ".log",
		"--property=StandardError=inherit",
		"--setenv=PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"--setenv=HOME_RUN_DEPS=/srv/cas",
		"--",
		"/bin/bash", "/srv/cas/" + r.att.ScriptSHA256, "--check", "--verbose",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argument list changed:\n got %q\nwant %q", got, want)
	}
}

func TestHardeningPropertiesAreAlwaysPresentAndNothingElseRunsTheScript(t *testing.T) {
	for _, op := range []string{OpCheck, OpApply, OpRecover} {
		r := newRig(t, op)
		args, err := BuildRunArgs(testCfg(), DefaultLimits(), r.att, r.env, nil)
		if err != nil {
			t.Fatal(err)
		}
		have := map[string]bool{}
		for _, a := range args {
			have[a] = true
		}
		for _, must := range []string{
			"--property=User=runner", "--property=NoNewPrivileges=yes", "--property=RestrictSUIDSGID=yes",
			"--property=CapabilityBoundingSet=", "--property=PrivateTmp=yes", "--property=ProtectSystem=strict",
			"--property=NoExecPaths=/", "--property=KillMode=control-group",
			"--property=InaccessiblePaths=/run/docker.sock /var/run/docker.sock /run/containerd",
		} {
			if !have[must] {
				t.Errorf("%s: missing %s", op, must)
			}
		}
		for _, a := range args {
			if strings.HasPrefix(a, "--property=ReadWritePaths") {
				t.Errorf("%s: no writes declared, yet %s", op, a)
			}
			if strings.Contains(a, "journal") || strings.HasPrefix(a, "--user") || a == "--scope" || a == "--pty" || a == "--pipe" {
				t.Errorf("%s: unexpected %s", op, a)
			}
		}
		// the logged output never goes to the journal: StandardOutput is a file, exactly once
		n := 0
		for _, a := range args {
			if strings.HasPrefix(a, "--property=StandardOutput=file:") {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s: %d StandardOutput properties", op, n)
		}
		// everything after "--" is the interpreter, the CAS copy, then the signed flags
		i := indexOf(args, "--")
		if i < 0 || !reflect.DeepEqual(args[i+1:], LaunchArgv("/bin/bash", "/srv/cas/"+r.att.ScriptSHA256, r.env)) {
			t.Errorf("%s: command tail %q", op, args[i+1:])
		}
	}
}

func indexOf(a []string, s string) int {
	for i, v := range a {
		if v == s {
			return i
		}
	}
	return -1
}

func TestRuntimeCapMapping(t *testing.T) {
	lim := DefaultLimits()
	for _, c := range []struct {
		op   string
		want string
	}{{OpCheck, "600"}, {OpApply, "10800"}, {OpRecover, "10800"}} {
		r := newRig(t, c.op)
		r.att.RuntimeMax = lim.RuntimeCap(c.op)
		args, err := BuildRunArgs(testCfg(), lim, r.att, r.env, nil)
		if err != nil {
			t.Fatalf("%s at the cap: %v", c.op, err)
		}
		if !contains(args, "--property=RuntimeMaxSec="+c.want) {
			t.Errorf("%s: cap %s not mapped: %q", c.op, c.want, args)
		}
		r.att.RuntimeMax = lim.RuntimeCap(c.op) + time.Second
		if _, err := BuildRunArgs(testCfg(), lim, r.att, r.env, nil); !errors.Is(err, ErrLaunchRefused) {
			t.Errorf("%s above the cap: %v", c.op, err)
		}
	}
	// the signed value, not the cap, is what is mapped
	r := newRig(t, OpApply)
	r.att.RuntimeMax = 90 * time.Second
	args, _ := BuildRunArgs(testCfg(), lim, r.att, r.env, nil)
	if !contains(args, "--property=RuntimeMaxSec=90") {
		t.Errorf("signed runtime not used: %q", args)
	}
	// a lowered policy cap applies to an attempt consumed under a higher one
	low := lim
	low.ApplyRuntimeMax = time.Minute
	if _, err := BuildRunArgs(testCfg(), low, r.att, r.env, nil); !errors.Is(err, ErrLaunchRefused) {
		t.Errorf("policy lowered but 90s still launched: %v", err)
	}
	for _, d := range []time.Duration{0, -time.Second, 1500 * time.Millisecond} {
		r.att.RuntimeMax = d
		if _, err := BuildRunArgs(testCfg(), lim, r.att, r.env, nil); !errors.Is(err, ErrLaunchRefused) {
			t.Errorf("runtime %v accepted", d)
		}
	}
}

func contains(a []string, s string) bool { return indexOf(a, s) >= 0 }

func TestRefusals(t *testing.T) {
	cases := map[string]func(r *rig, c *LaunchConfig, w *[]string){
		"root privilege":        func(r *rig, c *LaunchConfig, w *[]string) { r.att.Privilege = PrivRoot },
		"envelope root":         func(r *rig, c *LaunchConfig, w *[]string) { r.env.Privilege = PrivRoot },
		"op mismatch":           func(r *rig, c *LaunchConfig, w *[]string) { r.env.Op = OpApply },
		"script mismatch":       func(r *rig, c *LaunchConfig, w *[]string) { r.env.ScriptSHA256 = sha2Hex },
		"card mismatch":         func(r *rig, c *LaunchConfig, w *[]string) { r.env.CardID = "AUT-9" },
		"bad flag":              func(r *rig, c *LaunchConfig, w *[]string) { r.env.Flags = []string{"-x"} },
		"flag injecting a prop": func(r *rig, c *LaunchConfig, w *[]string) { r.env.Flags = []string{"--a b"} },
		"bad attempt id":        func(r *rig, c *LaunchConfig, w *[]string) { r.att.ID = "not-a-ulid" },
		"root user":             func(r *rig, c *LaunchConfig, w *[]string) { c.User = "root" },
		"empty user":            func(r *rig, c *LaunchConfig, w *[]string) { c.User = "" },
		"relative cas":          func(r *rig, c *LaunchConfig, w *[]string) { c.CASRoot = "cas" },
		"no log dir":            func(r *rig, c *LaunchConfig, w *[]string) { c.LogDir = "" },
		"no writes root":        func(r *rig, c *LaunchConfig, w *[]string) { c.WritesRoot = "" },
		"write outside root":    func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/etc"} },
		"write is the root":     func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/home/runner"} },
		"write sibling prefix":  func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/home/runner2/x"} },
		"write dotdot":          func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/home/runner/../etc"} },
		"write with space":      func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/home/runner/a /etc"} },
		"write with specifier":  func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/home/runner/%h"} },
		"write duplicate":       func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/home/runner/a", "/home/runner/a"} },
		"write with newline":    func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/home/runner/a\nExecStart=/x"} },
		"write trailing slash":  func(r *rig, c *LaunchConfig, w *[]string) { *w = []string{"/home/runner/a/"} },
		"nil attempt":           func(r *rig, c *LaunchConfig, w *[]string) { r.att = nil },
		"short script digest":   func(r *rig, c *LaunchConfig, w *[]string) { r.att.ScriptSHA256, r.env.ScriptSHA256 = "abc", "abc" },
		"unknown op":            func(r *rig, c *LaunchConfig, w *[]string) { r.att.Op, r.env.Op = "run", "run" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, OpCheck)
			cfg, writes := testCfg(), []string(nil)
			mut(r, &cfg, &writes)
			if _, err := BuildRunArgs(cfg, DefaultLimits(), r.att, r.env, writes); !errors.Is(err, ErrLaunchRefused) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestLaunchRecordsIntentBeforeSystemdAndStartedAfter(t *testing.T) {
	r := newRig(t, OpApply)
	ctx := context.Background()
	r.sys.onStart = func([]string) {
		// at the moment systemd-run is invoked the intent is already durable: a second handle on the
		// same database sees it, and ABORTED is already refused
		if err := r.s.RecordAborted(ctx, r.att.ID, "x", r.now); !errors.Is(err, ErrLaunchIntent) {
			t.Errorf("intent not durable when systemd-run ran: %v", err)
		}
	}
	info, err := r.l.Launch(ctx, r.att, r.env, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*r.log, ","); got != "intent,systemd-run,show,started" {
		t.Fatalf("order %s", got)
	}
	if info.Unit != UnitName(r.att.ID) || info.InvocationID != goodInv || info.CASSHA != r.att.ScriptSHA256 {
		t.Fatalf("info %+v", info)
	}
	if r.sys.startArgs[0][0] != "--unit="+UnitName(r.att.ID) {
		t.Fatalf("unit not named by UnitName: %q", r.sys.startArgs[0][0])
	}
	recs, _ := r.s.Records(ctx)
	if last := recs[len(recs)-1]; last.Kind != KindStarted || last.Unit != info.Unit || last.InvocationID != goodInv {
		t.Fatalf("audit %+v", last)
	}
	if err := r.s.CheckAuditChain(ctx, false); err != nil {
		t.Fatal(err)
	}
}

func TestRefusalBeforeIntentTouchesNothingAndMayAbort(t *testing.T) {
	r := newRig(t, OpCheck)
	r.env.Flags = []string{"bad"}
	_, err := r.l.Launch(context.Background(), r.att, r.env, nil)
	if !errors.Is(err, ErrLaunchRefused) || len(*r.log) != 0 {
		t.Fatalf("err %v, calls %v", err, *r.log)
	}
	if err := r.s.RecordAborted(context.Background(), r.att.ID, "refused", r.now); err != nil {
		t.Fatalf("a refused launch must stay abortable: %v", err)
	}
}

func TestIntentFailureMeansNoSystemdCall(t *testing.T) {
	r := newRig(t, OpCheck)
	ctx := context.Background()
	if err := r.s.RecordAborted(ctx, r.att.ID, "gone", r.now); err != nil {
		t.Fatal(err)
	}
	_, err := r.l.Launch(ctx, r.att, r.env, nil)
	if !errors.Is(err, ErrLaunchRefused) || !errors.Is(err, ErrBadTransition) {
		t.Fatalf("got %v", err)
	}
	if got := strings.Join(*r.log, ","); got != "intent" {
		t.Fatalf("calls %s", got)
	}
}

// recoverWith runs RecoverUnits the way the service will: no by-invocation lookup, the unit lookup over systemd.
func recoverWith(t *testing.T, r *rig, out OutputSummary) []Record {
	t.Helper()
	recs, err := r.s.RecoverUnits(context.Background(), nil, UnitLookupFor(r.sys, out), r.now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func launchLastKind(recs []Record, attempt string) Kind {
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].Attempt == attempt {
			return recs[i].Kind
		}
	}
	return ""
}

type fixedOut struct {
	sha string
	n   int64
	err error
}

func (f fixedOut) Summarize(string) (string, int64, error) { return f.sha, f.n, f.err }

func TestCrashStartFailsLeavesIntentAndRecoveryDecides(t *testing.T) {
	// systemd-run failed: but the unit may exist (the error could have come after it was created).
	for name, c := range map[string]struct {
		unitExists bool
		want       Kind
	}{"unit absent": {false, KindUncertain}, "unit exists": {true, KindStarted}} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, OpApply)
			r.sys.startErr = errors.New("connection reset")
			_, err := r.l.Launch(context.Background(), r.att, r.env, nil)
			if !errors.Is(err, ErrLaunchUnknown) {
				t.Fatalf("got %v", err)
			}
			if err := r.s.RecordAborted(context.Background(), r.att.ID, "x", r.now); !errors.Is(err, ErrLaunchIntent) {
				t.Fatalf("abort allowed after intent: %v", err)
			}
			if c.unitExists {
				r.sys.units = map[string]UnitProps{UnitName(r.att.ID): {LoadState: "loaded", ActiveState: "active", SubState: "running", InvocationID: goodInv}}
			}
			if k := launchLastKind(recoverWith(t, r, nil), r.att.ID); k != c.want {
				t.Fatalf("recovered as %s, want %s", k, c.want)
			}
		})
	}
}

func TestCrashShowFailsAfterStart(t *testing.T) {
	r := newRig(t, OpApply)
	r.sys.showErr = errors.New("bus down")
	_, err := r.l.Launch(context.Background(), r.att, r.env, nil)
	if !errors.Is(err, ErrLaunchUnknown) {
		t.Fatalf("got %v", err)
	}
	if got := strings.Join(*r.log, ","); got != "intent,systemd-run,show" {
		t.Fatalf("order %s", got)
	}
	r.sys.showErr = nil // the bus is back: the unit is running
	if k := launchLastKind(recoverWith(t, r, nil), r.att.ID); k != KindStarted {
		t.Fatalf("recovered as %s", k)
	}
}

func TestCrashBadInvocationIDFromShow(t *testing.T) {
	r := newRig(t, OpApply)
	r.sys.invFor = "" // a unit with no InvocationID cannot be recorded as STARTED
	_, err := r.l.Launch(context.Background(), r.att, r.env, nil)
	if !errors.Is(err, ErrLaunchUnknown) {
		t.Fatalf("got %v", err)
	}
	if len(*r.log) != 3 {
		t.Fatalf("RecordStarted was attempted: %v", *r.log)
	}
}

func TestCrashRecordStartedFailsThenUnitFinished(t *testing.T) {
	r := newRig(t, OpApply)
	r.led.startedErr = errors.New("disk full")
	r.l.Ledger = &r.led
	if _, err := r.l.Launch(context.Background(), r.att, r.env, nil); !errors.Is(err, ErrLaunchUnknown) {
		t.Fatal(err)
	}
	// the script ran to completion before the verifier came back
	unit := UnitName(r.att.ID)
	r.sys.units[unit] = UnitProps{LoadState: "loaded", ActiveState: "inactive", SubState: "dead", Result: "success", InvocationID: goodInv,
		StartedAt: time.Unix(r.now.Unix()+3, 0).UTC(), ExitedAt: time.Unix(r.now.Unix()+45, 0).UTC()}
	out := fixedOut{sha: sha2Hex, n: 7}
	recs := recoverWith(t, r, out)
	last := recs[len(recs)-1]
	if last.Kind != KindEnded || last.Result != ResultSuccess || last.OutputSHA256 != sha2Hex || *last.OutputBytes != 7 || *last.DurationMS != 42000 {
		t.Fatalf("recovered %+v", last)
	}
	if err := r.s.CheckAuditChain(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func TestCrashBetweenIntentAndLaunchNeverRetriesAndHoldsCapacity(t *testing.T) {
	r := newRig(t, OpApply)
	ctx := context.Background()
	// the process died after the intent: nothing else was called
	if _, err := r.s.RecordLaunchIntent(ctx, r.att.ID, r.now); err != nil {
		t.Fatal(err)
	}
	// Recovery finds no unit and records UNCERTAIN; a launch after that is refused, so nothing is started twice.
	recs := recoverWith(t, r, nil)
	if k := launchLastKind(recs, r.att.ID); k != KindUncertain {
		t.Fatalf("got %s", k)
	}
	if _, err := r.l.Launch(ctx, r.att, r.env, nil); !errors.Is(err, ErrLaunchRefused) || len(r.sys.startArgs) != 0 {
		t.Fatalf("launched after recovery: %v %v", err, r.sys.startArgs)
	}
}

func TestUnitLookupMapping(t *testing.T) {
	id := "01HZZZZZZZZZZZZZZZZZZZZZZZ"
	unit := UnitName(id)
	mk := func(p UnitProps) (UnitState, error) {
		f := &fakeSystemd{log: &[]string{}, units: map[string]UnitProps{unit: p}}
		return UnitLookupFor(f, fixedOut{sha: sha2Hex, n: 3})(context.Background(), unit)
	}
	exited := UnitProps{LoadState: "loaded", ActiveState: "inactive", SubState: "dead", InvocationID: goodInv, ExitStatus: 0}
	cases := []struct {
		name    string
		p       UnitProps
		found   bool
		ended   bool
		result  string
		status  int
		invalid bool // EndInfo.validate must fail
		err     bool
	}{
		{"not found", UnitProps{LoadState: "not-found"}, false, false, "", 0, false, false},
		{"running", UnitProps{LoadState: "loaded", ActiveState: "active", SubState: "running", InvocationID: goodInv}, true, false, "", 0, false, false},
		{"activating", UnitProps{LoadState: "loaded", ActiveState: "activating", InvocationID: goodInv}, true, false, "", 0, false, false},
		{"success", with(exited, "success", 0), true, true, ResultSuccess, 0, false, false},
		{"exit-code 3", with(unitFailed(exited), "exit-code", 3), true, true, ResultFailed, 3, false, false},
		{"exit-code zero is unmappable", with(unitFailed(exited), "exit-code", 0), true, true, ResultFailed, 0, true, false},
		{"timeout", with(unitFailed(exited), "timeout", 0), true, true, ResultTimeout, 0, false, false},
		{"signal", with(unitFailed(exited), "signal", 9), true, true, ResultSignal, 9, false, false},
		{"core-dump", with(unitFailed(exited), "core-dump", 11), true, true, ResultSignal, 11, false, false},
		{"resources unmapped", with(unitFailed(exited), "resources", 0), true, true, "", 0, true, false},
		// fix 1: a finished main process with a live cgroup is running, never ended
		{"active/exited, cgroup empty", retained(0, true), true, true, ResultSuccess, 0, false, false},
		{"active/exited, cgroup populated", retained(3, true), true, false, "", 0, false, false},
		{"active/exited, cgroup unknown", retained(0, false), true, false, "", 0, false, false},
		{"inactive with a populated cgroup", populated(with(exited, "success", 0)), true, false, "", 0, false, false},
		{"failed with a populated cgroup", populated(with(unitFailed(exited), "exit-code", 1)), true, false, "", 0, false, false},
		{"unknown state", UnitProps{LoadState: "loaded", ActiveState: "maintenance", InvocationID: goodInv}, false, false, "", 0, false, true},
	}
	for _, c := range cases {
		st, err := mk(c.p)
		if (err != nil) != c.err {
			t.Errorf("%s: err %v", c.name, err)
			continue
		}
		if c.err {
			continue
		}
		if st.Found != c.found || (st.Ended != nil) != c.ended {
			t.Errorf("%s: %+v", c.name, st)
			continue
		}
		if c.ended {
			if st.Ended.Result != c.result || st.Ended.ExitStatus != c.status || (st.Ended.validate() != nil) != c.invalid {
				t.Errorf("%s: %+v (validate %v)", c.name, *st.Ended, st.Ended.validate())
			}
		}
	}
	if _, err := UnitLookupFor(&fakeSystemd{log: &[]string{}}, nil)(context.Background(), "ssh.service"); err == nil {
		t.Error("looked up a unit that is not a run unit")
	}
	// no output summary: the unit is known to have ended, but the outcome is not recordable as ENDED
	f := &fakeSystemd{log: &[]string{}, units: map[string]UnitProps{unit: with(exited, "success", 0)}}
	st, _ := UnitLookupFor(f, nil)(context.Background(), unit)
	if st.Ended == nil || st.Ended.validate() == nil {
		t.Errorf("ended unit without an output digest must not validate: %+v", st.Ended)
	}
}

func with(p UnitProps, result string, status int) UnitProps {
	p.Result, p.ExitStatus = result, status
	return p
}
func unitFailed(p UnitProps) UnitProps { p.ActiveState, p.SubState = "failed", "failed"; return p }

// populated: processes are still in the unit's cgroup (a background child outlived the main process).
func populated(p UnitProps) UnitProps { p.TasksKnown, p.Tasks = true, 2; return p }

// retained is what RemainAfterExit=yes leaves behind once the cgroup is empty: active (exited).
func retained(tasks uint64, known bool) UnitProps {
	return UnitProps{LoadState: "loaded", ActiveState: "active", SubState: "exited", Result: "success", InvocationID: goodInv, TasksKnown: known, Tasks: tasks}
}

func TestRecoveryEndedWithUnreadableLogIsUncertain(t *testing.T) {
	r := newRig(t, OpApply)
	if _, err := r.l.Launch(context.Background(), r.att, r.env, nil); err != nil {
		t.Fatal(err)
	}
	r.sys.units[UnitName(r.att.ID)] = with(UnitProps{LoadState: "loaded", ActiveState: "inactive", SubState: "dead", InvocationID: goodInv}, "success", 0)
	if k := launchLastKind(recoverWith(t, r, fixedOut{err: os.ErrNotExist}), r.att.ID); k != KindUncertain {
		t.Fatalf("got %s", k)
	}
}

func TestRecoveryRejectsUnitWithOtherInvocation(t *testing.T) {
	r := newRig(t, OpApply)
	if _, err := r.l.Launch(context.Background(), r.att, r.env, nil); err != nil {
		t.Fatal(err)
	}
	// a different invocation of the same name: the recorded run is gone, its outcome unknown
	r.sys.units[UnitName(r.att.ID)] = UnitProps{LoadState: "loaded", ActiveState: "active", SubState: "running", InvocationID: strings.Repeat("a", 32)}
	if k := launchLastKind(recoverWith(t, r, nil), r.att.ID); k != KindUncertain {
		t.Fatalf("got %s", k)
	}
}

type fakeRunner struct {
	calls [][]string
	out   []byte
	err   error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.out, f.err
}

func TestCLISystemdCommandLines(t *testing.T) {
	fr := &fakeRunner{out: []byte("LoadState=loaded\nActiveState=inactive\nSubState=dead\nControlGroup=\nResult=exit-code\nInvocationID=" + goodInv +
		"\nExecMainStatus=3\nExecMainStartTimestamp=@1000\nExecMainExitTimestamp=@1005\n")}
	c := CLISystemd{Runner: fr}
	if err := c.Start(context.Background(), []string{"--unit=x.service", "--", "/bin/bash"}); err != nil {
		t.Fatal(err)
	}
	p, err := c.Show(context.Background(), "x.service")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"systemd-run", "--unit=x.service", "--", "/bin/bash"},
		{"systemctl", "show", "--timestamp=unix", "--property=LoadState,ActiveState,SubState,Result,InvocationID,ExecMainStatus,ExecMainStartTimestamp,ExecMainExitTimestamp,TasksCurrent", "--", "x.service"},
	}
	if !reflect.DeepEqual(fr.calls, want) {
		t.Fatalf("commands %q", fr.calls)
	}
	for _, call := range fr.calls {
		if contains(call, "--user") {
			t.Fatal("user manager used")
		}
	}
	if p.Result != "exit-code" || p.ExitStatus != 3 || p.InvocationID != goodInv || p.ExitedAt.Sub(p.StartedAt) != 5*time.Second {
		t.Fatalf("props %+v", p)
	}
	fr.err = fmt.Errorf("boom")
	if err := c.Start(context.Background(), nil); err == nil {
		t.Fatal("start error swallowed")
	}
	if _, err := c.Show(context.Background(), "x"); err == nil {
		t.Fatal("show error swallowed")
	}
}

func TestParseShowRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "ActiveState=active\n", "LoadState=loaded\nExecMainStatus=x\n", "LoadState=loaded\nExecMainExitTimestamp=Tue 2026-01-01\n"} {
		if _, err := ParseShow([]byte(in)); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	p, err := ParseShow([]byte("LoadState=not-found\nExecMainStartTimestamp=n/a\nExecMainExitTimestamp=@0\n"))
	if err != nil || p.LoadState != "not-found" || !p.StartedAt.IsZero() || !p.ExitedAt.IsZero() {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestAttemptFromUnitInvertsUnitName(t *testing.T) {
	id := "01HZZZZZZZZZZZZZZZZZZZZZZZ"
	got, err := AttemptFromUnit(UnitName(id))
	if err != nil || got != id {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"", "homerun-.service", "homerun-xyz.service", "other-01hzzzzzzzzzzzzzzzzzzzzzzz.service", "homerun-01hzzzzzzzzzzzzzzzzzzzzzzz.timer", "homerun-01HZZZZZZZZZZZZZZZZZZZZZZZ.service"} {
		if _, err := AttemptFromUnit(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestFileOutputSummarize(t *testing.T) {
	dir := t.TempDir()
	cfg := LaunchConfig{LogDir: dir}
	if err := os.WriteFile(filepath.Join(dir, "A.log"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}
	sha, n, err := FileOutput{Config: cfg}.Summarize("A")
	if err != nil || n != 5 || sha != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("%s %d %v", sha, n, err)
	}
	if _, _, err := (FileOutput{Config: cfg}).Summarize("missing"); err == nil {
		t.Fatal("missing log accepted")
	}
}

// Compile-time checks: the launcher takes interfaces, and only ExecRunner starts a process.
var (
	_ Systemd = CLISystemd{}
	_ Ledger  = (*Store)(nil)
)

func TestFix1BackgroundChildNeverRecordsEnded(t *testing.T) {
	for name, p := range map[string]UnitProps{
		"active/exited, cgroup populated": retained(2, true),
		"active/exited, cgroup unknown":   retained(0, false),
		"inactive, cgroup populated":      populated(UnitProps{LoadState: "loaded", ActiveState: "inactive", SubState: "dead", Result: "success", InvocationID: goodInv}),
	} {
		r := newRig(t, OpApply)
		if _, err := r.l.Launch(context.Background(), r.att, r.env, nil); err != nil {
			t.Fatal(err)
		}
		r.sys.units[UnitName(r.att.ID)] = p
		k := launchLastKind(recoverWith(t, r, fixedOut{sha: sha2Hex, n: 1}), r.att.ID)
		if k == KindEnded || k == KindUncertain {
			t.Fatalf("%s: recorded %s; a populated cgroup is still running", name, k)
		}
	}
}

func TestFix1UnitStaysRunningUntilEveryProcessExits(t *testing.T) {
	r := newRig(t, OpApply)
	args, err := BuildRunArgs(testCfg(), DefaultLimits(), r.att, r.env, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--property=ExitType=cgroup", "--property=TasksAccounting=yes", "--property=RemainAfterExit=yes", "--property=KillMode=control-group"} {
		if !contains(args, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestFix3LogIsCreatedPrivate(t *testing.T) {
	// No log exists before the run: the file systemd creates gets 0666 &^ UMask, so UMask must be 0027.
	dir := t.TempDir()
	cfg := testCfg()
	cfg.LogDir = dir
	r := newRig(t, OpCheck)
	args, err := BuildRunArgs(cfg, DefaultLimits(), r.att, r.env, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(args, "--property=UMask=0027") {
		t.Fatalf("no restrictive umask: %q", args)
	}
	if _, err := os.Stat(filepath.Join(dir, r.att.ID+".log")); !os.IsNotExist(err) {
		t.Fatal("the launcher must not rely on a pre-created log")
	}
}

func TestFix2ResolveWrites(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	mkdir := func(p ...string) string {
		d := filepath.Join(append([]string{root}, p...)...)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	cfg := LaunchConfig{User: "runner", CASRoot: "/srv/cas", LogDir: "/srv/log", WritesRoot: root}
	work := mkdir("work")
	mkdir("other")
	sym := func(target, name string) string {
		l := filepath.Join(root, name)
		if err := os.Symlink(target, l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	toRoot := sym(root, "alias-root")      // points at the whole writes root
	toOther := sym("other", "alias-other") // relative link, stays inside
	deep := mkdir("work", "sub")
	sym(root, "work/up")
	if got, err := ResolveWrites(cfg, []string{work, deep}); err != nil || got.Paths[0] != work || got.Paths[1] != deep {
		t.Fatalf("plain paths: %v %v", got, err)
	}
	for name, w := range map[string][]string{
		"symlink to the root":     {toRoot},
		"symlink inside the root": {toOther},
		"symlink component":       {filepath.Join(toOther, "x")},
		"nested symlink":          {filepath.Join(work, "up")},
		"missing path":            {filepath.Join(root, "nope")},
		"outside the root":        {"/etc"},
	} {
		if _, err := ResolveWrites(cfg, w); !errors.Is(err, ErrLaunchRefused) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	// the root itself may be reached through a symlink; the resolved real path is what is passed on
	alias := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	cfg2 := cfg
	cfg2.WritesRoot = alias
	got, err := ResolveWrites(cfg2, []string{filepath.Join(alias, "work")})
	if err != nil || got.Paths[0] != work {
		t.Fatalf("root alias: %v %v", got, err)
	}
	// swap after approval: the directory is replaced by a symlink before the unit starts
	ws, err := ResolveWrites(cfg, []string{work})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Verify(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(deep); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(work, "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(work); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, work); err != nil {
		t.Fatal(err)
	}
	if err := ws.Verify(); err == nil {
		t.Fatal("swapped path still verifies")
	}
}

func TestFix2LaunchPassesResolvedPathsAndRechecksBeforeStart(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "ln")
	if err := os.Symlink(work, link); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, OpApply)
	r.l.Config.WritesRoot = root
	ctx := context.Background()
	if _, err := r.l.Launch(ctx, r.att, r.env, []string{link}); !errors.Is(err, ErrLaunchRefused) || len(*r.log) != 0 {
		t.Fatalf("symlinked write path launched: %v %v", err, *r.log)
	}
	// swap between the intent and systemd-run
	r.sys.onStart = nil
	r.l.AfterIntent = func() {
		_ = os.Remove(work)
		_ = os.Symlink(root, work)
	}
	_, err := r.l.Launch(ctx, r.att, r.env, []string{work})
	if !errors.Is(err, ErrLaunchUnknown) || len(r.sys.startArgs) != 0 {
		t.Fatalf("swap not caught before systemd-run: %v %v", err, r.sys.startArgs)
	}
}

func TestFix2ResolvedPathIsWhatSystemdGets(t *testing.T) {
	real, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.Mkdir(filepath.Join(real, "w"), 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "h")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, OpApply)
	r.l.Config.WritesRoot = alias
	if _, err := r.l.Launch(context.Background(), r.att, r.env, []string{filepath.Join(alias, "w")}); err != nil {
		t.Fatal(err)
	}
	if !contains(r.sys.startArgs[0], "--property=ReadWritePaths="+filepath.Join(real, "w")) {
		t.Fatalf("args %q", r.sys.startArgs[0])
	}
}

func TestParseShowTasksCurrent(t *testing.T) {
	for in, want := range map[string]struct {
		known bool
		n     uint64
	}{
		"LoadState=loaded\nTasksCurrent=0\n":                    {true, 0},
		"LoadState=loaded\nTasksCurrent=4\n":                    {true, 4},
		"LoadState=loaded\nTasksCurrent=[not set]\n":            {false, 0},
		"LoadState=loaded\nTasksCurrent=18446744073709551615\n": {false, 0},
		"LoadState=loaded\n":                                    {false, 0},
	} {
		p, err := ParseShow([]byte(in))
		if err != nil || p.TasksKnown != want.known || (want.known && p.Tasks != want.n) {
			t.Errorf("%q: %+v %v", in, p, err)
		}
	}
}
