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

type Options struct {
	Name         string
	Project      string
	Capabilities []string
	Metadata     map[string]string
	Status       string
}

var (
	newClient    = ic.New
	registerAgent = func(ctx context.Context, c *ic.Client, agent ic.Agent) (ic.Agent, error) {
		return c.RegisterAgent(ctx, agent)
	}
	heartbeat = func(ctx context.Context, c *ic.Client, id string) error {
		return c.Heartbeat(ctx, id)
	}
	warnOfflineOnce sync.Once
)

// Start registers an agent with Intermute and starts heartbeat goroutine.
// Deprecated: Use pkg/intermute.Register() or pkg/intermute.RegisterTool() instead.
func Start(ctx context.Context, opts Options) (func(), error) {
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
		return nil, err
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
