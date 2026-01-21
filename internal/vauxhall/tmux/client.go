package tmux

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Session represents an active tmux session
type Session struct {
	Name         string    `json:"name"`
	Created      time.Time `json:"created"`
	LastActivity time.Time `json:"last_activity"`
	WindowCount  int       `json:"window_count"`
	Attached     bool      `json:"attached"`
	CurrentPath  string    `json:"current_path"`
}

// Status represents the current state of an agent session
type Status string

const (
	StatusUnknown Status = "unknown"
	StatusRunning Status = "running" // Agent is actively generating output
	StatusWaiting Status = "waiting" // Agent is waiting for user input
	StatusIdle    Status = "idle"    // Session exists but no recent activity
	StatusError   Status = "error"   // Agent encountered an error
)

// sessionCache holds cached session data to reduce subprocess spawns
type sessionCache struct {
	mu         sync.RWMutex
	sessions   map[string]*cachedSession // session_name -> cached data
	lastUpdate time.Time
	ttl        time.Duration
}

type cachedSession struct {
	Session
	WindowActivity int64 // Unix timestamp of most recent window activity
}

// Client interacts with tmux via CLI commands
type Client struct {
	tmuxPath string
	cache    *sessionCache
}

// NewClient creates a new tmux client with session caching
func NewClient() *Client {
	// Find tmux binary
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		tmuxPath = "tmux" // fallback, will error on use
	}
	return &Client{
		tmuxPath: tmuxPath,
		cache: &sessionCache{
			sessions: make(map[string]*cachedSession),
			ttl:      2 * time.Second, // Cache valid for 2 seconds (4 ticks at 500ms)
		},
	}
}

// IsAvailable checks if tmux is installed and running
func (c *Client) IsAvailable() bool {
	cmd := exec.Command(c.tmuxPath, "list-sessions")
	err := cmd.Run()
	// Exit code 1 with "no server running" is expected when no sessions exist
	return err == nil || (cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1)
}

// RefreshCache updates the session cache with a single tmux command
// This reduces subprocess spawns from O(n) to O(1) per refresh cycle
func (c *Client) RefreshCache() error {
	// Get sessions with activity in a single call
	// Using list-windows -a to get window_activity (updates on terminal output)
	// instead of session_activity (only updates on session-level events)
	format := "#{session_name}\t#{session_created}\t#{session_windows}\t#{session_attached}\t#{window_activity}\t#{pane_current_path}"
	cmd := exec.Command(c.tmuxPath, "list-windows", "-a", "-F", format)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// No sessions is not an error - clear cache
		if strings.Contains(stderr.String(), "no server running") ||
			strings.Contains(stderr.String(), "no sessions") {
			c.cache.mu.Lock()
			c.cache.sessions = make(map[string]*cachedSession)
			c.cache.lastUpdate = time.Now()
			c.cache.mu.Unlock()
			return nil
		}
		return fmt.Errorf("failed to refresh session cache: %w: %s", err, stderr.String())
	}

	newCache := make(map[string]*cachedSession)
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}

		name := parts[0]

		// Parse timestamps
		createdTS, _ := strconv.ParseInt(parts[1], 10, 64)
		windowCount, _ := strconv.Atoi(parts[2])
		attached := parts[3] == "1"
		activityTS, _ := strconv.ParseInt(parts[4], 10, 64)
		currentPath := parts[5]

		// Keep maximum activity if session has multiple windows
		existing, ok := newCache[name]
		if !ok || activityTS > existing.WindowActivity {
			newCache[name] = &cachedSession{
				Session: Session{
					Name:         name,
					Created:      time.Unix(createdTS, 0),
					LastActivity: time.Unix(activityTS, 0),
					WindowCount:  windowCount,
					Attached:     attached,
					CurrentPath:  currentPath,
				},
				WindowActivity: activityTS,
			}
		} else if ok {
			// Update window count (sum of windows)
			existing.WindowCount = windowCount
		}
	}

	c.cache.mu.Lock()
	c.cache.sessions = newCache
	c.cache.lastUpdate = time.Now()
	c.cache.mu.Unlock()

	return nil
}

// ListSessions returns all active tmux sessions (uses cache if valid)
func (c *Client) ListSessions() ([]Session, error) {
	c.cache.mu.RLock()
	cacheValid := time.Since(c.cache.lastUpdate) < c.cache.ttl
	c.cache.mu.RUnlock()

	if !cacheValid {
		if err := c.RefreshCache(); err != nil {
			return nil, err
		}
	}

	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()

	sessions := make([]Session, 0, len(c.cache.sessions))
	for _, cached := range c.cache.sessions {
		sessions = append(sessions, cached.Session)
	}

	return sessions, nil
}

