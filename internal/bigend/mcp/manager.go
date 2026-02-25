package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Status represents the current MCP component status.
type Status string

const (
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
	StatusError   Status = "error"
)

// ComponentStatus tracks MCP component state.
type ComponentStatus struct {
	ProjectPath string
	Component   string
	Status      Status
	Pid         int
	StartedAt   time.Time
	LastError   string
	LogTail     []string
}

// Process represents a running MCP process.
type Process interface {
	Pid() int
	Stdout() <-chan string
	Stderr() <-chan string
	Wait() error
	Stop() error
}

// Runner starts MCP processes.
type Runner interface {
	Start(ctx context.Context, cmd []string, workdir string) (Process, error)
}

type managedProcess struct {
	status *ComponentStatus
	proc   Process
}

// Manager supervises MCP processes.
type Manager struct {
	mu     sync.RWMutex
	items  map[string]*managedProcess
	runner Runner
}

// NewManager creates a new MCP manager.
func NewManager() *Manager {
	return &Manager{items: make(map[string]*managedProcess)}
}

// NewManagerWithRunner creates a manager with an injected runner (for tests).
func NewManagerWithRunner(r Runner) *Manager {
	m := NewManager()
	m.runner = r
	return m
}

func key(project, component string) string {
	return project + "::" + component
}

// Stop marks a component as stopped. It is idempotent.
func (m *Manager) Stop(project, component string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(project, component)
	item, ok := m.items[k]
	if !ok {
		m.items[k] = &managedProcess{status: &ComponentStatus{ProjectPath: project, Component: component, Status: StatusStopped}}
		return nil
	}
	if item.proc != nil {
		_ = item.proc.Stop()
		item.proc = nil
	}
	item.status.Status = StatusStopped
	item.status.Pid = 0
	return nil
}

// StopAll stops all managed MCP processes.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.items {
		if item.proc != nil {
			_ = item.proc.Stop()
			item.proc = nil
		}
		if item.status != nil {
			item.status.Status = StatusStopped
			item.status.Pid = 0
		}
	}
}

// Start starts a component process and begins log tailing.
func (m *Manager) Start(ctx context.Context, project, component string, cmd []string, workdir string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("missing command")
	}
	m.mu.RLock()
	if existing, ok := m.items[key(project, component)]; ok && existing.status != nil {
		if existing.status.Status == StatusRunning && existing.proc != nil {
			m.mu.RUnlock()
			return nil
		}
	}
	m.mu.RUnlock()
	if m.runner == nil {
		m.runner = &execRunner{}
	}

	proc, err := m.runner.Start(ctx, cmd, workdir)
	status := &ComponentStatus{
		ProjectPath: project,
		Component:   component,
		Status:      StatusRunning,
		Pid:         0,
		StartedAt:   time.Now(),
	}
	if err != nil {
		status.Status = StatusError
		status.LastError = err.Error()
		m.mu.Lock()
		m.items[key(project, component)] = &managedProcess{status: status}
		m.mu.Unlock()
		return err
	}
	status.Pid = proc.Pid()

	m.mu.Lock()
	m.items[key(project, component)] = &managedProcess{status: status, proc: proc}
	m.mu.Unlock()

	m.consumeLogs(project, component, proc.Stdout())
	m.consumeLogs(project, component, proc.Stderr())

	go func() {
		if waitErr := proc.Wait(); waitErr != nil {
			m.setError(project, component, waitErr)
			return
		}
		m.setStatus(project, component, StatusStopped)
	}()

	return nil
}

// Status returns a snapshot of the component status.
func (m *Manager) Status(project, component string) *ComponentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.items[key(project, component)]
	if !ok || item.status == nil {
		return nil
	}
	clone := *item.status
	if item.status.LogTail != nil {
		clone.LogTail = append([]string(nil), item.status.LogTail...)
	}
	return &clone
}

func (m *Manager) setStatus(project, component string, status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[key(project, component)]
	if !ok || item.status == nil {
		return
	}
	item.status.Status = status
	if status != StatusRunning {
		item.status.Pid = 0
	}
}

func (m *Manager) setError(project, component string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[key(project, component)]
	if !ok || item.status == nil {
		return
	}
	item.status.Status = StatusError
	item.status.LastError = err.Error()
	item.status.Pid = 0
}

func (m *Manager) consumeLogs(project, component string, lines <-chan string) {
	go func() {
		for line := range lines {
			m.appendLog(project, component, line)
		}
	}()
}

func (m *Manager) appendLog(project, component, line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[key(project, component)]
	if !ok || item.status == nil {
		return
	}
	item.status.LogTail = append(item.status.LogTail, line)
	if len(item.status.LogTail) > 50 {
		item.status.LogTail = item.status.LogTail[len(item.status.LogTail)-50:]
	}
}

type execRunner struct{}

func (r *execRunner) Start(ctx context.Context, cmd []string, workdir string) (Process, error) {
	if len(cmd) == 0 {
		return nil, fmt.Errorf("missing command")
	}
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	if workdir != "" {
		c.Dir = workdir
	}

	stdoutPipe, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := c.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := c.Start(); err != nil {
		return nil, err
	}

	p := &execProcess{
		cmd:    c,
		stdout: make(chan string, 64),
		stderr: make(chan string, 64),
	}

	go scanLines(stdoutPipe, p.stdout)
	go scanLines(stderrPipe, p.stderr)

	return p, nil
}

type execProcess struct {
	cmd    *exec.Cmd
	stdout chan string
	stderr chan string
}

func (p *execProcess) Pid() int              { return p.cmd.Process.Pid }
func (p *execProcess) Stdout() <-chan string { return p.stdout }
func (p *execProcess) Stderr() <-chan string { return p.stderr }
func (p *execProcess) Wait() error           { return p.cmd.Wait() }
func (p *execProcess) Stop() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func scanLines(pipe io.ReadCloser, out chan<- string) {
	defer close(out)
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		out <- scanner.Text()
	}
}
