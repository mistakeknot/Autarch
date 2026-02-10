// Package intermute provides Intermute server management for Autarch.
package intermute

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Manager handles Intermute server lifecycle - detecting existing servers
// or spawning standalone ones as needed.
type Manager struct {
	host    string
	port    int
	dataDir string
	cmd     *exec.Cmd
	started bool
}

// Config for the Intermute manager
type Config struct {
	Host    string // Default: 127.0.0.1
	Port    int    // Default: 7338
	DataDir string // Default: ~/.autarch
}

// NewManager creates a new Intermute manager
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 7338
	}
	if cfg.DataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cfg.DataDir = filepath.Join(home, ".autarch")
	}

	return &Manager{
		host:    cfg.Host,
		port:    cfg.Port,
		dataDir: cfg.DataDir,
	}, nil
}

// URL returns the Intermute server URL
func (m *Manager) URL() string {
	return fmt.Sprintf("http://%s:%d", m.host, m.port)
}

// EnsureRunning checks if Intermute is already running, and if not, starts it.
// Returns a cleanup function that should be called on shutdown.
func (m *Manager) EnsureRunning(ctx context.Context) (func(), error) {
	// Check if server is already running
	if m.isHealthy() {
		// Existing server found, no cleanup needed
		return func() {}, nil
	}

	// No server running, start our own
	if err := m.start(ctx); err != nil {
		return nil, fmt.Errorf("start intermute: %w", err)
	}

	// Return cleanup function that stops our managed server
	return func() {
		m.stop()
	}, nil
}

// isHealthy checks if an Intermute server is responding at the configured address
func (m *Manager) isHealthy() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}

	// Try the sessions endpoint since that's what we need
	resp, err := client.Get(m.URL() + "/api/sessions")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// 200 or empty array both indicate healthy domain-enabled server
	return resp.StatusCode == http.StatusOK
}

// start spawns the Intermute server as a subprocess
func (m *Manager) start(ctx context.Context) error {
	// Ensure data directory exists
	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Find intermute binary
	binary, err := m.findBinary()
	if err != nil {
		return err
	}

	dbPath := filepath.Join(m.dataDir, "data.db")

	// Start the server
	m.cmd = exec.Command(binary, "serve",
		"--port", fmt.Sprintf("%d", m.port),
		"--host", m.host,
		"--db", dbPath,
	)

	// Redirect output to files for debugging
	logPath := filepath.Join(m.dataDir, "intermute.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		m.cmd.Stdout = logFile
		m.cmd.Stderr = logFile
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("exec intermute: %w", err)
	}

	m.started = true

	// Wait for server to be ready
	for {
		select {
		case <-ctx.Done():
			m.stop()
			return fmt.Errorf("intermute server did not become healthy: %w", ctx.Err())
		default:
		}

		if m.isHealthy() {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// findBinary locates the intermute binary
func (m *Manager) findBinary() (string, error) {
	// Check common locations in order
	candidates := []string{
		"intermute",                                    // In PATH
		filepath.Join(m.dataDir, "bin", "intermute"),   // ~/.autarch/bin/intermute
		"/usr/local/bin/intermute",
	}

	for _, path := range candidates {
		if path == "intermute" {
			// Check PATH
			if p, err := exec.LookPath("intermute"); err == nil {
				return p, nil
			}
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("intermute binary not found; install with: go install github.com/mistakeknot/intermute/cmd/intermute@latest")
}

// stop terminates the managed Intermute server
func (m *Manager) stop() {
	if !m.started || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	// Try graceful shutdown first
	_ = m.cmd.Process.Signal(os.Interrupt)

	// Wait briefly for clean exit
	done := make(chan error, 1)
	go func() {
		done <- m.cmd.Wait()
	}()

	select {
	case <-done:
		// Clean exit
	case <-time.After(2 * time.Second):
		// Force kill
		_ = m.cmd.Process.Kill()
		<-done
	}

	m.started = false
	m.cmd = nil
}
