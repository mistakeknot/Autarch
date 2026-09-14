package agenttransport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

const fixture = "/tmp/autarch.sock\x1f42\x1f1700000000\x1f$1\x1f@2\x1f%3\x1f100\x1f0\x1fwork\x1feditor\x1f12345\x1f/est/a\x1fcodex\n"

type call struct {
	args  []string
	input string
}
type fakeRunner struct {
	calls        []call
	listing      string
	fail         string
	changed      bool
	notReady     bool
	staleReady   bool
	changingBusy bool
	captures     int
}

func (f *fakeRunner) Run(_ context.Context, input io.Reader, args ...string) ([]byte, error) {
	c := call{args: append([]string(nil), args...)}
	if input != nil {
		b, _ := io.ReadAll(input)
		c.input = string(b)
	}
	f.calls = append(f.calls, c)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, f.fail) && f.fail != "" {
		return nil, errors.New("injected failure")
	}
	if strings.Contains(joined, "list-panes") {
		return []byte(f.listing), nil
	}
	if strings.Contains(joined, "capture-pane") {
		f.captures++
		if f.changingBusy {
			return []byte(fmt.Sprintf("esc to interrupt\nstatus row\nstatus row\nstatus row\n› old prompt\nworking %d seconds\n", f.captures)), nil
		}
		if f.notReady {
			return []byte("still working\n"), nil
		}
		if f.staleReady || f.captures > 1 {
			return []byte("status\n› \n"), nil
		}
		return []byte("still working\n"), nil
	}
	if strings.Contains(joined, "if-shell") {
		if f.changed {
			return []byte("AUTARCH_TARGET_CHANGED\n"), nil
		}
		return []byte("AUTARCH_OK\n"), nil
	}
	return nil, nil
}

func TestInterruptRequiresObservedProviderReadiness(t *testing.T) {
	f := &fakeRunner{listing: fixture, notReady: true}
	result := NewTmux(f, "").Interrupt(context.Background(), target(t))
	if result.State != Uncertain || result.Err == nil || !strings.Contains(result.Err.Error(), "readiness") {
		t.Fatalf("unobserved readiness was treated as safe: %+v", result)
	}
}

func TestInterruptRejectsPromptThatPredatesStopInteraction(t *testing.T) {
	f := &fakeRunner{listing: fixture, staleReady: true}
	result := NewTmux(f, "").Interrupt(context.Background(), target(t))
	if result.State != Uncertain {
		t.Fatalf("stale prompt was treated as post-interrupt readiness: %+v", result)
	}
}

func TestInterruptRejectsChangingBusyStatusAroundOldPrompt(t *testing.T) {
	f := &fakeRunner{listing: fixture, changingBusy: true}
	result := NewTmux(f, "").Interrupt(context.Background(), target(t))
	if result.State != Uncertain {
		t.Fatalf("changing busy status was treated as provider readiness: %+v", result)
	}
}
func target(t *testing.T) Target {
	t.Helper()
	ps, e := ParsePanes(fixture)
	if e != nil {
		t.Fatal(e)
	}
	return ps[0].Target
}

func TestInventoryAllPanesAndStrictIdentity(t *testing.T) {
	other := strings.ReplaceAll(strings.ReplaceAll(fixture, "%3", "%4"), "@2", "@5")
	ps, e := ParsePanes(fixture + other)
	if e != nil || len(ps) != 2 || ps[0].Target == ps[1].Target {
		t.Fatalf("panes=%+v err=%v", ps, e)
	}
	for _, bad := range []string{strings.Replace(fixture, "$1", "work", 1), strings.Replace(fixture, "42", "x", 1), strings.Replace(fixture, "1700000000", "", 1), strings.Replace(fixture, "%3", "%3; kill-server", 1)} {
		if _, e := ParsePanes(bad); e == nil {
			t.Fatalf("accepted invalid identity %q", bad)
		}
	}
	escaped := strings.ReplaceAll(fixture, "\x1f", `\037`)
	if ps, e := ParsePanes(escaped); e != nil || len(ps) != 1 {
		t.Fatalf("escaped: %v", e)
	}
}

