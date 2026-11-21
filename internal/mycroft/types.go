// Package mycroft supports routed worker spawning and explicit approval logs.
package mycroft

import "github.com/mistakeknot/autarch/pkg/fleet"

// BeadView is the work description passed to the spawner.
type BeadView = fleet.BeadView

// DispatchAction is the stored approval action; its value preserves existing logs.
type DispatchAction string

const ActionSuggest DispatchAction = "suggest"

// DispatchOutcome records the result of an explicit approval.
type DispatchOutcome string

const (
	OutcomeAccepted DispatchOutcome = "accepted"
	OutcomeFailure  DispatchOutcome = "failure"
)
