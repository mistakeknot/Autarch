package escalcheck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const maxScript = 4 << 20

type snapshot struct {
	canon  string
	data   []byte
	digest string
	mode   os.FileMode
	size   int64
	dev    uint64
	ino    uint64
}

func takeSnapshot(path string) (*snapshot, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	if real != filepath.Clean(abs) {
		return nil, fmt.Errorf("a symlink is in the path: %s resolves to %s", abs, real)
	}
	b, st, err := readRegular(real, maxScript)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	s := &snapshot{canon: real, data: b, digest: hex.EncodeToString(sum[:]), mode: st.Mode(), size: st.Size()}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		s.dev, s.ino = uint64(sys.Dev), uint64(sys.Ino)
	}
	return s, nil
}

func (s *snapshot) same(o *snapshot) bool {
	return s.canon == o.canon && s.digest == o.digest && s.mode == o.mode && s.size == o.size && s.dev == o.dev && s.ino == o.ino
}

// childEnv is the inherited environment without anything that redirects gh or
// the rehearsal state, plus the given KEY=VALUE pairs.
func childEnv(environ []string, extra ...string) []string {
	drop := map[string]bool{"GH_HOST": true, "GH_REPO": true, "GH_ENTERPRISE_TOKEN": true, "GITHUB_ENTERPRISE_TOKEN": true, "HANDOFF_REHEARSE_STATE": true}
	var out []string
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if drop[k] {
			continue
		}
		out = append(out, kv)
	}
	return append(out, extra...)
}

func getenv(environ []string, key string) string {
	v := ""
	for _, kv := range environ {
		if k, val, ok := strings.Cut(kv, "="); ok && k == key {
			v = val
		}
	}
	return v
}

// stateDir is where the rehearsal tool keeps receipts: the override, else the
// tool's own default, computed once and passed to the child explicitly.
func stateDir(o Options, environ []string) string {
	var d string
	switch {
	case o.RehearseState != "":
		d = o.RehearseState
	case getenv(environ, "HANDOFF_REHEARSE_STATE") != "":
		d = getenv(environ, "HANDOFF_REHEARSE_STATE")
	default:
		base := getenv(environ, "XDG_STATE_HOME")
		if base == "" {
			base = filepath.Join(getenv(environ, "HOME"), ".local", "state")
		}
		d = filepath.Join(base, "handoff-rehearse")
	}
	if abs, err := filepath.Abs(d); err == nil {
		d = abs
	}
	return filepath.Clean(d)
}

var rehearsedRe = regexp.MustCompile(`^REHEARSED sha256=([0-9a-f]{64}) uid=([0-9]+) check_rc=(-?[0-9]+) log=(.+)$`)

func checkScript(ctx context.Context, o Options, env Env) ([]Item, string) {
	var items []Item
	snap, err := takeSnapshot(o.Script)
	if err != nil {
		return []Item{item("script.path", Fail, "%v", err)}, ""
	}
	items = append(items, item("script.path", Pass, "%s", snap.canon))

	want := strings.ToLower(o.SHA)
	if snap.digest == want {
		items = append(items, item("script.sha256", Pass, "file digest %s equals the given sha", snap.digest))
	} else {
		items = append(items, item("script.sha256", Fail, "file digest is %s, the given sha is %s", snap.digest, want))
	}

	items = append(items, lintItem(ctx, snap, env))

	if snap.mode&0o100 != 0 {
		items = append(items, item("script.exec_bit", Pass, "mode %v has the owner exec bit", snap.mode.Perm()))
	} else {
		items = append(items, item("script.exec_bit", Fail, "mode %v has no owner exec bit", snap.mode.Perm()))
	}

	items = append(items, rehearsedItem(ctx, o, env, snap))

	if again, err := takeSnapshot(o.Script); err != nil || !snap.same(again) {
		why := "the file differs from the one first read"
		if err != nil {
			why = err.Error()
		}
		items = append(items, item("script.stable", Fail, "the script changed while it was checked: %s; every script item is void", why))
		for i := range items {
			if items[i].ID != "script.stable" {
				items[i].Status = Fail
				items[i].Detail += " (void: script.changed)"
			}
		}
	} else {
		items = append(items, item("script.stable", Pass, "same file, digest and mode before and after"))
	}
	return items, snap.digest
}

