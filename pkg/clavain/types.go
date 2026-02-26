// Package clavain provides a Go client for the clavain-cli binary (OS layer).
// It shells out to clavain-cli with subprocess calls, mirroring
// pkg/intercore's pattern for the ic binary.
//
// For policy-governing write operations (sprint creation, dispatch, advancement,
// gate enforcement, artifact registration), apps should use this package
// instead of calling ic directly.
package clavain

import "errors"

// ErrUnavailable is returned when clavain-cli is not found on PATH.
var ErrUnavailable = errors.New("clavain: clavain-cli binary not available")

// SprintCreateResult from clavain-cli sprint-create.
type SprintCreateResult struct {
	BeadID string `json:"bead_id"`
	RunID  string `json:"run_id"`
}

// AdvanceResult from clavain-cli sprint-advance.
type AdvanceResult struct {
	Advanced  bool   `json:"advanced"`
	FromPhase string `json:"from_phase"`
	ToPhase   string `json:"to_phase"`
	Reason    string `json:"reason,omitempty"`
}

// GateResult from clavain-cli enforce-gate.
type GateResult struct {
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
}

// DispatchResult from clavain-cli dispatch-task.
type DispatchResult struct {
	DispatchID string `json:"dispatch_id"`
	RunID      string `json:"run_id"`
}
