//go:build unix

package homerun

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
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
		// A finished unit stays (RemainAfterExit) so recovery can read how it ended, but it is only
		// finished once its whole cgroup is empty (ExitType=cgroup, and TasksAccounting so the
		// process count can be read). See UnitProps.settled and UnitLookupFor.
		prop("TasksAccounting", "yes"),
		prop("ExitType", "cgroup"),
		prop("RemainAfterExit", "yes"),
		// The log is created by systemd as the unit's user; this umask makes it 0640 instead of 0644.
		prop("UMask", "0027"),
		prop("StandardOutput", "file:"+cfg.LogPath(att.ID)),
		prop("StandardError", "inherit"),
		"--setenv=PATH="+UnitPATH,
		"--setenv="+DepsEnv+"="+cfg.CASRoot,
		"--",
	)
	return append(args, LaunchArgv(cfg.bash(), casPath, env)...), nil
}

// WriteSet is a set of writable directories resolved to real paths, with the identity of every path
// component, so that a later Verify can notice a swap.
type WriteSet struct {
	Root  string   // the real path of WritesRoot
	Paths []string // real paths, in the order given
	ids   []map[string]fileID
}

type fileID struct{ dev, ino uint64 }

func lstatID(p string) (fileID, os.FileInfo, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return fileID{}, nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, nil, fmt.Errorf("no file identity for %s", p)
	}
	return fileID{uint64(st.Dev), uint64(st.Ino)}, fi, nil //nolint:unconvert
}

// ResolveWrites turns the requested writable paths into real directories under WritesRoot. Every
// component below the root is lstat'ed: a symlink anywhere in it is refused, so the path cannot lead
// outside the root. WritesRoot itself may be reached through a symlink; its real path is what is used.
// The resolved paths, not the requested ones, are what the unit is given.
//
// Residual risk, not closed here: this checks the tree at one moment. systemd re-resolves ReadWritePaths
// when it builds the unit's mount namespace, after systemd-run is called, so a path an attacker who can
// write inside WritesRoot swaps for a symlink in that window is followed by systemd. Launch narrows the
// window (Verify runs immediately before systemd-run, after the launch intent) but cannot close it;
// closing it needs WritesRoot to be writable only by trusted accounts, or a directory the unit's own
// user cannot rename within.
func ResolveWrites(cfg LaunchConfig, writes []string) (WriteSet, error) {
	ws, err := resolveWrites(cfg, writes)
	if err != nil {
		return WriteSet{}, fmt.Errorf("%w: %v", ErrLaunchRefused, err)
	}
	return ws, nil
}

func resolveWrites(cfg LaunchConfig, writes []string) (WriteSet, error) {
	if len(writes) == 0 {
		return WriteSet{}, nil
	}
	if err := cfg.CheckWrites(writes); err != nil {
		return WriteSet{}, err
	}
	realRoot, err := filepath.EvalSymlinks(cfg.WritesRoot)
	if err != nil {
		return WriteSet{}, fmt.Errorf("writes root %s: %w", cfg.WritesRoot, err)
	}
	if err := checkUnitPath("writes root", realRoot); err != nil {
		return WriteSet{}, err
	}
	ws := WriteSet{Root: realRoot}
	seen := map[string]bool{}
	for _, w := range writes {
		rel, ok := strings.CutPrefix(w, cfg.WritesRoot+"/")
		if !ok {
			if rel, ok = strings.CutPrefix(w, realRoot+"/"); !ok {
				return WriteSet{}, fmt.Errorf("writes entry %q is not under %s", w, cfg.WritesRoot)
			}
		}
		cur, ids := realRoot, map[string]fileID{}
		for _, comp := range strings.Split(rel, "/") {
			cur = path.Join(cur, comp)
			id, fi, err := lstatID(cur)
			if err != nil {
				return WriteSet{}, fmt.Errorf("writes entry %q: %w", w, err)
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				return WriteSet{}, fmt.Errorf("writes entry %q passes through a symlink at %s", w, cur)
			}
			if !fi.IsDir() {
				return WriteSet{}, fmt.Errorf("writes entry %q: %s is not a directory", w, cur)
			}
			ids[cur] = id
		}
		if seen[cur] {
			return WriteSet{}, fmt.Errorf("writes entry %q is repeated", w)
		}
		seen[cur] = true
		ws.Paths = append(ws.Paths, cur)
		ws.ids = append(ws.ids, ids)
	}
	return ws, nil
}

