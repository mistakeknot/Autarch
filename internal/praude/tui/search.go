package tui

import (
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

type SearchState struct {
	Active bool
	Query  string
}

func filterSummaries(items []specs.Summary, filter string) []specs.Summary {
	trim := strings.TrimSpace(filter)
	if trim == "" {
		return items
	}
	needle := strings.ToLower(trim)
	var out []specs.Summary
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ID), needle) || strings.Contains(strings.ToLower(item.Title), needle) {
			out = append(out, item)
		}
	}
	return out
}

func updateSearch(state *SearchState, key string) (done bool, canceled bool) {
	switch key {
	case "enter":
		return true, false
	case "esc":
		return true, true
	case "backspace":
		if len(state.Query) > 0 {
			state.Query = state.Query[:len(state.Query)-1]
		}
		return false, false
	default:
		state.Query += key
		return false, false
	}
}
