// Package intermute provides a unified registration API for Autarch tools
// connecting to the Intermute coordination server.
package intermute

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	ic "github.com/mistakeknot/intermute/client"
	"github.com/mistakeknot/autarch/pkg/timeout"
)

// Options configures agent registration with Intermute
type Options struct {
	Name         string            // Agent name (e.g., "bigend", "gurgeh")
	Project      string            // Optional project scope
	Capabilities []string          // Capabilities the agent provides
	Metadata     map[string]string // Additional metadata
	Status       string            // Initial status
}

// Test hooks for mocking in tests
var (
	newClient     = ic.New
	registerAgent = func(ctx context.Context, c *ic.Client, agent ic.Agent) (ic.Agent, error) {
		return c.RegisterAgent(ctx, agent)
	}
	heartbeat = func(ctx context.Context, c *ic.Client, id string) error {
		return c.Heartbeat(ctx, id)
	}
	warnOfflineOnce sync.Once
)

// Register connects an agent to the Intermute server and returns a cleanup function.
// The agent will send periodic heartbeats until the cleanup function is called.
//
// Environment variables:
//   - INTERMUTE_URL: Required. The Intermute server URL.
//   - INTERMUTE_AGENT_NAME: Optional. Overrides opts.Name.
//   - INTERMUTE_PROJECT: Optional. Sets the project scope.
//   - INTERMUTE_API_KEY: Optional. API key for authentication.
//   - INTERMUTE_HEARTBEAT_INTERVAL: Optional. Heartbeat interval (default: 30s).
func Register(ctx context.Context, opts Options) (func(), error) {
	url := strings.TrimSpace(os.Getenv("INTERMUTE_URL"))
	if url == "" {
		if strings.TrimSpace(os.Getenv("INTERMUTE_API_KEY")) != "" || strings.TrimSpace(os.Getenv("INTERMUTE_PROJECT")) != "" {
			warnOfflineOnce.Do(func() {
				log.Printf("intermute offline: INTERMUTE_URL missing while other Intermute env vars are set")
			})
		}
		return func() {}, nil
	}

	name := opts.Name
	if env := strings.TrimSpace(os.Getenv("INTERMUTE_AGENT_NAME")); env != "" {
		name = env
	}

	project := opts.Project
	if project == "" {
		project = strings.TrimSpace(os.Getenv("INTERMUTE_PROJECT"))
	}

	apiKey := strings.TrimSpace(os.Getenv("INTERMUTE_API_KEY"))
	if apiKey != "" && project == "" {
		return nil, fmt.Errorf("INTERMUTE_PROJECT required when INTERMUTE_API_KEY is set")
	}

	var clientOpts []ic.Option
	if apiKey != "" {
		clientOpts = append(clientOpts, ic.WithAPIKey(apiKey))
	}
	if project != "" {
		clientOpts = append(clientOpts, ic.WithProject(project))
	}

	client := newClient(url, clientOpts...)
	agent, err := registerAgent(ctx, client, ic.Agent{
		Name:         name,
		Project:      project,
		Capabilities: opts.Capabilities,
		Metadata:     opts.Metadata,
		Status:       opts.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("register agent %q: %w", name, err)
	}

	// Parse heartbeat interval from environment
	interval := 30 * time.Second
	if env := os.Getenv("INTERMUTE_HEARTBEAT_INTERVAL"); env != "" {
		if d, err := time.ParseDuration(env); err == nil {
			interval = d
		}
	}

	stop := make(chan struct{})
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				hbCtx, cancel := context.WithTimeout(context.Background(), timeout.HTTPDefault)
				_ = heartbeat(hbCtx, client, agent.ID)
				cancel()
			case <-stop:
				return
			}
		}
	}()

	return func() { close(stop) }, nil
}

// RegisterTool is a convenience function that registers a tool with default capabilities.
// The capabilities list defaults to []string{toolName}.
func RegisterTool(ctx context.Context, toolName string) (func(), error) {
	return Register(ctx, Options{
		Name:         toolName,
		Capabilities: []string{toolName},
	})
}
