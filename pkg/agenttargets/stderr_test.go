package agenttargets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestStreamingStderr(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		for _, large := range []bool{false, true} {
			t.Run(agent+map[bool]string{false: "/short", true: "/tail"}[large], func(t *testing.T) {
				dir := t.TempDir()
				text := "authentication failed"
				if large {
					text = "discard-me" + strings.Repeat("x", 128*1024) + "\nquota exceeded"
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
					want = "quota exceeded"
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
			{"success", false, 0}, {"exit", false, 0}, {"exit-before-deadline", false, 300 * time.Millisecond}, {"timeout", true, 300 * time.Millisecond},
		} {
			timeout := tc.timeout
			t.Run(agent+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				script := "#!/bin/sh\nprintf diagnostic >&2\n/bin/sleep 8 >/dev/null </dev/null &\n"
				if tc.name == "success" {
					if agent == "claude" {
						script += "printf '%s\\n' '{\"type\":\"result\",\"result\":\"ok\"}'\n"
					}
					script += "exit 0\n"
				} else if timeout {
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
				var output string
				var collectErr error
				if tc.name == "success" {
					output, _, collectErr = CollectResult(h)
				} else {
					for range h.Events {
					}
				}
				result, err := h.Wait()
				if time.Since(start) > 3*time.Second {
					t.Errorf("Wait took %s", time.Since(start))
				}
				if result.TimedOut != timeout {
					t.Errorf("TimedOut=%v, want %v", result.TimedOut, timeout)
				}
				if tc.name == "success" {
					if err != nil || collectErr != nil || (agent == "claude" && output != "ok") {
						t.Fatalf("successful result lost: output=%q err=%v collectErr=%v", output, err, collectErr)
					}
				} else if err == nil || !strings.Contains(err.Error(), "diagnostic") {
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
	tail.Write([]byte("\x1b[31merror\x1b[0m\r sk-xxxxxxxxxxxx Bearer secret456\x00"))
	err := processError(context.Canceled, false, &tail)
	for _, bad := range []string{"\x1b", "\r", "\x00", "sk-xxxxxxxxxxxx", "secret456"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("unsafe error: %q", err)
		}
	}
}

func TestSanitizedStderrUTF8AndWords(t *testing.T) {
	text := sanitizedStderr([]byte(strings.Repeat("é", 2100) + "x"))
	if !utf8.ValidString(text) || len(text) > stderrTailBytes {
		t.Errorf("invalid bounded UTF-8: len=%d valid=%v", len(text), utf8.ValidString(text))
	}
	const words = "disk-full; task-runner"
	if got := sanitizedStderr([]byte(words)); got != words {
		t.Errorf("ordinary words changed: %q", got)
	}
}

func TestStreamingNonErrorResultWithFailedExit(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"result\":\"ok\"}'\nprintf diagnostic >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	h, err := Dispatch(context.Background(), DispatchConfig{PreferredAgent: "claude"}, dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := CollectResult(h)
	if err == nil || !strings.Contains(err.Error(), "diagnostic") || out != "" {
		t.Fatalf("failed exit accepted: out=%q err=%v", out, err)
	}
}

func TestAdditionalSecretShapes(t *testing.T) {
	for _, input := range []string{"ghp_xxxxxxxxxxxx", "gho_xxxxxxxxxxxx", "github_pat_xxxxxxxxxxxx", `{"access_token":"xxxxxxxxxxxx"}`, `{"api_key": "xxxxxxxxxxxx"}`, `{"token":"xxxxxxxxxxxx"}`, "Authorization: Basic xxxxxxxxxxxx", "Bearer xxxxxxxxxxxx", "sk-xxxxxxxxxxxx"} {
		t.Run(input, func(t *testing.T) {
			if got := sanitizedStderr([]byte(input)); strings.Contains(got, "xxxxxxxxxxxx") {
				t.Fatalf("secret survived: %s", got)
			}
		})
	}
	var tail stderrTail
	tail.Write([]byte("ghp_" + strings.Repeat("a", 5000) + "\nuseful diagnostic"))
	if got := sanitizedStderr(tail.data); strings.Contains(got, "aaaa") || !strings.Contains(got, "useful diagnostic") {
		t.Fatalf("partial token survived: %.100s", got)
	}
}

func TestCodexTranscriptRedactedInError(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'transcript context ghp_xxxxxxxxxxxx {\"access_token\":\"xxxxxxxxxxxx\"}' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	h, err := Dispatch(context.Background(), DispatchConfig{PreferredAgent: "codex"}, dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	for range h.Events {
	}
	_, err = h.Wait()
	if err == nil || !strings.Contains(err.Error(), "transcript context") || strings.Contains(err.Error(), "xxxxxxxxxxxx") {
		t.Fatalf("unexpected transcript error: %v", err)
	}
}

func TestMoreSecretShapes(t *testing.T) {
	for _, input := range []string{"ghs_xxxxxxxxxxxx", "ghu_xxxxxxxxxxxx", "ghr_xxxxxxxxxxxx", "x-api-key: xxxxxxxxxxxx", "https://example.test/?access_token=xxxxxxxxxxxx&ok=yes", `{"refresh_token":"xxxxxxxxxxxx"}`, `{"id_token":"xxxxxxxxxxxx"}`, `{"client_secret":"xxxxxxxxxxxx"}`, `{"apiKey":"xxxxxxxxxxxx"}`, `{"token":"xxxxxxxxxxxx`} {
		t.Run(input, func(t *testing.T) {
			if got := sanitizedStderr([]byte(input)); strings.Contains(got, "xxxxxxxxxxxx") {
				t.Fatalf("unredacted: %s", got)
			}
		})
	}
}
func TestTailCutInsideKeywords(t *testing.T) {
	for _, key := range []string{`"access_token"`, `"api_key"`, `"token"`, "Authorization", "Bearer", `"refresh_token"`, `"id_token"`, `"client_secret"`, `"apiKey"`, "x-api-key", "access_token="} {
		for offset := 0; offset < len(key); offset++ {
			suffix := key[offset:] + `: "xxxxxxxxxxxx"` + "\nuseful diagnostic\n"
			input := strings.Repeat("a", 5000) + suffix + strings.Repeat(" ", stderrTailBytes-len(suffix))
			var tail stderrTail
			tail.Write([]byte(input))
			if got := sanitizedStderr(tail.data); strings.Contains(got, "xxxxxxxxxxxx") || !strings.Contains(got, "useful diagnostic") {
				t.Fatalf("key %q cut %d leaked: %.100s", key, offset, got)
			}
		}
	}
}
