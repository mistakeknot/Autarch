package review

import (
	"context"
	"errors"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
)

// HandoffCoordinator is the controller-owned transport seam. The UI talks to
// it over IPC and never opens the durable store alongside the controller.
type HandoffCoordinator struct {
	Store     *Store
	Transport agenttransport.Transport
}

func (c HandoffCoordinator) validateProject(r Request) error {
	project, err := projectPath(r.Project)
	if err != nil {
		return err
	}
	handoff, ok := c.Store.Snapshot().ExternalHandoffs[r.Target]
	if !ok || handoff.Project != project {
		return errors.New("handoff project/identity mismatch")
	}
	return nil
}

func (c HandoffCoordinator) Handle(ctx context.Context, r Request) Response {
	response := Response{Version: Version, ID: r.Target}
	if c.Store == nil || c.Transport == nil {
		response.Error = "external handoff coordinator unavailable"
		return response
	}
	if err := c.validateProject(r); err != nil {
		response.Error = err.Error()
		return response
	}
	var result agenttransport.Result
	switch r.Method {
	case "handoff.deliver":
		result = c.Store.DeliverHandoff(ctx, r.Target, c.Transport)
	case "handoff.interrupt":
		result = c.Store.InterruptHandoff(ctx, r.Target, c.Transport)
	default:
		response.Error = "unknown external handoff operation"
		return response
	}
	response.Delivery = result.State
	if result.Err != nil {
		response.Error = result.Err.Error()
	}
	return response
}