func TestRevalidationRenamedExitedReplaced(t *testing.T) {
	for _, tc := range []struct {
		name, listing string
		ok            bool
	}{
		{"renamed", strings.Replace(fixture, "work", "renamed", 1), true},
		{"exited", "", false}, {"new server", strings.Replace(fixture, "1700000000", "1700000001", 1), false},
		{"respawn", strings.Replace(fixture, "100\x1f", "101\x1f", 1), false},
		{"moved", strings.Replace(fixture, "@2", "@9", 1), false},
		{"dead", strings.Replace(fixture, "\x1f0\x1f", "\x1f1\x1f", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRunner{listing: tc.listing}
			tr := NewTmux(f, "")
			r := tr.Interrupt(context.Background(), target(t))
			if (r.State == Delivered) != tc.ok {
				t.Fatalf("%+v", r)
			}
			if !tc.ok {
				for _, c := range f.calls {
					if strings.Contains(strings.Join(c.args, " "), "send-keys") {
						t.Fatal("input to invalid pane")
					}
				}
			}
		})
	}
}

func TestLiteralMultilineAndUncertainBoundary(t *testing.T) {
	payload := "hello\n$(touch /tmp/never) `whoami` ' \" \\ % ;\nlast"
	for _, fail := range []string{"", "load-buffer", "paste-buffer", "delete-buffer", "send-keys"} {
		t.Run(fail, func(t *testing.T) {
			f := &fakeRunner{listing: fixture, fail: fail}
			tr := NewTmux(f, "")
			r := tr.Send(context.Background(), target(t), payload)
			want := Delivered
			if fail == "load-buffer" {
				want = NotSent
			} else if fail != "" {
				want = Uncertain
			}
			if r.State != want {
				t.Fatalf("%+v want %s", r, want)
			}
			found := false
			for _, c := range f.calls {
				if strings.Contains(strings.Join(c.args, " "), payload) {
					t.Fatal("payload in argv")
				}
				if c.input == payload {
					found = true
				}
			}
			if !found {
				t.Fatal("payload not passed verbatim on stdin")
			}
		})
	}
}

func TestInputGuardAndBoundedPreview(t *testing.T) {
	f := &fakeRunner{listing: fixture, changed: true}
	tr := NewTmux(f, "")
	r := tr.Send(context.Background(), target(t), "draft")
	if r.State != Uncertain {
		t.Fatalf("guard failure after paste attempt: %+v", r)
	}
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c.args, " "), "send-keys") {
			t.Fatal("submitted after guard failure")
		}
	}
	f.changed = false
	if _, e := tr.Observe(context.Background(), target(t)); e != nil {
		t.Fatal(e)
	}
	bounded := false
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c.args, " "), "capture-pane") && strings.Contains(strings.Join(c.args, " "), "-S -80") {
			bounded = true
		}
	}
	if !bounded {
		t.Fatal("unbounded preview")
	}
}
func TestOriginalPaneCanBeOpenedAfterAgentExit(t *testing.T) {
	f := &fakeRunner{listing: strings.Replace(fixture, "codex", "zsh", 1)}
	tr := NewTmux(f, "")
	t.Setenv("TMUX", "/tmp/autarch.sock,42,1")
	if e := tr.Open(context.Background(), target(t)); e != nil {
		t.Fatalf("could not open original pane after agent exit: %v", e)
	}
}

func TestAttachCommandFailsWhenExecutionTimeGuardChanges(t *testing.T) {
	f := &fakeRunner{listing: fixture}
	command, err := NewTmux(f, "").AttachCommand(context.Background(), target(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(command.Args, " "); !strings.Contains(got, "run-shell 'exit 73'") {
		t.Fatalf("attach fallback would exit successfully: %s", got)
	}
}

func TestAgentExitAndUnsafePasteModeFailClosed(t *testing.T) {
	f := &fakeRunner{listing: strings.Replace(fixture, "codex", "zsh", 1)}
	tr := NewTmux(f, "")
	if r := tr.Send(context.Background(), target(t), "message"); r.State != NotSent {
		t.Fatalf("sent after agent exited: %+v", r)
	}
	f = &fakeRunner{listing: fixture}
	tr = NewTmux(f, "")
	tr.Send(context.Background(), target(t), "hello\nworld")
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c.args, " "), "paste-buffer") {
			guard := strings.Join(c.args, " ")
			for _, required := range []string{"pane_current_command", "synchronize-panes", "pane_in_mode"} {
				if !strings.Contains(guard, required) {
					t.Errorf("missing %s input guard", required)
				}
			}
		}
	}
}
