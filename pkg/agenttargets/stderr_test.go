package agenttargets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStreamingStderr(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		for _, large := range []bool{false, true} {
			t.Run(agent+map[bool]string{false: "/short", true: "/tail"}[large], func(t *testing.T) {
				dir := t.TempDir()
				text := "authentication failed"
				if large {
					text = "discard-me" + strings.Repeat("x", 128*1024) + "quota exceeded"
				}
				script := "#!/bin/sh\nprintf '%s' '" + text + "' >&2\nexit 1\n"
				if err := os.WriteFile(filepath.Join(dir, agent), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir)
				h, err := Dispatch(context.Background(), DispatchConfig{PreferredAgent: agent, Timeout: 5 * time.Second}, dir, "test")
				if err != nil {
					t.Fatal(err)
				}
				for range h.Events {
				}
				_, err = h.Wait()
				if err == nil {
					t.Fatal("expected process error with stderr")
				}
				want := text
				if large {
					want = text[len(text)-4096:]
				}
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("missing stderr tail in error: %.200s", err)
				}
				if large && (strings.Contains(err.Error(), "discard-me") || len(err.Error()) > 4300) {
					t.Fatal("stderr was not bounded")
				}
			})
		}
	}
}

func TestStreamingStderrTimeoutAndSuccess(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		for _, timeout := range []bool{false, true} {
			t.Run(agent+map[bool]string{false: "/success", true: "/timeout"}[timeout], func(t *testing.T) {
				dir := t.TempDir()
				script := "#!/bin/sh\nprintf 'diagnostic' >&2\n"
				if timeout {
					script += "while :; do :; done\n"
				} else if agent == "claude" {
					script += "printf '%s\\n' '{\"type\":\"result\",\"result\":\"ok\"}'\n"
				}
				if err := os.WriteFile(filepath.Join(dir, agent), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir)
				limit := 5 * time.Second
				if timeout {
					limit = 200 * time.Millisecond
				}
				h, err := Dispatch(context.Background(), DispatchConfig{PreferredAgent: agent, Timeout: limit}, dir, "test")
				if err != nil {
					t.Fatal(err)
				}
				_, _, err = CollectResult(h)
				if timeout {
					if err == nil || !strings.Contains(err.Error(), "diagnostic") {
						t.Fatalf("missing timeout diagnostic: %v", err)
					}
					result, _ := h.Wait()
					if !result.TimedOut {
						t.Fatal("expected timeout")
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestStreamingInheritedStderr(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		for _, tc := range []struct {
			name    string
			timeout bool
			limit   time.Duration
		}{
			{"exit", false, 0}, {"exit-before-deadline", false, 300 * time.Millisecond}, {"timeout", true, 300 * time.Millisecond},
		} {
			timeout := tc.timeout
			t.Run(agent+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				script := "#!/bin/sh\nprintf diagnostic >&2\n/bin/sleep 8 >/dev/null </dev/null &\n"
				if timeout {
					script += "while :; do :; done\n"
				} else {
					script += "exit 1\n"
				}
				if err := os.WriteFile(filepath.Join(dir, agent), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir)
				start := time.Now()
				h, err := Dispatch(context.Background(), DispatchConfig{PreferredAgent: agent, Timeout: tc.limit}, dir, "test")
				if err != nil {
					t.Fatal(err)
				}
				for range h.Events {
				}
				result, err := h.Wait()
				if time.Since(start) > 3*time.Second {
					t.Errorf("Wait took %s", time.Since(start))
				}
				if result.TimedOut != timeout {
					t.Errorf("TimedOut=%v, want %v", result.TimedOut, timeout)
				}
				if err == nil || !strings.Contains(err.Error(), "diagnostic") {
					t.Fatalf("missing diagnostic: %v", err)
				}
			})
		}
	}
}

func TestStreamingErrorResultPrecedence(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"is_error\":true,\"result\":\"Invalid API key - please run /login\"}'\nprintf diagnostic >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	h, err := Dispatch(context.Background(), DispatchConfig{PreferredAgent: "claude"}, dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = CollectResult(h)
	if err == nil || !strings.HasPrefix(err.Error(), "agent returned error: Invalid API key") || !strings.Contains(err.Error(), "diagnostic") {
		t.Fatalf("lost result diagnostic: %v", err)
	}
}

func TestStderrSanitized(t *testing.T) {
	var tail stderrTail
	tail.Write([]byte("\x1b[31merror\x1b[0m\r sk-secret123 Bearer secret456\x00"))
	err := processError(context.Canceled, false, &tail)
	for _, bad := range []string{"\x1b", "\r", "\x00", "sk-secret123", "secret456"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("unsafe error: %q", err)
		}
	}
}
