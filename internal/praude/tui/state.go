package tui

import "github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"

type SharedState struct {
	Summaries []specs.Summary
	Selected  int
	Focus     string
	Filter    string
}

func NewSharedState() *SharedState {
	return &SharedState{Focus: "LIST"}
}
