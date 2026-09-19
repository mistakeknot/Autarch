// Package agenttransport owns Autarch's local external-session boundary.
// It performs no model calls. Callers must durably claim a handoff before Send.
package agenttransport

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	Socket        string `json:"socket"`
	ServerPID     int64  `json:"server_pid"`
	ServerStarted int64  `json:"server_started"`
	SessionID     string `json:"session_id"`
	WindowID      string `json:"window_id"`
	PaneID        string `json:"pane_id"`
	PanePID       int64  `json:"pane_pid"`
	Command       string `json:"command"`
}

func (t Target) Clone() Target { return t }
func (t Target) Key() string {
	return fmt.Sprintf("%s/%d/%d/%s/%s/%s/%d", t.Socket, t.ServerPID, t.ServerStarted, t.SessionID, t.WindowID, t.PaneID, t.PanePID)
}

// SamePane includes the server incarnation and pane process, but not a linked
// window's session alias. Navigation still requires the complete Target.
func (t Target) SamePane(other Target) bool {
	return t.Socket == other.Socket && t.ServerPID == other.ServerPID && t.ServerStarted == other.ServerStarted && t.PaneID == other.PaneID && t.PanePID == other.PanePID
}

// PaneKey renders exactly the identity SamePane compares, in the form the
// registry's pane_binding.pane_key generated column computes. It lives beside
// SamePane so the two cannot drift, and registry pins it to the database by
// test. Note it omits SessionID: a session name or alias is an observation
// that changes under a rename, while this is identity.
func (t Target) PaneKey() string {
	return fmt.Sprintf("%s/%d/%d/%s/%d", t.Socket, t.ServerPID, t.ServerStarted, t.PaneID, t.PanePID)
}

func (t Target) address() string { return t.SessionID + ":" + t.WindowID + "." + t.PaneID }

var ids = regexp.MustCompile(`^[$@%][0-9]+$`)

func (t Target) Validate() error {
	if !filepath.IsAbs(t.Socket) || strings.ContainsAny(t.Socket, "\x00\n\r\x1f") || t.ServerPID <= 0 || t.ServerStarted <= 0 || t.PanePID <= 0 {
		return errors.New("incomplete tmux server/pane identity")
	}
	for i, s := range []string{t.SessionID, t.WindowID, t.PaneID} {
		if !ids.MatchString(s) || s[0] != "$@%"[i] {
			return errors.New("invalid tmux target IDs")
		}
	}
	return nil
}

type Pane struct {
	Target                                 Target
	SessionName, WindowName, Path, Command string
	Activity                               int64
	Dead                                   bool
}

func (p Pane) Clone() Pane { return p }

type Delivery string

const (
	NotSent   Delivery = "not_sent"
	InFlight  Delivery = "in_flight"
	Delivered Delivery = "delivered"
	Uncertain Delivery = "uncertain"
)

type Result struct {
	State Delivery
	Err   error
}
type Transport interface {
	List(context.Context) ([]Pane, error)
	Observe(context.Context, Target) (string, error)
	Send(context.Context, Target, string) Result
	Interrupt(context.Context, Target) Result
	Open(context.Context, Target) error
}

// Runner receives message bytes only through stdin, never through arguments.
type Runner interface {
	Run(context.Context, io.Reader, ...string) ([]byte, error)
}
type ExecRunner struct{}
type cappedOutput struct{ bytes.Buffer }

func (b *cappedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4<<20 {
		return 0, errors.New("tmux output exceeds local bound")
	}
	return b.Buffer.Write(p)
}
func (ExecRunner) Run(ctx context.Context, in io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "tmux", args...)
	c.Stdin = in
	c.WaitDelay = 500 * time.Millisecond
	var b cappedOutput
	c.Stdout = &b
	c.Stderr = &b
	e := c.Run()
	return b.Bytes(), e
}

type Tmux struct {
	runner Runner
	socket string
}

func NewTmux(r Runner, socket string) *Tmux {
	if r == nil {
		r = ExecRunner{}
	}
	return &Tmux{runner: r, socket: socket}
}

const PaneFormat = "#{socket_path}\x1f#{pid}\x1f#{start_time}\x1f#{session_id}\x1f#{window_id}\x1f#{pane_id}\x1f#{pane_pid}\x1f#{pane_dead}\x1f#{session_name}\x1f#{window_name}\x1f#{session_activity}\x1f#{pane_current_path}\x1f#{pane_current_command}"

