package agenttargets

import (
	"context"
	"testing"
	"time"
)

// stubDetector always returns a fixed result.
type stubDetector struct {
	name  string
	tool  *DetectedTool
	found bool
	delay time.Duration
}

func (s *stubDetector) Name() string { return s.name }
func (s *stubDetector) Detect(ctx context.Context) (*DetectedTool, bool) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, false
		}
	}
	return s.tool, s.found
}

func TestDetectorCache_BasicDetection(t *testing.T) {
	cache := NewDetectorCache([]ToolDetector{
		&stubDetector{name: "claude", tool: &DetectedTool{Name: "claude", Path: "/usr/bin/claude", Version: "1.0"}, found: true},
		&stubDetector{name: "codex", found: false},
	}, time.Minute)

	ctx := context.Background()

	tool, found := cache.Detect(ctx, "claude")
	if !found || tool.Name != "claude" {
		t.Error("expected to find claude")
	}

	_, found = cache.Detect(ctx, "codex")
	if found {
		t.Error("codex should not be found")
	}

	_, found = cache.Detect(ctx, "unknown")
	if found {
		t.Error("unknown should not be found")
	}
}

func TestDetectorCache_CachingWorks(t *testing.T) {
	counting := &countingDetector{
		inner: &stubDetector{
			name:  "claude",
			tool:  &DetectedTool{Name: "claude", Path: "/bin/claude", Version: "1.0"},
			found: true,
		},
	}
	cache := NewDetectorCache([]ToolDetector{counting}, time.Minute)
	ctx := context.Background()

	cache.Detect(ctx, "claude")
	cache.Detect(ctx, "claude")

	if counting.calls != 1 {
		t.Errorf("expected 1 detect call, got %d", counting.calls)
	}
}

type countingDetector struct {
	inner ToolDetector
	calls int
}

func (c *countingDetector) Name() string { return c.inner.Name() }
func (c *countingDetector) Detect(ctx context.Context) (*DetectedTool, bool) {
	c.calls++
	return c.inner.Detect(ctx)
}

func TestDetectorCache_InvalidateClears(t *testing.T) {
	counting := &countingDetector{
		inner: &stubDetector{
			name:  "claude",
			tool:  &DetectedTool{Name: "claude", Path: "/bin/claude", Version: "1.0"},
			found: true,
		},
	}
	cache := NewDetectorCache([]ToolDetector{counting}, time.Minute)
	ctx := context.Background()

	cache.Detect(ctx, "claude")
	cache.Invalidate("claude")
	cache.Detect(ctx, "claude")

	if counting.calls != 2 {
		t.Errorf("expected 2 detect calls after invalidate, got %d", counting.calls)
	}
}

func TestDetectorCache_TTLExpiry(t *testing.T) {
	counting := &countingDetector{
		inner: &stubDetector{
			name:  "claude",
			tool:  &DetectedTool{Name: "claude", Path: "/bin/claude", Version: "1.0"},
			found: true,
		},
	}
	cache := NewDetectorCache([]ToolDetector{counting}, time.Millisecond)
	ctx := context.Background()

	cache.Detect(ctx, "claude")
	time.Sleep(5 * time.Millisecond)
	cache.Detect(ctx, "claude")

	if counting.calls != 2 {
		t.Errorf("expected 2 detect calls after TTL expiry, got %d", counting.calls)
	}
}

func TestDetectorCache_DetectAll_Parallel(t *testing.T) {
	cache := NewDetectorCache([]ToolDetector{
		&stubDetector{name: "claude", tool: &DetectedTool{Name: "claude", Path: "/bin/claude"}, found: true, delay: 10 * time.Millisecond},
		&stubDetector{name: "codex", tool: &DetectedTool{Name: "codex", Path: "/bin/codex"}, found: true, delay: 10 * time.Millisecond},
	}, time.Minute)

	start := time.Now()
	tools := cache.DetectAll(context.Background())
	elapsed := time.Since(start)

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("DetectAll took %v — should be parallel", elapsed)
	}
	if tools[0].Name != "claude" || tools[1].Name != "codex" {
		t.Errorf("wrong order: %s, %s", tools[0].Name, tools[1].Name)
	}
}

func TestDetectorCache_DetectPreferred(t *testing.T) {
	cache := NewDetectorCache([]ToolDetector{
		&stubDetector{name: "claude", found: false},
		&stubDetector{name: "codex", tool: &DetectedTool{Name: "codex", Path: "/bin/codex"}, found: true},
	}, time.Minute)

	tool, found := cache.DetectPreferred(context.Background())
	if !found || tool.Name != "codex" {
		t.Error("expected codex as preferred when claude not found")
	}
}

func TestDetectorCache_DetectPreferred_NoneFound(t *testing.T) {
	cache := NewDetectorCache([]ToolDetector{
		&stubDetector{name: "claude", found: false},
		&stubDetector{name: "codex", found: false},
	}, time.Minute)

	_, found := cache.DetectPreferred(context.Background())
	if found {
		t.Error("expected no tool found")
	}
}

func TestMultiMethodDetector_LookPathWorks(t *testing.T) {
	d := NewMultiMethodDetector("echo", "echo", "")
	tool, found := d.Detect(context.Background())
	if !found {
		t.Skip("echo not in PATH")
	}
	if tool.Path == "" {
		t.Error("detected path is empty")
	}
}

func TestDetectedTool_String(t *testing.T) {
	tool := &DetectedTool{Name: "claude", Path: "/bin/claude", Version: "1.2.3"}
	if s := tool.String(); s != "claude (1.2.3)" {
		t.Errorf("String() = %q", s)
	}

	var nilTool *DetectedTool
	if s := nilTool.String(); s != "none" {
		t.Errorf("nil String() = %q", s)
	}
}
