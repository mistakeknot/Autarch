package agenttargets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DetectedTool holds the result of detecting an agent CLI tool.
type DetectedTool struct {
	Name    string // "claude", "codex"
	Path    string // Absolute path to the binary
	Version string // Output of --version (may be "unknown")
}

// String returns a display string like "claude (1.0.3)".
func (t *DetectedTool) String() string {
	if t == nil {
		return "none"
	}
	return fmt.Sprintf("%s (%s)", t.Name, t.Version)
}

// ToolDetector probes for the presence of a specific agent CLI.
type ToolDetector interface {
	// Name returns the agent name (e.g., "claude").
	Name() string
	// Detect probes for the tool and returns its info if found.
	Detect(ctx context.Context) (*DetectedTool, bool)
}

// DetectorCache wraps multiple ToolDetectors with TTL-based caching
// and lazy per-tool detection.
type DetectorCache struct {
	detectors []ToolDetector
	ttl       time.Duration

	mu    sync.Mutex
	cache map[string]*cachedResult
}

type cachedResult struct {
	tool      *DetectedTool
	found     bool
	expiresAt time.Time
}

// NewDetectorCache creates a cache that wraps the given detectors.
// Results are cached for the given TTL. Pass 0 for no caching.
func NewDetectorCache(detectors []ToolDetector, ttl time.Duration) *DetectorCache {
	return &DetectorCache{
		detectors: detectors,
		ttl:       ttl,
		cache:     make(map[string]*cachedResult),
	}
}

// DefaultDetectorCache returns a cache with the standard Claude+Codex
// detectors and a 5-minute TTL.
func DefaultDetectorCache() *DetectorCache {
	return NewDetectorCache(
		[]ToolDetector{
			NewMultiMethodDetector("claude", "claude", "--version"),
			NewMultiMethodDetector("codex", "codex", "--version"),
		},
		5*time.Minute,
	)
}

// Detect probes for a specific tool by name. Returns cached result if fresh.
func (dc *DetectorCache) Detect(ctx context.Context, name string) (*DetectedTool, bool) {
	dc.mu.Lock()
	if cached, ok := dc.cache[name]; ok && time.Now().Before(cached.expiresAt) {
		dc.mu.Unlock()
		return cached.tool, cached.found
	}
	dc.mu.Unlock()

	for _, d := range dc.detectors {
		if d.Name() == name {
			tool, found := d.Detect(ctx)
			dc.mu.Lock()
			dc.cache[name] = &cachedResult{
				tool:      tool,
				found:     found,
				expiresAt: time.Now().Add(dc.ttl),
			}
			dc.mu.Unlock()
			return tool, found
		}
	}
	return nil, false
}

// DetectAll probes all registered tools in parallel with a 3s timeout.
// Returns all found tools in detector registration order.
func (dc *DetectorCache) DetectAll(ctx context.Context) []*DetectedTool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	type indexedResult struct {
		idx  int
		tool *DetectedTool
	}

	ch := make(chan indexedResult, len(dc.detectors))
	for i, d := range dc.detectors {
		go func(idx int, det ToolDetector) {
			tool, found := dc.Detect(ctx, det.Name())
			if found {
				ch <- indexedResult{idx: idx, tool: tool}
			} else {
				ch <- indexedResult{idx: idx, tool: nil}
			}
		}(i, d)
	}

	results := make([]*DetectedTool, len(dc.detectors))
	for range dc.detectors {
		r := <-ch
		results[r.idx] = r.tool
	}

	var found []*DetectedTool
	for _, t := range results {
		if t != nil {
			found = append(found, t)
		}
	}
	return found
}

// DetectPreferred returns the first available tool in registration order.
func (dc *DetectorCache) DetectPreferred(ctx context.Context) (*DetectedTool, bool) {
	for _, d := range dc.detectors {
		if tool, found := dc.Detect(ctx, d.Name()); found {
			return tool, true
		}
	}
	return nil, false
}

// Invalidate clears the cache entry for the given tool name.
func (dc *DetectorCache) Invalidate(name string) {
	dc.mu.Lock()
	delete(dc.cache, name)
	dc.mu.Unlock()
}

// InvalidateAll clears all cached results.
func (dc *DetectorCache) InvalidateAll() {
	dc.mu.Lock()
	dc.cache = make(map[string]*cachedResult)
	dc.mu.Unlock()
}

