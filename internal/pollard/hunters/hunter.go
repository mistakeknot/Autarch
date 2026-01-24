// Package hunters provides research agent implementations for Pollard.
package hunters

import (
	"context"
	"fmt"
	"time"
)

// Hunter is the interface that all research agents must implement.
type Hunter interface {
	// Name returns the hunter's identifier (e.g., "github-scout").
	Name() string

	// Hunt performs the research collection.
	Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error)
}

// HunterConfig provides configuration for a hunt operation.
type HunterConfig struct {
	// Queries to search for
	Queries []string

	// MaxResults per query
	MaxResults int

	// MinStars for GitHub repos (optional)
	MinStars int

	// MinPoints for HackerNews items (optional)
	MinPoints int

	// Categories for arXiv (optional)
	Categories []string

	// Targets for competitor tracking (optional)
	Targets []CompetitorTarget

	// OutputDir is the relative path for output files
	OutputDir string

	// ProjectPath is the root project path
	ProjectPath string

	// APIToken for authenticated access (optional)
	APIToken string
}

// CompetitorTarget represents a competitor to track.
type CompetitorTarget struct {
	Name      string `yaml:"name"`
	Changelog string `yaml:"changelog,omitempty"`
	Docs      string `yaml:"docs,omitempty"`
	GitHub    string `yaml:"github,omitempty"`
}

// HuntResult contains the results of a hunt operation.
type HuntResult struct {
	HunterName       string
	StartedAt        time.Time
	CompletedAt      time.Time
	SourcesCollected int
	InsightsCreated  int
	OutputFiles      []string
	Errors           []error
}

// String returns a summary of the hunt result.
func (r *HuntResult) String() string {
	duration := r.CompletedAt.Sub(r.StartedAt).Round(time.Millisecond)
	return fmt.Sprintf("%s: %d sources, %d insights in %v",
		r.HunterName, r.SourcesCollected, r.InsightsCreated, duration)
}

// Success returns true if the hunt completed without errors.
func (r *HuntResult) Success() bool {
	return len(r.Errors) == 0
}

// RateLimiter provides rate limiting for API calls.
type RateLimiter struct {
	requests      int
	perDuration   time.Duration
	tokens        int
	lastRefill    time.Time
	authenticated bool
}

// NewRateLimiter creates a rate limiter with the specified limits.
func NewRateLimiter(requests int, per time.Duration, authenticated bool) *RateLimiter {
	return &RateLimiter{
		requests:      requests,
		perDuration:   per,
		tokens:        requests,
		lastRefill:    time.Now(),
		authenticated: authenticated,
	}
}

// Wait blocks until a request can be made within rate limits.
func (r *RateLimiter) Wait(ctx context.Context) error {
	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	tokensToAdd := int(elapsed / r.perDuration * time.Duration(r.requests))
	if tokensToAdd > 0 {
		r.tokens = min(r.requests, r.tokens+tokensToAdd)
		r.lastRefill = now
	}

	// If we have tokens, use one
	if r.tokens > 0 {
		r.tokens--
		return nil
	}

	// Wait for next refill
	waitTime := r.perDuration - elapsed
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(waitTime):
		r.tokens = r.requests - 1
		r.lastRefill = time.Now()
		return nil
	}
}

// Registry holds all available hunters.
type Registry struct {
	hunters map[string]Hunter
}

// NewRegistry creates a new hunter registry.
func NewRegistry() *Registry {
	return &Registry{
		hunters: make(map[string]Hunter),
	}
}

// Register adds a hunter to the registry.
func (r *Registry) Register(h Hunter) {
	r.hunters[h.Name()] = h
}

// Get returns a hunter by name.
func (r *Registry) Get(name string) (Hunter, bool) {
	h, ok := r.hunters[name]
	return h, ok
}

// All returns all registered hunters.
func (r *Registry) All() []Hunter {
	result := make([]Hunter, 0, len(r.hunters))
	for _, h := range r.hunters {
		result = append(result, h)
	}
	return result
}

// DefaultRegistry returns a registry with all default hunters.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(NewGitHubScout())
	reg.Register(NewHackerNewsHunter())
	reg.Register(NewArxivHunter())
	reg.Register(NewCompetitorTracker())
	return reg
}