// GetSession returns a specific session from cache
func (c *Client) GetSession(name string) (*Session, bool) {
	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()

	cached, ok := c.cache.sessions[name]
	if !ok {
		return nil, false
	}
	return &cached.Session, true
}

// SessionExists checks if a session exists (from cache)
func (c *Client) SessionExists(name string) bool {
	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()
	_, exists := c.cache.sessions[name]
	return exists
}

// GetSessionPath returns the current working directory of a session's active pane
func (c *Client) GetSessionPath(sessionName string) (string, error) {
	// Try cache first
	c.cache.mu.RLock()
	if cached, ok := c.cache.sessions[sessionName]; ok && cached.CurrentPath != "" {
		path := cached.CurrentPath
		c.cache.mu.RUnlock()
		return path, nil
	}
	c.cache.mu.RUnlock()

	// Fall back to direct query
	cmd := exec.Command(c.tmuxPath, "display-message", "-t", sessionName, "-p", "#{pane_current_path}")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to get session path: %w: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// CapturePane returns recent output from a session's active pane
func (c *Client) CapturePane(sessionName string, lines int) (string, error) {
	linesArg := fmt.Sprintf("-%d", lines)
	cmd := exec.Command(c.tmuxPath, "capture-pane", "-t", sessionName, "-p", "-S", linesArg)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane: %w: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// DetectStatus determines the current status of an agent session by capturing pane output
func (c *Client) DetectStatus(sessionName string) Status {
	output, err := c.CapturePane(sessionName, 50)
	if err != nil {
		slog.Debug("failed to capture pane for status detection", "session", sessionName, "error", err)
		return StatusUnknown
	}

	lines := strings.Split(output, "\n")

	// Check last few non-empty lines for status indicators
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-10; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)

		// Check for waiting indicators (prompt patterns)
		if strings.HasSuffix(line, ">") ||
			strings.HasSuffix(line, "❯") ||
			strings.Contains(lower, "enter a command") ||
			strings.Contains(lower, "what would you like") ||
			strings.Contains(lower, "waiting for") ||
			strings.HasPrefix(line, "$ ") ||
			strings.HasPrefix(line, "% ") {
			return StatusWaiting
		}

		// Check for error indicators
		if strings.Contains(lower, "error:") ||
			strings.Contains(lower, "failed:") ||
			strings.Contains(lower, "exception:") ||
			strings.Contains(lower, "panic:") {
			return StatusError
		}

		// Check for running indicators (tool calls, processing)
		if strings.Contains(line, "⠋") || strings.Contains(line, "⠙") ||
			strings.Contains(line, "⠹") || strings.Contains(line, "⠸") ||
			strings.Contains(line, "⠼") || strings.Contains(line, "⠴") ||
			strings.Contains(line, "⠦") || strings.Contains(line, "⠧") ||
			strings.Contains(line, "⠇") || strings.Contains(line, "⠏") ||
			strings.Contains(lower, "reading") ||
			strings.Contains(lower, "writing") ||
			strings.Contains(lower, "searching") ||
			strings.Contains(lower, "running") {
			return StatusRunning
		}
	}

	// Check activity timestamp to determine if idle
	c.cache.mu.RLock()
	cached, ok := c.cache.sessions[sessionName]
	c.cache.mu.RUnlock()

	if ok {
		// If no activity in last 30 seconds, consider idle
		if time.Since(cached.LastActivity) > 30*time.Second {
			return StatusIdle
		}
	}

	return StatusUnknown
}

// EnableMouseMode enables mouse support for a session (for scrolling)
func (c *Client) EnableMouseMode(sessionName string) error {
	cmd := exec.Command(c.tmuxPath, "set-option", "-t", sessionName, "mouse", "on")
	return cmd.Run()
}

// SendKeys sends keystrokes to a session
func (c *Client) SendKeys(sessionName string, keys string) error {
	cmd := exec.Command(c.tmuxPath, "send-keys", "-t", sessionName, keys)
	return cmd.Run()
}

// AttachSession attaches to an existing session (for TUI integration)
func (c *Client) AttachSession(sessionName string) error {
	cmd := exec.Command(c.tmuxPath, "attach-session", "-t", sessionName)
	cmd.Stdin = nil // Will be set by caller for interactive use
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
