//go:build unix

package homerun

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This file is the as-mk launcher (bead mk-21nfo, slice 3): it turns a consumed attempt and its
// envelope into one transient systemd unit, in the order the audit trail needs. It never runs a
// process itself; every call to systemd goes through the Systemd interface (systemd.go).

// Fixed hardening from the design (section 4.4 and the dependency rule of 4.3). They are not
// configuration: loosening one changes what a signed approval means.
var (
	// execPathsSystem are the trees a run may execute from, besides the CAS root. Before launch the
	// verifier scans them for mk-writable files (a later slice).
	execPathsSystem = []string{"/usr", "/bin", "/sbin", "/lib", "/lib64"}
	// inaccessiblePaths are the container-runtime sockets a run must not reach.
	inaccessiblePaths = []string{"/run/docker.sock", "/var/run/docker.sock", "/run/containerd"}
)

// UnitPATH is the PATH of every run (design 4.3).
const UnitPATH = "/usr/sbin:/usr/bin:/sbin:/bin"

// DepsEnv names the environment variable that points at the CAS root, so a script can reach its pinned
// dependencies as "$HOME_RUN_DEPS/<sha256>".
const DepsEnv = "HOME_RUN_DEPS"

var (
	// ErrLaunchRefused: the launch was refused before a launch intent was recorded. Nothing was started
	// and the attempt is still AUTHORIZED, so the caller may record ABORTED_BEFORE_START.
	ErrLaunchRefused = errors.New("launch refused before any launch intent")
	// ErrLaunchUnknown: a launch intent is on record and the unit may or may not exist. The attempt must
	// not be aborted or retried; RecoverUnits settles it.
	ErrLaunchUnknown = errors.New("launch outcome unknown; reconcile with RecoverUnits")
)

// LaunchConfig is the deployment the launcher targets. There are deliberately no defaults for the user,
// the CAS root, the log directory or the writable root: a launcher that guessed them could run a script
// as the wrong account or let it write the wrong tree.
type LaunchConfig struct {
	User       string // the account the unit runs as (User=)
	Bash       string // absolute path of the interpreter; default /bin/bash
	CASRoot    string // absolute CAS directory (the same one CAS.Root names)
	LogDir     string // absolute directory for <attempt>.log
	WritesRoot string // every ReadWritePaths entry must be under this directory
}

var (
	reUser    = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	reUnitPth = regexp.MustCompile(`^/[A-Za-z0-9._/@+=,~-]*$`)
)

// checkUnitPath accepts only a clean absolute path made of characters that need no quoting in a systemd
// property (no space, so the entry cannot split into two; no %, so no specifier).
func checkUnitPath(what, p string) error {
	if p == "" || len(p) > maxPathLen || !reUnitPth.MatchString(p) || path.Clean(p) != p || p == "/" {
		return fmt.Errorf("%s %q must be a clean absolute path of plain characters, not /", what, p)
	}
	return nil
}

func (c LaunchConfig) bash() string {
	if c.Bash == "" {
		return "/bin/bash"
	}
	return c.Bash
}

