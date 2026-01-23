package web

import (
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tmux"
)

type FilterState struct {
	Raw      string
	Terms    []string
	Statuses map[tmux.Status]bool
}

func parseFilter(input string) FilterState {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return FilterState{Raw: ""}
	}
	terms := []string{}
	statuses := map[tmux.Status]bool{}
	for _, token := range strings.Fields(strings.ToLower(raw)) {
		if strings.HasPrefix(token, "!") {
			switch strings.TrimPrefix(token, "!") {
			case "running":
				statuses[tmux.StatusRunning] = true
				continue
			case "waiting":
				statuses[tmux.StatusWaiting] = true
				continue
			case "idle":
				statuses[tmux.StatusIdle] = true
				continue
			case "error":
				statuses[tmux.StatusError] = true
				continue
			case "unknown":
				statuses[tmux.StatusUnknown] = true
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

func filterSessions(sessions []aggregator.TmuxSession, state FilterState, statusBySession map[string]tmux.Status) []aggregator.TmuxSession {
	if state.Raw == "" {
		return sessions
	}
	filtered := make([]aggregator.TmuxSession, 0, len(sessions))
	for _, session := range sessions {
		if len(state.Statuses) > 0 {
			status := tmux.StatusUnknown
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

func filterAgents(agents []aggregator.Agent, state FilterState, statusBySession map[string]tmux.Status) []aggregator.Agent {
	if state.Raw == "" {
		return agents
	}
	filtered := make([]aggregator.Agent, 0, len(agents))
	for _, agent := range agents {
		if len(state.Statuses) > 0 {
			status := tmux.StatusUnknown
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
