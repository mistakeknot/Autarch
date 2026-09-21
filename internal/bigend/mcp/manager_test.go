package mcp

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestManagerStartStopIdempotent(t *testing.T) {
	m := NewManager()
	if err := m.Stop("/root/projects/demo", "server"); err != nil {
		t.Fatalf("stop should be idempotent: %v", err)
	}
}

type fakeProcess struct {
	pid    int
	stdout chan string
	stderr chan string
	waitCh chan struct{}
}

func newFakeProcess(pid int) *fakeProcess {
	return &fakeProcess{
		pid:    pid,
		stdout: make(chan string, 64),
		stderr: make(chan string, 64),
		waitCh: make(chan struct{}),
	}
}

func (p *fakeProcess) Pid() int              { return p.pid }
func (p *fakeProcess) Stdout() <-chan string { return p.stdout }
func (p *fakeProcess) Stderr() <-chan string { return p.stderr }
func (p *fakeProcess) Stop() error           { close(p.waitCh); return nil }
func (p *fakeProcess) Wait() error           { <-p.waitCh; return nil }

type fakeRunner struct {
	process *fakeProcess
	starts  int
}

func (r *fakeRunner) Start(ctx context.Context, cmd []string, workdir string) (Process, error) {
	if r.process == nil {
		return nil, fmt.Errorf("no process")
	}
	r.starts++
	return r.process, nil
}

func TestManagerStartIdempotent(t *testing.T) {
	proc := newFakeProcess(7)
	runner := &fakeRunner{process: proc}
	m := NewManagerWithRunner(runner)

	if err := m.Start(context.Background(), "/root/projects/demo", "server", []string{"node", "server.js"}, ""); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if err := m.Start(context.Background(), "/root/projects/demo", "server", []string{"node", "server.js"}, ""); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	if runner.starts != 1 {
		t.Fatalf("expected 1 start, got %d", runner.starts)
	}

	_ = proc.Stop()
	close(proc.stdout)
	close(proc.stderr)
}

func TestManagerLogTailUpdates(t *testing.T) {
	proc := newFakeProcess(42)
	runner := &fakeRunner{process: proc}
	m := NewManagerWithRunner(runner)

	if err := m.Start(context.Background(), "/root/projects/demo", "server", []string{"node", "server.js"}, ""); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	for i := 0; i < 60; i++ {
		proc.stdout <- fmt.Sprintf("line-%02d", i)
	}
	close(proc.stdout)

	waitFor(t, func() bool {
		status := m.Status("/root/projects/demo", "server")
		if status == nil || len(status.LogTail) == 0 {
			return false
		}
		return status.LogTail[len(status.LogTail)-1] == "line-59"
	})

	status := m.Status("/root/projects/demo", "server")
	if status == nil {
		t.Fatalf("expected status")
	}
	if len(status.LogTail) > 50 {
		t.Fatalf("expected log tail capped at 50, got %d", len(status.LogTail))
	}
	if got := status.LogTail[len(status.LogTail)-1]; got != "line-59" {
		t.Fatalf("unexpected last line: %q", got)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met in time")
}
