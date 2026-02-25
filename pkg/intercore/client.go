package intercore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout for ic subprocess calls.
const DefaultTimeout = 10 * time.Second

// Option configures a Client.
type Option func(*Client)

// WithDBPath overrides the --db flag for all ic calls.
func WithDBPath(path string) Option {
	return func(c *Client) { c.dbPath = path }
}

// WithTimeout sets the default subprocess timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithBinPath forces a specific ic binary path (skips LookPath).
func WithBinPath(path string) Option {
	return func(c *Client) { c.binPath = path }
}

// Client wraps the ic CLI binary.
type Client struct {
	binPath string
	dbPath  string
	timeout time.Duration
}

// New discovers the ic binary and verifies it's healthy.
// Returns ErrUnavailable if ic is not found or health check fails.
func New(opts ...Option) (*Client, error) {
	c := &Client{timeout: DefaultTimeout}
	for _, o := range opts {
		o(c)
	}
	if c.binPath == "" {
		path, err := exec.LookPath("ic")
		if err != nil {
			return nil, ErrUnavailable
		}
		c.binPath = path
	}
	// Health check.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.execRaw(ctx, "health"); err != nil {
		return nil, fmt.Errorf("%w: health check failed: %v", ErrUnavailable, err)
	}
	return c, nil
}

// Available returns true if ic is discoverable and healthy.
func Available() bool {
	_, err := New()
	return err == nil
}

// baseArgs returns the common prefix args (--json, --db if set).
func (c *Client) baseArgs(useJSON bool) []string {
	var args []string
	if c.dbPath != "" {
		args = append(args, "--db="+c.dbPath)
	}
	if useJSON {
		// --json must come before the subcommand (positional).
		args = append(args, "--json")
	}
	return args
}

// execRaw runs ic with the given args and returns stdout.
func (c *Client) execRaw(ctx context.Context, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := c.timeout
	if _, ok := ctx.Deadline(); !ok && timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, c.binPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		// Return stdout alongside the error — some commands (gate check)
		// write valid JSON to stdout even on non-zero exit.
		return stdout.Bytes(), fmt.Errorf("ic %s: %s", strings.Join(args, " "), errMsg)
	}
	return stdout.Bytes(), nil
}

// execJSON runs ic --json <args> and returns the raw JSON bytes.
func (c *Client) execJSON(ctx context.Context, args ...string) ([]byte, error) {
	fullArgs := append(c.baseArgs(true), args...)
	return c.execRaw(ctx, fullArgs...)
}

// execText runs ic <args> (no --json) and returns trimmed stdout.
func (c *Client) execText(ctx context.Context, args ...string) (string, error) {
	fullArgs := append(c.baseArgs(false), args...)
	out, err := c.execRaw(ctx, fullArgs...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// unmarshal is a convenience for JSON decoding into a typed value.
func unmarshal[T any](data []byte) (T, error) {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("intercore: json decode: %w", err)
	}
	return v, nil
}