// Verify re-checks every resolved component: still no symlink, still the same file, and WritesRoot
// still resolves to the same place.
func (ws WriteSet) Verify() error {
	if len(ws.Paths) == 0 {
		return nil
	}
	if cur, err := filepath.EvalSymlinks(ws.Root); err != nil || cur != ws.Root {
		return fmt.Errorf("writes root %s changed", ws.Root)
	}
	for _, ids := range ws.ids {
		for p, want := range ids {
			id, fi, err := lstatID(p)
			if err != nil || fi.Mode()&os.ModeSymlink != 0 || id != want {
				return fmt.Errorf("writes path %s changed after it was checked", p)
			}
		}
	}
	return nil
}

// checkTrustedChain requires root and every parent up to "/" to be a real directory (no symlink) owned
// by an allowed uid and writable by neither group nor other, sticky bit or not. Such a tree cannot be
// renamed or swapped by an unprivileged agent, which is what closes the window between Verify and
// systemd resolving ReadWritePaths for it. lstat returns uid, mode.
//
// UNVERIFIED-live: nothing here is exercised against a real tree in tests (they use a fake lstat); it
// also does not cover an agent that already runs as root or as an allowed owner, nor ACLs, capabilities,
// or a writable bind mount over a parent.
func checkTrustedChain(root string, lstat func(string) (uint32, fs.FileMode, error), allowed func(uint32) bool) error {
	for p := root; ; p = path.Dir(p) {
		uid, mode, err := lstat(p)
		if err != nil {
			return fmt.Errorf("trust check %s: %w", p, err)
		}
		if mode&fs.ModeSymlink != 0 || !mode.IsDir() {
			return fmt.Errorf("%s is not a real directory", p)
		}
		if !allowed(uid) {
			return fmt.Errorf("%s is owned by uid %d, which is not trusted", p, uid)
		}
		if mode.Perm()&0o022 != 0 {
			return fmt.Errorf("%s is writable by group or other (mode %04o)", p, mode.Perm())
		}
		if p == "/" {
			return nil
		}
	}
}

// TrustedRoot is the production trust check: owner root or the service user.
func TrustedRoot(serviceUID uint32) func(string) error {
	return func(root string) error {
		return checkTrustedChain(root, func(p string) (uint32, fs.FileMode, error) {
			fi, err := os.Lstat(p)
			if err != nil {
				return 0, 0, err
			}
			st, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				return 0, 0, fmt.Errorf("no owner for %s", p)
			}
			return st.Uid, fi.Mode(), nil
		}, func(uid uint32) bool { return uid == 0 || uid == serviceUID })
	}
}

// LogFiles creates and removes the unit's log file.
type LogFiles interface {
	// Create makes the log exclusively (it must not exist), owned root:<log group>, mode 0640.
	Create(path string) error
	Remove(path string) error
}

// RootLogs is the production LogFiles. The launcher process must be root for the chown.
//
// UNVERIFIED-live: that systemd, which opens StandardOutput=file: before it drops to User=, appends to
// an existing file of this owner and mode without changing them, and that the service user can still
// write it. Systemd's documentation says "file:" opens (truncates=no) and does not chown; not observed.
type RootLogs struct {
	Group     string
	LookupGID func(group string) (int, error)      // default os/user
	Fchown    func(f *os.File, uid, gid int) error // default f.Chown
}