func ParsePanes(out string) ([]Pane, error) {
	var panes []Pane
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		p := strings.Split(line, "\x1f")
		if len(p) == 1 {
			p = strings.Split(line, `\037`)
		}
		if len(p) != 13 {
			return nil, errors.New("tmux inventory has malformed fields")
		}
		nums := make([]int64, 4)
		for i, n := range []int{1, 2, 6, 10} {
			v, e := strconv.ParseInt(p[n], 10, 64)
			if e != nil || v < 0 {
				return nil, errors.New("invalid tmux numeric identity/activity")
			}
			nums[i] = v
		}
		target := Target{Socket: p[0], ServerPID: nums[0], ServerStarted: nums[1], SessionID: p[3], WindowID: p[4], PaneID: p[5], PanePID: nums[2], Command: p[12]}
		if e := target.Validate(); e != nil {
			return nil, e
		}
		if p[7] != "0" && p[7] != "1" {
			return nil, errors.New("invalid pane liveness")
		}
		if seen[target.Key()] {
			return nil, errors.New("duplicate pane identity")
		}
		seen[target.Key()] = true
		panes = append(panes, Pane{Target: target, SessionName: p[8], WindowName: p[9], Activity: nums[3], Path: p[11], Command: p[12], Dead: p[7] == "1"})
	}
	sort.Slice(panes, func(i, j int) bool { return panes[i].Target.Key() < panes[j].Target.Key() })
	return panes, nil
}
func (t *Tmux) list(ctx context.Context, socket string) ([]Pane, error) {
	args := []string{}
	if socket != "" {
		args = append(args, "-S", socket)
	}
	args = append(args, "list-panes", "-a", "-F", PaneFormat)
	out, e := t.runner.Run(ctx, nil, args...)
	if e != nil {
		msg := strings.TrimSpace(string(out))
		stopped := strings.HasPrefix(msg, "no server running on ") || (strings.HasPrefix(msg, "error connecting to ") && (strings.HasSuffix(msg, "(No such file or directory)") || strings.HasSuffix(msg, "(Connection refused)")))
		if ctx.Err() == nil && stopped {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux inventory: %w", e)
	}
	ps, e := ParsePanes(string(out))
	if e != nil {
		return nil, e
	}
	if socket != "" {
		for _, p := range ps {
			if p.Target.Socket != socket {
				return nil, errors.New("tmux socket identity changed")
			}
		}
	}
	return ps, nil
}
func (t *Tmux) List(ctx context.Context) ([]Pane, error) { return t.list(ctx, t.socket) }
func (t *Tmux) revalidate(ctx context.Context, target Target) error {
	return t.validate(ctx, target, true)
}
func (t *Tmux) validate(ctx context.Context, target Target, requireProcess bool) error {
	if e := target.Validate(); e != nil {
		return e
	}
	ps, e := t.list(ctx, target.Socket)
	if e != nil {
		return e
	}
	for _, p := range ps {
		if p.Target.Key() == target.Key() && !p.Dead && (!requireProcess || p.Target.Command == target.Command) {
			return nil
		}
	}
	return errors.New("original tmux pane exited, moved or was replaced; open the original pane")
}

// The guard is evaluated by tmux on its command queue, not by a shell. Only
// validated numeric IDs and generated buffer names enter its command strings.
// Revalidate again in the same queue as every operation that can send input.
func (target Target) guard() string {
	pairs := [][2]string{{"pid", strconv.FormatInt(target.ServerPID, 10)}, {"start_time", strconv.FormatInt(target.ServerStarted, 10)}, {"session_id", target.SessionID}, {"window_id", target.WindowID}, {"pane_id", target.PaneID}, {"pane_pid", strconv.FormatInt(target.PanePID, 10)}, {"pane_dead", "0"}}
	expr := "1"
	for _, p := range pairs {
		expr = "#{&&:" + expr + ",#{==:#{" + p[0] + "}," + p[1] + "}}"
	}
	return expr
}

var claudeVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func (target Target) agent() bool {
	return target.Provider() != ""
}
func (target Target) Provider() string {
	switch {
	case target.Command == "codex":
		return "codex"
	case target.Command == "claude" || claudeVersion.MatchString(target.Command):
		return "claude-code"
	default:
		return ""
	}
}
func (target Target) inputGuard(_ bool) string {
	expr := target.guard()
	// Only agent command spellings from this closed grammar enter tmux formats.
	if !target.agent() {
		return "0"
	}
	pairs := [][2]string{{"pane_current_command", target.Command}, {"pane_in_mode", "0"}, {"synchronize-panes", "0"}}
	for _, p := range pairs {
		expr = "#{&&:" + expr + ",#{==:#{" + p[0] + "}," + p[1] + "}}"
	}
	return expr
}
func (t *Tmux) guarded(ctx context.Context, target Target, command string) error {
	guard := target.guard()
	if strings.HasPrefix(command, "paste-buffer") {
		guard = target.inputGuard(true)
	} else if strings.HasPrefix(command, "send-keys") {
		guard = target.inputGuard(false)
	}
	out, e := t.runner.Run(ctx, nil, "-S", target.Socket, "if-shell", "-F", "-t", target.address(), guard, command+" ; display-message -p AUTARCH_OK", "display-message -p AUTARCH_TARGET_CHANGED")
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(out)) != "AUTARCH_OK" {
		return errors.New("tmux target guard failed")
	}
	return nil
}
func (t *Tmux) Observe(ctx context.Context, target Target) (string, error) {
	if e := t.validate(ctx, target, false); e != nil {
		return "", e
	}
	out, e := t.runner.Run(ctx, nil, "-S", target.Socket, "capture-pane", "-p", "-t", target.address(), "-S", "-80")
	if e != nil {
		return "", e
	}
	if e = t.validate(ctx, target, false); e != nil {
		return "", e
	}
	if len(out) > 32<<10 {
		out = out[len(out)-(32<<10):]
	}
	return string(out), nil
}
func (t *Tmux) Send(ctx context.Context, target Target, message string) Result {
	if !target.agent() {
		return Result{NotSent, errors.New("direct input requires an observed Claude Code or Codex process")}
	}
	if message == "" || len(message) > 1<<20 || strings.ContainsAny(message, "\x00\x1b\r") {
		return Result{NotSent, errors.New("message is empty, too large, or contains terminal control bytes")}
	}
	// Permit LF and tab, but never terminal control input disguised as text.
	for _, r := range message {
		if (r < 32 && r != '\n' && r != '\t') || r == 127 {
			return Result{NotSent, errors.New("message contains terminal control bytes")}
		}
	}
	if e := t.revalidate(ctx, target); e != nil {
		return Result{NotSent, e}
	}
	var random [24]byte
	if _, e := rand.Read(random[:]); e != nil {
		return Result{NotSent, e}
	}
	name := "autarch-" + hex.EncodeToString(random[:])
	// Cleanup uses a fresh bounded context even when the input operation times out.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		t.runner.Run(cleanup, nil, "-S", target.Socket, "delete-buffer", "-b", name)
	}()
	if _, e := t.runner.Run(ctx, strings.NewReader(message), "-S", target.Socket, "load-buffer", "-b", name, "-"); e != nil {
		return Result{NotSent, e}
	}
	if e := t.revalidate(ctx, target); e != nil {
		return Result{NotSent, e}
	}
	if e := t.guarded(ctx, target, "paste-buffer -r -p -b "+name+" -t '"+target.address()+"'"); e != nil {
		return Result{Uncertain, e}
	}
	if _, e := t.runner.Run(ctx, nil, "-S", target.Socket, "delete-buffer", "-b", name); e != nil {
		return Result{Uncertain, e}
	}
	if e := t.revalidate(ctx, target); e != nil {
		return Result{Uncertain, e}
	}
	if e := t.guarded(ctx, target, "send-keys -t '"+target.address()+"' Enter"); e != nil {
		return Result{Uncertain, e}
	}
	return Result{State: Delivered}
}
func (t *Tmux) Interrupt(ctx context.Context, target Target) Result {
	if !target.agent() {
		return Result{NotSent, errors.New("interrupt requires an observed Claude Code or Codex process")}
	}
	interruptCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	baseline, e := t.captureReadiness(interruptCtx, target)
	if e != nil {
		return Result{NotSent, e}
	}
	baselineEvidence, baselineReady := providerReadinessEvidence(target.Command, baseline)
	if e := t.guarded(interruptCtx, target, "send-keys -t '"+target.address()+"' C-c"); e != nil {
		return Result{Uncertain, e}
	}
	// C-c being accepted by tmux is not proof that the provider is ready for
	// replacement text. Wait for a native prompt on the unchanged process; an
	// uncertain observation leaves the draft for the user to inspect manually.
	stableEvidence, stableCount := "", 0
	for attempt := 0; attempt < 40; attempt++ {
		out, e := t.captureReadiness(interruptCtx, target)
		if e != nil {
			return Result{Uncertain, e}
		}
		evidence, ready := providerReadinessEvidence(target.Command, out)
		if ready && (!baselineReady || evidence != baselineEvidence) {
			if evidence == stableEvidence {
				stableCount++
			} else {
				stableEvidence, stableCount = evidence, 1
			}
			if stableCount >= 2 {
				return Result{State: Delivered}
			}
		} else {
			stableEvidence, stableCount = "", 0
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-interruptCtx.Done():
			timer.Stop()
			return Result{Uncertain, interruptCtx.Err()}
		case <-timer.C:
		}
	}
	return Result{Uncertain, errors.New("provider readiness was not observed after interrupt")}
}

