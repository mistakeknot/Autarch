package clavain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout for clavain-cli subprocess calls.
const DefaultTimeout = 15 * time.Second

// Option configures a Client.
type Option func(*Client)

// WithBinPath forces a specific clavain-cli binary path (skips LookPath).
func WithBinPath(path string) Option {
	return func(c *Client) { c.binPath = path }
}

// WithTimeout sets the subprocess timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// Client wraps the clavain-cli binary for OS-layer operations.
type Client struct {
	binPath string
	timeout time.Duration
}

// New discovers the clavain-cli binary on PATH.
// Returns ErrUnavailable if not found.
func New(opts ...Option) (*Client, error) {
	c := &Client{timeout: DefaultTimeout}
	for _, o := range opts {
		o(c)
	}
	if c.binPath == "" {
		path, err := exec.LookPath("clavain-cli")
		if err != nil {
			return nil, ErrUnavailable
		}
		c.binPath = path
	}
	return c, nil
}

// Available returns true if clavain-cli is discoverable.
func Available() bool {
	_, err := New()
	return err == nil
}

// execRaw runs clavain-cli with the given args and returns stdout.
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
		return stdout.Bytes(), fmt.Errorf("clavain-cli %s: %s", strings.Join(args, " "), errMsg)
	}
	return stdout.Bytes(), nil
}

// execText runs clavain-cli and returns trimmed stdout text.
func (c *Client) execText(ctx context.Context, args ...string) (string, error) {
	out, err := c.execRaw(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// execJSON runs clavain-cli and unmarshals JSON stdout into dst.
func (c *Client) execJSON(ctx context.Context, dst any, args ...string) error {
	out, err := c.execRaw(ctx, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes.TrimSpace(out), dst)
}
