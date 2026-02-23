package hunters

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// RetryConfig controls retry behavior for a hunter.
type RetryConfig struct {
	MaxAttempts int           // Total attempts (1 = no retry)
	Backoff     time.Duration // Base delay between retries (doubled each attempt)
}

// DefaultRetryConfig returns a sensible default: 2 attempts with 1s backoff.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{MaxAttempts: 2, Backoff: 1 * time.Second}
}

// HuntWithRetry executes a hunter with retry on transient failures.
// Returns the result from the first successful attempt, or the last error.
func HuntWithRetry(ctx context.Context, h Hunter, cfg HunterConfig, rc RetryConfig) (*HuntResult, error) {
	if rc.MaxAttempts < 1 {
		rc.MaxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= rc.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		result, err := h.Hunt(ctx, cfg)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !isTransient(err) {
			return nil, err
		}

		if attempt < rc.MaxAttempts {
			delay := rc.Backoff * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return nil, fmt.Errorf("after %d attempts: %w", rc.MaxAttempts, lastErr)
}

// isTransient returns true for errors that may succeed on retry.
func isTransient(err error) bool {
	if err == nil {
		return false
	}

	// Network errors — only transient ones (timeout)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Common transient HTTP status messages in error strings
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"rate limit", "429", "503", "timeout", "temporary"} {
		if strings.Contains(msg, s) {
			return true
		}
	}

	return false
}