func providerReadinessEvidence(command, output string) (string, bool) {
	evidence, ready := providerPromptEvidence(command, output)
	if !ready {
		return "", false
	}
	lower := strings.ToLower(evidence)
	for _, hint := range []string{"esc to interrupt", "escape to interrupt", "esc to cancel", "esc to stop", "ctrl+c to interrupt", "ctrl-c to interrupt"} {
		if strings.Contains(lower, hint) {
			return "", false
		}
	}
	return evidence, true
}

func (t *Tmux) captureReadiness(ctx context.Context, target Target) (string, error) {
	if e := t.revalidate(ctx, target); e != nil {
		return "", e
	}
	out, e := t.runner.Run(ctx, nil, "-S", target.Socket, "capture-pane", "-p", "-t", target.address(), "-S", "-24")
	if e != nil {
		return "", e
	}
	if e = t.revalidate(ctx, target); e != nil {
		return "", e
	}
	return string(out), nil
}

func providerPromptEvidence(command, output string) (string, bool) {
	marker := ""
	switch {
	case command == "codex":
		marker = "›"
	case command == "claude" || claudeVersion.MatchString(command):
		marker = "❯"
	default:
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	start := len(lines) - 8
	if start < 0 {
		start = 0
	}
	for i := len(lines) - 1; i >= start; i-- {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == marker || strings.HasPrefix(trimmed, marker+" ") {
			// Provider status bars can sit several rows above the editable prompt.
			// Include the complete bounded tail so a changing busy indicator cannot
			// make an old prompt look newly ready after C-c.
			return strings.Join(lines[start:], "\n"), true
		}
	}
	return "", false
}

// Open switches the invoking tmux client only on its own server. Outside tmux
// the UI must suspend itself and run AttachCommand with the user's terminal.
func (t *Tmux) Open(ctx context.Context, target Target) error {
	if e := t.validate(ctx, target, false); e != nil {
		return e
	}
	current := strings.Split(os.Getenv("TMUX"), ",")
	if len(current) < 2 || current[0] != target.Socket {
		return errors.New("attach original pane in a terminal using AttachCommand")
	}
	return t.guarded(ctx, target, "select-window -t '"+target.SessionID+":"+target.WindowID+"' ; select-pane -t '"+target.address()+"' ; switch-client -t '"+target.address()+"'")
}
func (t *Tmux) AttachCommand(ctx context.Context, target Target) (*exec.Cmd, error) {
	if e := t.validate(ctx, target, false); e != nil {
		return nil, e
	}
	command := "select-window -t '" + target.SessionID + ":" + target.WindowID + "' ; select-pane -t '" + target.address() + "' ; attach-session -t '" + target.SessionID + "'"
	return exec.Command("tmux", "-S", target.Socket, "if-shell", "-F", "-t", target.address(), target.guard(), command, "run-shell 'exit 73'"), nil
}
