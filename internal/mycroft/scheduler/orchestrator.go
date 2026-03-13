package scheduler

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// Orchestrator coordinates the patrol→rank→dispatch loop based on the
// current autonomy tier. It is invoked after each patrol cycle with
// the latest FleetView.
type Orchestrator struct {
	db         *sql.DB
	spawner    AgentSpawner
	cfg        mycroft.Config
	project    string
	paused     bool
	lastDemote time.Time // debounce rapid demotions
}

// NewOrchestrator creates an orchestrator.
func NewOrchestrator(db *sql.DB, spawner AgentSpawner, cfg mycroft.Config, project string) *Orchestrator {
	if project == "" {
		project = "demarch"
	}
	return &Orchestrator{
		db:      db,
		spawner: spawner,
		cfg:     cfg,
		project: project,
	}
}

// OnCycle is the callback for the patrol loop. It evaluates the fleet view
// and takes action based on the current tier.
func (o *Orchestrator) OnCycle(view mycroft.FleetView) {
	if o.paused {
		return
	}

	tier := o.cfg.Tier

	// Rank available work.
	ranked := RankBeads(view.Work)
	if len(ranked) == 0 {
		return
	}

	// Find available agents (status = active or idle, not already dispatched).
	available := availableAgents(view.Agents)
	if len(available) == 0 {
		return
	}

	switch tier {
	case mycroft.T0:
		o.shadowSuggest(ranked, available, view.Conflicts)
	case mycroft.T1:
		o.suggest(ranked, available, view.Conflicts)
	case mycroft.T2:
		o.autoDispatchFiltered(ranked, available, view.Conflicts)
	case mycroft.T3:
		o.autoDispatchAll(ranked, available, view.Conflicts)
	}
}

// Pause stops dispatching. In-flight agents continue.
func (o *Orchestrator) Pause()  { o.paused = true }

// Resume re-enables dispatching.
func (o *Orchestrator) Resume() { o.paused = false }

// IsPaused returns whether dispatching is paused.
func (o *Orchestrator) IsPaused() bool { return o.paused }

// shadowSuggest logs what Mycroft would suggest without taking action (T0).
func (o *Orchestrator) shadowSuggest(ranked []mycroft.BeadView, agents []mycroft.AgentView, conflicts []mycroft.ConflictView) {
	for _, agent := range agents {
		selected := SelectForAgent(ranked, agent, conflicts, 1)
		if len(selected) == 0 {
			continue
		}
		bead := selected[0]
		slog.Info("shadow suggestion",
			"agent", agent.Name,
			"bead", bead.ID,
			"title", bead.Title,
			"priority", bead.Priority,
		)
		o.logDispatch(agent.Name, bead.ID, mycroft.ActionShadowSuggest, "", "")
	}
}

// suggest proposes assignments for user approval (T1).
func (o *Orchestrator) suggest(ranked []mycroft.BeadView, agents []mycroft.AgentView, conflicts []mycroft.ConflictView) {
	for _, agent := range agents {
		selected := SelectForAgent(ranked, agent, conflicts, 1)
		if len(selected) == 0 {
			continue
		}
		bead := selected[0]
		slog.Info("dispatch suggestion",
			"agent", agent.Name,
			"bead", bead.ID,
			"title", bead.Title,
			"priority", bead.Priority,
		)
		o.logDispatch(agent.Name, bead.ID, mycroft.ActionSuggest, "", "")
	}
}

// autoDispatchFiltered dispatches only beads that pass the T2 allowlist (T2).
func (o *Orchestrator) autoDispatchFiltered(ranked []mycroft.BeadView, agents []mycroft.AgentView, conflicts []mycroft.ConflictView) {
	for _, agent := range agents {
		selected := SelectForAgent(ranked, agent, conflicts, 1)
		if len(selected) == 0 {
			continue
		}
		bead := selected[0]

		if !AllowlistCheck(bead, o.cfg.T2DispatchAllowlist) {
			// Bead doesn't match allowlist — escalate to user as suggestion.
			slog.Info("T2 escalate — bead outside allowlist",
				"agent", agent.Name,
				"bead", bead.ID,
			)
			o.logDispatch(agent.Name, bead.ID, mycroft.ActionSuggest, "", "outside T2 allowlist")
			continue
		}

		o.dispatchToAgent(agent, bead)
	}
}

// autoDispatchAll dispatches any eligible bead (T3).
func (o *Orchestrator) autoDispatchAll(ranked []mycroft.BeadView, agents []mycroft.AgentView, conflicts []mycroft.ConflictView) {
	for _, agent := range agents {
		selected := SelectForAgent(ranked, agent, conflicts, 1)
		if len(selected) == 0 {
			continue
		}
		o.dispatchToAgent(agent, selected[0])
	}
}

// dispatchToAgent spawns an agent session for the bead.
func (o *Orchestrator) dispatchToAgent(agent mycroft.AgentView, bead mycroft.BeadView) {
	if o.spawner == nil {
		slog.Warn("no spawner configured — logging dispatch only",
			"agent", agent.Name, "bead", bead.ID)
		o.logDispatch(agent.Name, bead.ID, mycroft.ActionAutoDispatch, string(mycroft.OutcomeAccepted), "")
		return
	}

	sessionID, err := o.spawner.Spawn(agent.Name, bead, "")
	if err != nil {
		slog.Error("dispatch failed",
			"agent", agent.Name,
			"bead", bead.ID,
			"err", err,
		)
		o.logDispatch(agent.Name, bead.ID, mycroft.ActionAutoDispatch, string(mycroft.OutcomeFailure), err.Error())
		return
	}

	slog.Info("dispatched",
		"agent", agent.Name,
		"bead", bead.ID,
		"session", sessionID,
	)
	o.logDispatch(agent.Name, bead.ID, mycroft.ActionAutoDispatch, string(mycroft.OutcomeAccepted), "")
}

func (o *Orchestrator) logDispatch(agent, beadID string, action mycroft.DispatchAction, outcome, reason string) {
	if o.db == nil {
		return
	}
	_, err := o.db.Exec(
		`INSERT INTO dispatch_log (ts, project, agent, bead, action, outcome, reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), o.project, agent, beadID, string(action),
		nullString(outcome), nullString(reason),
	)
	if err != nil {
		slog.Error("log dispatch failed", "err", err)
	}
}

// availableAgents returns agents that are active or idle (not offline).
func availableAgents(agents []mycroft.AgentView) []mycroft.AgentView {
	var available []mycroft.AgentView
	for _, a := range agents {
		if a.Status == "active" || a.Status == "idle" {
			available = append(available, a)
		}
	}
	return available
}
