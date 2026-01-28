package signals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultSignalsURL = "http://127.0.0.1:8092"

// DefaultServerURL returns the signals server URL, using AUTARCH_SIGNALS_URL if set.
func DefaultServerURL() string {
	if v := strings.TrimSpace(os.Getenv("AUTARCH_SIGNALS_URL")); v != "" {
		return v
	}
	return defaultSignalsURL
}

// Client publishes signals to the signals server.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a new signals client with a short timeout.
func NewClient(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultSignalsURL
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// Publish sends a signal to the signals server.
func (c *Client) Publish(ctx context.Context, sig Signal) error {
	if c == nil {
		return fmt.Errorf("signals client is nil")
	}
	body, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("marshal signal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/signals", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("signals publish failed: %s", resp.Status)
	}
	return nil
}