// Create implements LogFiles.
func (r RootLogs) Create(p string) error {
	if r.Group == "" {
		return errors.New("no log group configured")
	}
	lookup := r.LookupGID
	if lookup == nil {
		lookup = func(g string) (int, error) {
			gr, err := user.LookupGroup(g)
			if err != nil {
				return 0, err
			}
			return strconv.Atoi(gr.Gid)
		}
	}
	gid, err := lookup(r.Group)
	if err != nil {
		return fmt.Errorf("log group %q: %w", r.Group, err)
	}
	chown := r.Fchown
	if chown == nil {
		chown = func(f *os.File, uid, gid int) error { return f.Chown(uid, gid) }
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o640)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		f.Close()
		_ = os.Remove(p) // never leave a log with the wrong owner or mode
		return err
	}
	if err := chown(f, 0, gid); err != nil {
		return fail(fmt.Errorf("chown log: %w", err))
	}
	if err := f.Chmod(0o640); err != nil { // the process umask may have narrowed it
		return fail(err)
	}
	return f.Close()
}

// Remove implements LogFiles.
func (RootLogs) Remove(p string) error { return os.Remove(p) }

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
	// AfterIntent, when set, runs between the launch intent and systemd-run. It is a seam for tests of the
	// swap-after-check window; production leaves it nil.
	AfterIntent func()
	// Trust checks that WritesRoot and its parents cannot be swapped by an unprivileged agent. Nil means
	// TrustedRoot for the account named in Config.User (looked up with LookupUID), which fails closed.
	Trust     func(writesRoot string) error
	LookupUID func(user string) (uint32, error) // default os/user
	// Logs precreates the log. Nil refuses every launch: a launcher with no way to make the log
	// root-owned would let systemd create it as the wrong group.
	Logs LogFiles
}

func (l *Launcher) trust(root string) error {
	if l.Trust != nil {
		return l.Trust(root)
	}
	lookup := l.LookupUID
	if lookup == nil {
		lookup = func(n string) (uint32, error) {
			u, err := user.Lookup(n)
			if err != nil {
				return 0, err
			}
			id, err := strconv.ParseUint(u.Uid, 10, 32)
			return uint32(id), err
		}
	}
	uid, err := lookup(l.Config.User)
	if err != nil {
		return fmt.Errorf("service user %q: %w", l.Config.User, err)
	}
	return TrustedRoot(uid)(root)
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
	ws, err := ResolveWrites(l.Config, writes)
	if err != nil {
		return StartInfo{}, err
	}
	cfg := l.Config
	if len(ws.Paths) > 0 {
		cfg.WritesRoot = ws.Root // the unit is given real paths only
	}
	args, err := BuildRunArgs(cfg, l.Limits, att, env, ws.Paths)
	if err != nil {
		return StartInfo{}, err
	}
	if len(ws.Paths) > 0 {
		if err := l.trust(ws.Root); err != nil {
			return StartInfo{}, fmt.Errorf("%w: %v", ErrLaunchRefused, err)
		}
	}
	if l.Logs == nil {
		return StartInfo{}, fmt.Errorf("%w: no log creator configured", ErrLaunchRefused)
	}
	logPath := cfg.LogPath(att.ID)
	if err := l.Logs.Create(logPath); err != nil {
		return StartInfo{}, fmt.Errorf("%w: create log: %v", ErrLaunchRefused, err)
	}
	unit, err := l.Ledger.RecordLaunchIntent(ctx, att.ID, l.now())
	if err != nil {
		// No intent was recorded, so nothing was launched; take the log back so a retry can create it.
		_ = l.Logs.Remove(logPath)
		return StartInfo{}, fmt.Errorf("%w: %w", ErrLaunchRefused, err)
	}
	if unit != UnitName(att.ID) {
		return StartInfo{}, fmt.Errorf("%w: ledger named unit %q, expected %q", ErrLaunchUnknown, unit, UnitName(att.ID))
	}
	if l.AfterIntent != nil {
		l.AfterIntent()
	}
	// Last look before systemd resolves the paths itself (see ResolveWrites for the residual window).
	if err := ws.Verify(); err != nil {
		return StartInfo{}, fmt.Errorf("%w: %v", ErrLaunchUnknown, err)
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