// Validate reports a configuration that cannot launch safely.
func (c LaunchConfig) Validate() error {
	var errs []error
	if !reUser.MatchString(c.User) || c.User == "root" {
		errs = append(errs, fmt.Errorf("User %q must be a non-root account name", c.User))
	}
	for name, p := range map[string]string{"Bash": c.bash(), "CASRoot": c.CASRoot, "LogDir": c.LogDir, "WritesRoot": c.WritesRoot} {
		if err := checkUnitPath(name, p); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CheckWrites validates the ReadWritePaths entries: clean, absolute, strictly under WritesRoot.
func (c LaunchConfig) CheckWrites(writes []string) error {
	seen := map[string]bool{}
	for _, w := range writes {
		if err := checkUnitPath("writes entry", w); err != nil {
			return err
		}
		if !strings.HasPrefix(w, c.WritesRoot+"/") {
			return fmt.Errorf("writes entry %q is not under %s", w, c.WritesRoot)
		}
		if seen[w] {
			return fmt.Errorf("writes entry %q is repeated", w)
		}
		seen[w] = true
	}
	return nil
}

// LogPath is where the unit's output goes.
func (c LaunchConfig) LogPath(attempt string) string { return path.Join(c.LogDir, attempt+".log") }

// BuildRunArgs is the systemd-run argument list (without the program name) for an attempt. It is a pure
// function of its inputs: the tests pin it as a golden list. The runtime comes from att.RuntimeMax and
// is checked against lim.RuntimeCap(att.Op); the script and flags come from env, which must be the
// envelope att was consumed from. Every error is a refusal (ErrLaunchRefused).
func BuildRunArgs(cfg LaunchConfig, lim Limits, att *Attempt, env *Envelope, writes []string) ([]string, error) {
	refuse := func(format string, a ...any) ([]string, error) {
		return nil, fmt.Errorf("%w: %s", ErrLaunchRefused, fmt.Sprintf(format, a...))
	}
	if att == nil || env == nil {
		return refuse("attempt and envelope are required")
	}
	if err := cfg.Validate(); err != nil {
		return refuse("%v", err)
	}
	if !reAttempt.MatchString(att.ID) {
		return refuse("attempt id is not a ULID")
	}
	if att.Privilege != PrivMk || env.Privilege != PrivMk {
		return refuse("the as-mk launcher starts privilege %q only", PrivMk)
	}
	if att.Op != env.Op || att.ScriptSHA256 != env.ScriptSHA256 || att.CardID != env.CardID {
		return refuse("attempt and envelope disagree on op, script or card")
	}
	switch att.Op {
	case OpCheck, OpApply, OpRecover:
	default:
		return refuse("unknown op %q", att.Op)
	}
	if !reHex64.MatchString(att.ScriptSHA256) {
		return refuse("script digest is not a sha256")
	}
	if len(env.Flags) > maxFlags {
		return refuse("too many flags")
	}
	for _, f := range env.Flags {
		if !reFlag.MatchString(f) {
			return refuse("flag %q is not allowed", f)
		}
	}
	// Whole seconds: RuntimeMaxSec=N is an integer, and a fractional signed value would silently round.
	if att.RuntimeMax < time.Second || att.RuntimeMax%time.Second != 0 || att.RuntimeMax > lim.RuntimeCap(att.Op) {
		return refuse("runtime %v is outside 1s..%v for %s", att.RuntimeMax, lim.RuntimeCap(att.Op), att.Op)
	}
	if err := cfg.CheckWrites(writes); err != nil {
		return refuse("%v", err)
	}
	cas := &CAS{Root: cfg.CASRoot}
	casPath, err := cas.Path(att.ScriptSHA256)
	if err != nil {
		return refuse("%v", err)
	}

	prop := func(k, v string) string { return "--property=" + k + "=" + v }
	args := []string{
		"--unit=" + UnitName(att.ID),
		"--description=Citadel script run " + att.ID,
		"--quiet",
		"--no-ask-password",
		// exec: systemd-run returns once the process has been exec'd, and fails if it cannot be.
		"--service-type=exec",
		prop("User", cfg.User),
		prop("NoNewPrivileges", "yes"),
		prop("RestrictSUIDSGID", "yes"),
		prop("CapabilityBoundingSet", ""),
		prop("InaccessiblePaths", strings.Join(inaccessiblePaths, " ")),
		prop("PrivateTmp", "yes"),
		prop("ProtectSystem", "strict"),
		prop("NoExecPaths", "/"),
		prop("ExecPaths", strings.Join(append(append([]string{}, execPathsSystem...), cfg.CASRoot), " ")),
	}
	if len(writes) > 0 {
		args = append(args, prop("ReadWritePaths", strings.Join(writes, " ")))
	}
	args = append(args,
		prop("RuntimeMaxSec", strconv.FormatInt(int64(att.RuntimeMax/time.Second), 10)),
		prop("KillMode", "control-group"),
		// The unit stays after the script exits, so recovery can read how it ended.
		prop("RemainAfterExit", "yes"),
		prop("StandardOutput", "file:"+cfg.LogPath(att.ID)),
		prop("StandardError", "inherit"),
		"--setenv=PATH="+UnitPATH,
		"--setenv="+DepsEnv+"="+cfg.CASRoot,
		"--",
	)
	return append(args, LaunchArgv(cfg.bash(), casPath, env)...), nil
}

// Ledger is the part of Store the launcher drives. *Store implements it.
type Ledger interface {
	RecordLaunchIntent(ctx context.Context, attempt string, at time.Time) (string, error)
	RecordStarted(ctx context.Context, attempt string, info StartInfo, at time.Time) error
}

// Launcher starts attempts as transient units.
type Launcher struct {
	Config LaunchConfig
	Limits Limits
	Ledger Ledger
	System Systemd
	Now    func() time.Time // default time.Now
}

func (l *Launcher) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// Launch runs the three steps in the only safe order:
//
//  1. validate and build the argument list (nothing recorded; ErrLaunchRefused);
//  2. RecordLaunchIntent, durably, before systemd is asked for anything;
//  3. start the unit, read its InvocationID back, and RecordStarted.
//
// After step 2 no failure is reported as "did not run": a failed start, a failed read-back or a failed
// RecordStarted returns ErrLaunchUnknown, the attempt keeps its intent, and RecoverUnits decides from
// what systemd says. The launcher never calls RecordAborted and never retries.
func (l *Launcher) Launch(ctx context.Context, att *Attempt, env *Envelope, writes []string) (StartInfo, error) {
	args, err := BuildRunArgs(l.Config, l.Limits, att, env, writes)
	if err != nil {
		return StartInfo{}, err
	}
	unit, err := l.Ledger.RecordLaunchIntent(ctx, att.ID, l.now())
	if err != nil {
		// No intent was recorded, so nothing was launched.
		return StartInfo{}, fmt.Errorf("%w: %w", ErrLaunchRefused, err)
	}
	if unit != UnitName(att.ID) {
		return StartInfo{}, fmt.Errorf("%w: ledger named unit %q, expected %q", ErrLaunchUnknown, unit, UnitName(att.ID))
	}
	if err := l.System.Start(ctx, args); err != nil {
		return StartInfo{}, fmt.Errorf("%w: start: %w", ErrLaunchUnknown, err)
	}
	props, err := l.System.Show(ctx, unit)
	if err != nil {
		return StartInfo{}, fmt.Errorf("%w: read back %s: %w", ErrLaunchUnknown, unit, err)
	}
	info := StartInfo{Unit: unit, InvocationID: props.InvocationID, CASSHA: att.ScriptSHA256}
	if err := info.validate(att.ScriptSHA256); err != nil {
		return StartInfo{}, fmt.Errorf("%w: %w", ErrLaunchUnknown, err)
	}
	if err := l.Ledger.RecordStarted(ctx, att.ID, info, l.now()); err != nil {
		return StartInfo{}, fmt.Errorf("%w: record started: %w", ErrLaunchUnknown, err)
	}
	return info, nil
}
