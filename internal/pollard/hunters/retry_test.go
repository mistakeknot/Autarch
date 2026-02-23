package hunters

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type fakeHunter struct {
	name  string
	calls int
	failN int   // Fail first N calls
	err   error // Error to return on failure
}

func (f *fakeHunter) Name() string { return f.name }

func (f *fakeHunter) Hunt(ctx context.Context, _ HunterConfig) (*HuntResult, error) {
	f.calls++
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.calls <= f.failN {
		return nil, f.err
	}
	return &HuntResult{HunterName: f.name, SourcesCollected: 1}, nil
}

func TestHuntWithRetry_SucceedsFirstAttempt(t *testing.T) {
	h := &fakeHunter{name: "test"}
	result, err := HuntWithRetry(context.Background(), h, HunterConfig{}, DefaultRetryConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.calls != 1 {
		t.Errorf("expected 1 call, got %d", h.calls)
	}
	if result.SourcesCollected != 1 {
		t.Errorf("expected 1 source, got %d", result.SourcesCollected)
	}
}

func TestHuntWithRetry_RetriesTransient(t *testing.T) {
	h := &fakeHunter{name: "test", failN: 1, err: &net.DNSError{IsTimeout: true}}
	result, err := HuntWithRetry(context.Background(), h, HunterConfig{}, RetryConfig{MaxAttempts: 2, Backoff: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.calls != 2 {
		t.Errorf("expected 2 calls, got %d", h.calls)
	}
	if result.SourcesCollected != 1 {
		t.Errorf("expected 1 source, got %d", result.SourcesCollected)
	}
}

func TestHuntWithRetry_NoRetryOnNonTransient(t *testing.T) {
	h := &fakeHunter{name: "test", failN: 10, err: errors.New("invalid config")}
	_, err := HuntWithRetry(context.Background(), h, HunterConfig{}, RetryConfig{MaxAttempts: 3, Backoff: time.Millisecond})
	if err == nil {
		t.Fatal("expected error")
	}
	if h.calls != 1 {
		t.Errorf("expected 1 call (no retry), got %d", h.calls)
	}
}

func TestHuntWithRetry_ExhaustsAttempts(t *testing.T) {
	h := &fakeHunter{name: "test", failN: 10, err: &net.DNSError{IsTimeout: true}}
	_, err := HuntWithRetry(context.Background(), h, HunterConfig{}, RetryConfig{MaxAttempts: 2, Backoff: time.Millisecond})
	if err == nil {
		t.Fatal("expected error")
	}
	if h.calls != 2 {
		t.Errorf("expected 2 calls, got %d", h.calls)
	}
}

func TestHuntWithRetry_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := &fakeHunter{name: "test", failN: 10, err: &net.DNSError{IsTimeout: true}}
	_, err := HuntWithRetry(ctx, h, HunterConfig{}, RetryConfig{MaxAttempts: 3, Backoff: time.Millisecond})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestHuntWithRetry_ZeroMaxAttempts(t *testing.T) {
	h := &fakeHunter{name: "test"}
	result, err := HuntWithRetry(context.Background(), h, HunterConfig{}, RetryConfig{MaxAttempts: 0, Backoff: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.calls != 1 {
		t.Errorf("expected 1 call (clamped to 1), got %d", h.calls)
	}
	if result.SourcesCollected != 1 {
		t.Errorf("expected 1 source, got %d", result.SourcesCollected)
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{"nil", nil, false},
		{"non-transient", errors.New("invalid config"), false},
		{"rate limit", errors.New("rate limit exceeded"), true},
		{"429", errors.New("HTTP 429 Too Many Requests"), true},
		{"503", errors.New("HTTP 503 Service Unavailable"), true},
		{"dns timeout", &net.DNSError{IsTimeout: true}, true},
		{"dns not found", &net.DNSError{IsNotFound: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransient(tt.err); got != tt.transient {
				t.Errorf("isTransient(%v) = %v, want %v", tt.err, got, tt.transient)
			}
		})
	}
}