// MultiMethodDetector probes for a tool using an ordered fallback chain:
// 1. PATH lookup (exec.LookPath)
// 2. Platform-specific known install paths
// 3. Homebrew (macOS)
// 4. npm global
type MultiMethodDetector struct {
	name        string
	binary      string
	versionFlag string
}

// NewMultiMethodDetector creates a detector for the given tool.
func NewMultiMethodDetector(name, binary, versionFlag string) *MultiMethodDetector {
	return &MultiMethodDetector{
		name:        name,
		binary:      binary,
		versionFlag: versionFlag,
	}
}

func (d *MultiMethodDetector) Name() string { return d.name }

func (d *MultiMethodDetector) Detect(ctx context.Context) (*DetectedTool, bool) {
	// Method 1: PATH lookup (fast, most common)
	if path, err := exec.LookPath(d.binary); err == nil {
		return d.buildResult(ctx, path), true
	}

	// Method 2: Known install paths
	for _, path := range d.knownPaths() {
		if fileExists(path) {
			return d.buildResult(ctx, path), true
		}
	}

	// Method 3: Homebrew (macOS only)
	if runtime.GOOS == "darwin" {
		if path := d.homebrewPath(); path != "" {
			return d.buildResult(ctx, path), true
		}
	}

	// Method 4: npm global
	if path := d.npmGlobalPath(); path != "" && fileExists(path) {
		return d.buildResult(ctx, path), true
	}

	return nil, false
}

func (d *MultiMethodDetector) buildResult(ctx context.Context, path string) *DetectedTool {
	version := "unknown"
	if d.versionFlag != "" {
		version = getToolVersion(ctx, path, d.versionFlag)
	}
	return &DetectedTool{
		Name:    d.name,
		Path:    path,
		Version: version,
	}
}

func (d *MultiMethodDetector) knownPaths() []string {
	home, _ := os.UserHomeDir()
	var paths []string

	paths = append(paths,
		filepath.Join(home, ".local", "bin", d.binary),
		filepath.Join(home, ".npm-global", "bin", d.binary),
	)
	if runtime.GOOS == "linux" {
		paths = append(paths, filepath.Join("/usr/local/bin", d.binary))
	}
	return paths
}

func (d *MultiMethodDetector) homebrewPath() string {
	prefixes := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	for _, prefix := range prefixes {
		path := filepath.Join(prefix, d.binary)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func (d *MultiMethodDetector) npmGlobalPath() string {
	out, err := exec.Command("npm", "prefix", "-g").Output()
	if err != nil {
		return ""
	}
	prefix := strings.TrimSpace(string(out))
	if prefix == "" {
		return ""
	}
	return filepath.Join(prefix, "bin", d.binary)
}

func getToolVersion(ctx context.Context, path, flag string) string {
	vctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(vctx, path, flag)
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	v := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(v, '\n'); idx >= 0 {
		v = v[:idx]
	}
	return v
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Package-level default cache (singleton, lazy init).
var (
	defaultCacheOnce      sync.Once
	defaultCacheSingleton *DetectorCache
)

func getDefaultCache() *DetectorCache {
	defaultCacheOnce.Do(func() {
		defaultCacheSingleton = DefaultDetectorCache()
	})
	return defaultCacheSingleton
}

// DetectTool probes for a specific tool using the default cache.
func DetectTool(ctx context.Context, name string) (*DetectedTool, bool) {
	return getDefaultCache().Detect(ctx, name)
}

// DetectAllTools probes all known tools using the default cache.
func DetectAllTools(ctx context.Context) []*DetectedTool {
	return getDefaultCache().DetectAll(ctx)
}

// DetectPreferredTool returns the first available tool (claude > codex).
func DetectPreferredTool(ctx context.Context) (*DetectedTool, bool) {
	return getDefaultCache().DetectPreferred(ctx)
}

// DetectAvailableTargets probes the system for installed agent CLIs and
// returns them as a Registry. The lookPath parameter is accepted for backward
// compatibility but is unused — detection goes through MultiMethodDetector.
func DetectAvailableTargets(_ func(string) (string, error)) Registry {
	tools := DetectAllTools(context.Background())
	reg := Registry{Targets: map[string]Target{}}
	for _, t := range tools {
		reg.Targets[t.Name] = Target{
			Name: t.Name,
			Type: TargetDetected,
		}
	}
	return reg
}

// MergeDetected merges registries in order: detected → global → project.
// Later registries override earlier ones.
func MergeDetected(detected, global, project Registry) Registry {
	merged := Merge(detected, global)
	return Merge(merged, project)
}
