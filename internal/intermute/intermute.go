package intermute

import (
	"context"
	"os"
	"strings"
	"time"

	ic "github.com/mistakeknot/intermute/client"
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
)

func Start(ctx context.Context, opts Options) (func(), error) {
	url := strings.TrimSpace(os.Getenv("INTERMUTE_URL"))
	if url == "" {
		return nil, nil
	}
	name := opts.Name
	if env := strings.TrimSpace(os.Getenv("INTERMUTE_AGENT_NAME")); env != "" {
		name = env
	}
	project := opts.Project
	if project == "" {
		project = strings.TrimSpace(os.Getenv("INTERMUTE_PROJECT"))
	}
	client := newClient(url)
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

	stop := make(chan struct{})
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = heartbeat(context.Background(), client, agent.ID)
			case <-stop:
				return
			}
		}
	}()

	return func() { close(stop) }, nil
}
