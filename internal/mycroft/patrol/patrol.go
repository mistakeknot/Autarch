// Package patrol implements the periodic polling loop that queries intermux,
// beads, and interlock to compose a FleetView each cycle.
package patrol

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// Intervals for the patrol cycle.
const (
	DefaultFleetInterval = 30 * time.Second // How often to check agent health
	DefaultWorkInterval  = 60 * time.Second // How often to check bead queue
)

// Patrol is the main coordinator that periodically queries data sources
// and composes a FleetView.
type Patrol struct {
	source        mycroft.DataSource
	cfg           mycroft.Config
	heartbeatDir  string
	fleetInterval time.Duration
	workInterval  time.Duration
	lastFleet     time.Time
	lastWork      time.Time
	lastView      mycroft.FleetView
	onCycle       func(mycroft.FleetView) // callback after each cycle
}

// Option configures a Patrol.
type Option func(*Patrol)

// WithFleetInterval sets the agent health check interval.
func WithFleetInterval(d time.Duration) Option {
	return func(p *Patrol) { p.fleetInterval = d }
}

// WithWorkInterval sets the bead queue check interval.
func WithWorkInterval(d time.Duration) Option {
	return func(p *Patrol) { p.workInterval = d }
}

// WithOnCycle sets a callback invoked after each patrol cycle.
func WithOnCycle(fn func(mycroft.FleetView)) Option {
	return func(p *Patrol) { p.onCycle = fn }
}

// New creates a Patrol with the given data source and config.
func New(source mycroft.DataSource, cfg mycroft.Config, heartbeatDir string, opts ...Option) *Patrol {
	p := &Patrol{
		source:        source,
		cfg:           cfg,
		heartbeatDir:  heartbeatDir,
		fleetInterval: DefaultFleetInterval,
		workInterval:  DefaultWorkInterval,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run starts the patrol loop, blocking until ctx is cancelled.
func (p *Patrol) Run(ctx context.Context) error {
	slog.Info("patrol started",
		"fleet_interval", p.fleetInterval,
		"work_interval", p.workInterval,
	)

	// Run first cycle immediately.
	p.cycle(ctx)

	ticker := time.NewTicker(p.fleetInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("patrol stopped")
			return ctx.Err()
		case <-ticker.C:
			p.cycle(ctx)
		}
	}
}

// RunOnce executes a single patrol cycle and returns the resulting FleetView.
func (p *Patrol) RunOnce(ctx context.Context) mycroft.FleetView {
	p.cycle(ctx)
	return p.lastView
}

// LastView returns the most recent FleetView.
func (p *Patrol) LastView() mycroft.FleetView {
	return p.lastView
}

func (p *Patrol) cycle(ctx context.Context) {
	now := time.Now()
	view := mycroft.FleetView{
		Freshness: make(map[string]time.Time),
	}

	// Always query fleet state (agent health).
	fleetView, err := p.source.FleetState()
	if err != nil {
		slog.Warn("fleet state query failed", "err", err)
	} else {
		view.Agents = fleetView.Agents
		view.Conflicts = fleetView.Conflicts
		view.Freshness["intermux"] = now
		p.lastFleet = now
	}

	// Query work queue on work interval.
	if now.Sub(p.lastWork) >= p.workInterval || p.lastWork.IsZero() {
		beads, err := p.source.BeadQueue()
		if err != nil {
			slog.Warn("bead queue query failed", "err", err)
		} else {
			view.Work = beads
			view.Freshness["beads"] = now
			p.lastWork = now
		}
	} else {
		// Carry forward previous work data.
		view.Work = p.lastView.Work
		if t, ok := p.lastView.Freshness["beads"]; ok {
			view.Freshness["beads"] = t
		}
	}

	p.lastView = view
	p.writeHeartbeat(now)

	if p.onCycle != nil {
		p.onCycle(view)
	}
}

// IsStale returns true if a source's data is older than 2x the poll interval.
func (p *Patrol) IsStale(source string) bool {
	ts, ok := p.lastView.Freshness[source]
	if !ok {
		return true
	}

	var interval time.Duration
	switch source {
	case "beads":
		interval = p.workInterval
	default:
		interval = p.fleetInterval
	}
	return time.Since(ts) > 2*interval
}

func (p *Patrol) writeHeartbeat(t time.Time) {
	if p.heartbeatDir == "" {
		return
	}
	path := filepath.Join(p.heartbeatDir, "heartbeat")
	os.MkdirAll(p.heartbeatDir, 0755)
	os.WriteFile(path, []byte(strconv.FormatInt(t.Unix(), 10)), 0644)
}
