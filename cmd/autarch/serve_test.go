package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// startServe runs `autarch serve` with args and returns the bound address once
// the startup line appears. The returned stop cancels it and waits for exit.
func startServe(t *testing.T, args ...string) (addr string, stop func() error) {
	t.Helper()
	cmd := serveCmd()
	out := &syncBuffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- cmd.ExecuteContext(ctx) }()
	re := regexp.MustCompile(`listening on (127\.0\.0\.1:\d+)`)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m := re.FindStringSubmatch(out.String()); m != nil {
			return m[1], func() error {
				cancel()
				select {
				case err := <-errc:
					return err
				case <-time.After(10 * time.Second):
					return fmt.Errorf("serve did not exit")
				}
			}
		}
		select {
		case err := <-errc:
			cancel()
			t.Fatalf("serve exited early: %v\n%s", err, out.String())
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	t.Fatalf("no startup line:\n%s", out.String())
	return "", nil
}

func TestServe_RejectsNonLoopback(t *testing.T) {
	cmd := serveCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--addr", "0.0.0.0:0", "--token-file", filepath.Join(t.TempDir(), "t"), "--project-dir", t.TempDir()})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "local") {
		t.Fatalf("expected local-only error, got %v", err)
	}
}

func TestServe_StartsAndStops(t *testing.T) {
	tok := filepath.Join(t.TempDir(), "serve.token")
	addr, stop := startServe(t, "--addr", "127.0.0.1:0", "--token-file", tok, "--project-dir", t.TempDir())
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("health never became ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestServe_ProjectDirParentResolvesChildNotParent(t *testing.T) {
	parent := t.TempDir()
	proj := filepath.Join(parent, "P")
	if err := os.MkdirAll(filepath.Join(proj, ".gurgeh", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	tok := filepath.Join(t.TempDir(), "serve.token")
	addr, stop := startServe(t, "--addr", "127.0.0.1:0", "--token-file", tok, "--project-dir", parent)
	defer stop()

	token, err := os.ReadFile(tok)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "http://"+addr+"/api/projects", nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list []struct{ Name, Root string }
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	wantRoot, _ := filepath.EvalSymlinks(proj)
	parentRoot, _ := filepath.EvalSymlinks(parent)
	if len(list) != 1 || list[0].Name != "P" || list[0].Root != wantRoot {
		t.Fatalf("want only P at %s, got %+v", wantRoot, list)
	}
	for _, p := range list {
		if p.Root == parentRoot {
			t.Fatalf("parent resolved as a project: %+v", p)
		}
	}
}
