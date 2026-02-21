package web

import (
	"strings"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/icdata"
)

type FilterState struct {
	Raw      string
	Terms    []string
	Statuses map[icdata.UnifiedStatus]bool
}

func parseFilter(input string) FilterState {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return FilterState{Raw: ""}
	}
	terms := []string{}
	statuses := map[icdata.UnifiedStatus]bool{}
	for _, token := range strings.Fields(strings.ToLower(raw)) {
		if strings.HasPrefix(token, "!") {
			switch strings.TrimPrefix(token, "!") {
			case "running", "active":
				statuses[icdata.StatusActive] = true
				continue
			case "waiting", "idle":
				statuses[icdata.StatusWaiting] = true
				continue
			case "blocked":
				statuses[icdata.StatusBlocked] = true
				continue
			case "error":
				statuses[icdata.StatusErr] = true
				continue
			case "done":
				statuses[icdata.StatusDone] = true
				continue
			case "unknown":
				statuses[icdata.StatusUnknown] = true
				continue
			default:
				token = strings.TrimPrefix(token, "!")
			}
		}
		if token != "" {
			terms = append(terms, token)
		}
	}
	if len(statuses) == 0 {
		statuses = nil
	}
	return FilterState{Raw: raw, Terms: terms, Statuses: statuses}
}

func filterSessions(sessions []aggregator.TmuxSession, state FilterState, statusBySession map[string]icdata.UnifiedStatus) []aggregator.TmuxSession {
	if state.Raw == "" {
		return sessions
	}
	filtered := make([]aggregator.TmuxSession, 0, len(sessions))
	for _, session := range sessions {
		if len(state.Statuses) > 0 {
			status := icdata.StatusUnknown
			if statusBySession != nil {
				if mapped, ok := statusBySession[session.Name]; ok {
					status = mapped
				}
			}
			if !state.Statuses[status] {
				continue
			}
		}
		haystack := strings.ToLower(strings.Join([]string{
			session.Name,
			session.AgentName,
			session.AgentType,
			session.ProjectPath,
		}, " "))
		matches := true
		for _, term := range state.Terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

func filterAgents(agents []aggregator.Agent, state FilterState, statusBySession map[string]icdata.UnifiedStatus) []aggregator.Agent {
	if state.Raw == "" {
		return agents
	}
	filtered := make([]aggregator.Agent, 0, len(agents))
	for _, agent := range agents {
		if len(state.Statuses) > 0 {
			status := icdata.StatusUnknown
			if agent.SessionName != "" && statusBySession != nil {
				if mapped, ok := statusBySession[agent.SessionName]; ok {
					status = mapped
				}
			}
			if !state.Statuses[status] {
				continue
			}
		}
		haystack := strings.ToLower(strings.Join([]string{
			agent.Name,
			agent.Program,
			agent.Model,
			agent.ProjectPath,
		}, " "))
		matches := true
		for _, term := range state.Terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, agent)
		}
	}
	return filtered
}