// lintItem lints an immutable copy of the captured bytes, so the verdict is
// about those bytes even if the path changes while the linter runs.
func lintItem(ctx context.Context, s *snapshot, env Env) Item {
	dir, err := os.MkdirTemp(env.TempDir, "escheck-")
	if err != nil {
		return item("script.lint", Unknown, "cannot make a temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	cp := filepath.Join(dir, filepath.Base(s.canon))
	if err := os.WriteFile(cp, s.data, s.mode.Perm()); err != nil {
		return item("script.lint", Unknown, "cannot write the lint copy: %v", err)
	}
	if err := os.Chmod(cp, s.mode.Perm()); err != nil {
		return item("script.lint", Unknown, "cannot set the lint copy mode: %v", err)
	}
	res, err := env.Runner.Run(ctx, []string{"handoff-lint", cp}, childEnv(env.Environ()))
	if err != nil {
		return item("script.lint", Unknown, "handoff-lint did not run: %v", err)
	}
	switch {
	case res.TimedOut:
		return item("script.lint", Unknown, "handoff-lint timed out")
	case res.Code == 0:
		return item("script.lint", Pass, "handoff-lint exit 0 on a copy of the captured bytes")
	case res.Code == 1:
		return item("script.lint", Fail, "handoff-lint findings: %s", firstLines(res.Stdout+res.Stderr, 20))
	}
	return item("script.lint", Unknown, "handoff-lint exit %d: %s", res.Code, firstLines(res.Stderr, 5))
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "...")
	}
	return strings.Join(lines, "; ")
}

func rehearsedItem(ctx context.Context, o Options, env Env, s *snapshot) Item {
	const id = "script.rehearsed"
	environ := env.Environ()
	state := stateDir(o, environ)
	res, err := env.Runner.Run(ctx, []string{"handoff-rehearse", "--verify", s.canon}, childEnv(environ, "HANDOFF_REHEARSE_STATE="+state))
	if err != nil {
		return item(id, Unknown, "handoff-rehearse did not run: %v", err)
	}
	if res.TimedOut {
		return item(id, Unknown, "handoff-rehearse --verify timed out")
	}
	if res.Code == 1 {
		return item(id, Fail, "handoff-rehearse --verify: %s", firstLines(res.Stderr, 3))
	}
	if res.Code != 0 {
		return item(id, Unknown, "handoff-rehearse --verify exit %d", res.Code)
	}
	line, _, _ := strings.Cut(strings.TrimRight(res.Stdout, "\n"), "\n")
	m := rehearsedRe.FindStringSubmatch(line)
	if m == nil {
		return item(id, Unknown, "no REHEARSED line in the output")
	}
	if m[1] != s.digest {
		return item(id, Fail, "the REHEARSED line is for sha256 %s, the script is %s", m[1], s.digest)
	}
	logPath := m[4]
	if !filepath.IsAbs(logPath) {
		return item(id, Unknown, "the log path is not absolute")
	}
	recPath := filepath.Join(filepath.Dir(filepath.Clean(logPath)), "receipt.json")
	realState, err1 := filepath.EvalSymlinks(state)
	realRec, err2 := filepath.EvalSymlinks(recPath)
	if err1 != nil || err2 != nil {
		return item(id, Unknown, "cannot resolve the state directory or the receipt")
	}
	if !strings.HasPrefix(realRec, strings.TrimRight(realState, "/")+"/") {
		return item(id, Fail, "the receipt %s is outside the rehearsal state directory %s", realRec, realState)
	}
	b, _, err := readRegular(realRec, 64<<10)
	if err != nil {
		return item(id, Unknown, "cannot read the receipt: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var rec map[string]any
	if err := dec.Decode(&rec); err != nil || rec == nil {
		return item(id, Unknown, "the receipt is not a JSON object")
	}
	num := func(k string) (string, bool) {
		n, ok := rec[k].(json.Number)
		return n.String(), ok
	}
	if rec["sha256"] != s.digest || rec["script"] != s.canon {
		return item(id, Fail, "the receipt is for a different script or digest")
	}
	if rec["passed"] != true {
		return item(id, Fail, "the receipt does not say passed")
	}
	if rc, ok := num("check_rc"); !ok || rc != "0" {
		return item(id, Fail, "the receipt's check_rc is not 0")
	}
	if _, present := rec["lint_rc"]; present {
		if rc, ok := num("lint_rc"); !ok || rc != "0" {
			return item(id, Fail, "the receipt's lint_rc is not the number 0")
		}
	}
	if p, ok := rec["problems"].([]any); (ok && len(p) > 0) || (!ok && rec["problems"] != nil) {
		return item(id, Fail, "the receipt lists problems")
	}
	if u, ok := num("uid"); !ok || u != m[2] {
		return item(id, Fail, "the receipt's uid does not match the REHEARSED line")
	}
	it := item(id, Pass, "the rehearsal tool's recorded state for these bytes is consistent (state directory %s); this does not prove who wrote that state", realState)
	it.Data = map[string]any{"state_dir": realState}
	return it
}
