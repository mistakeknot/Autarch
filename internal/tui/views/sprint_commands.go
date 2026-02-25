package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/intercore"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// SprintCommandRouter intercepts slash commands from chat input and routes
// them to Intercore sprint/dispatch actions. Non-command messages pass through
// to the wrapped handler.
type SprintCommandRouter struct {
	inner   pkgtui.ChatHandler
	iclient *intercore.Client
}

// NewSprintCommandRouter wraps a chat handler with sprint command routing.
func NewSprintCommandRouter(inner pkgtui.ChatHandler, iclient *intercore.Client) *SprintCommandRouter {
	return &SprintCommandRouter{inner: inner, iclient: iclient}
}

// HandleMessage intercepts /sprint and /dispatch commands.
func (r *SprintCommandRouter) HandleMessage(ctx context.Context, userMsg string) (<-chan pkgtui.StreamMsg, error) {
	msg := strings.TrimSpace(userMsg)
	if !strings.HasPrefix(msg, "/") || r.iclient == nil {
		return r.inner.HandleMessage(ctx, userMsg)
	}

	parts := strings.Fields(msg)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/sprint":
		return r.handleSprint(ctx, parts[1:])
	case "/dispatch":
		return r.handleDispatch(ctx, parts[1:])
	case "/research":
		return r.handleResearch(ctx, parts[1:])
	default:
		// Unknown slash command — pass through to inner handler.
		return r.inner.HandleMessage(ctx, userMsg)
	}
}

func (r *SprintCommandRouter) handleSprint(ctx context.Context, args []string) (<-chan pkgtui.StreamMsg, error) {
	if len(args) == 0 {
		return immediateResponse("Usage: /sprint [status|advance|cancel|list]"), nil
	}

	sub := strings.ToLower(args[0])
	ic := r.iclient

	switch sub {
	case "status":
		return asyncResponse(func() string {
			runs, err := ic.RunList(ctx, true)
			if err != nil {
				return fmt.Sprintf("Error: %s", err)
			}
			if len(runs) == 0 {
				return "No active sprints."
			}
			var b strings.Builder
			for _, r := range runs {
				b.WriteString(fmt.Sprintf("**%s** — phase: %s, status: %s\n", r.ID, r.Phase, r.Status))
				if r.Goal != "" {
					b.WriteString(fmt.Sprintf("  Goal: %s\n", r.Goal))
				}
			}
			return b.String()
		}), nil

	case "advance":
		return asyncResponse(func() string {
			runs, err := ic.RunList(ctx, true)
			if err != nil || len(runs) == 0 {
				return "No active sprint to advance."
			}
			result, err := ic.RunAdvance(ctx, runs[0].ID)
			if err != nil {
				return fmt.Sprintf("Advance failed: %s", err)
			}
			if result.Succeeded() {
				return fmt.Sprintf("Advanced: %s → %s", result.FromPhase, result.ToPhase)
			}
			return fmt.Sprintf("Gate blocked: %s — %s", result.GateResult, result.Reason)
		}), nil

	case "cancel":
		return asyncResponse(func() string {
			runs, err := ic.RunList(ctx, true)
			if err != nil || len(runs) == 0 {
				return "No active sprint to cancel."
			}
			if err := ic.RunCancel(ctx, runs[0].ID); err != nil {
				return fmt.Sprintf("Cancel failed: %s", err)
			}
			return fmt.Sprintf("Cancelled sprint %s", runs[0].ID)
		}), nil

	case "list":
		return asyncResponse(func() string {
			active, _ := ic.RunList(ctx, true)
			inactive, _ := ic.RunList(ctx, false)
			all := append(active, inactive...)
			if len(all) == 0 {
				return "No sprints found."
			}
			var b strings.Builder
			for _, r := range all {
				age := time.Since(r.CreatedTime()).Truncate(time.Minute)
				b.WriteString(fmt.Sprintf("**%s** %s (phase: %s, %s ago)\n", r.ID, r.Status, r.Phase, age))
			}
			return b.String()
		}), nil

	case "create":
		goal := "Sprint"
		if len(args) > 1 {
			goal = strings.Join(args[1:], " ")
		}
		return asyncResponse(func() string {
			runID, err := ic.RunCreate(ctx, ".", goal)
			if err != nil {
				return fmt.Sprintf("Create failed: %s", err)
			}
			return fmt.Sprintf("Created sprint **%s** — goal: %s", runID, goal)
		}), nil

	default:
		return immediateResponse(fmt.Sprintf("Unknown: /sprint %s\nAvailable: status, advance, cancel, list, create", sub)), nil
	}
}

func (r *SprintCommandRouter) handleDispatch(ctx context.Context, args []string) (<-chan pkgtui.StreamMsg, error) {
	if len(args) < 1 {
		return immediateResponse("Usage: /dispatch list | /dispatch spawn <type> [name]"), nil
	}

	sub := strings.ToLower(args[0])
	ic := r.iclient

	switch sub {
	case "list":
		return asyncResponse(func() string {
			dispatches, err := ic.DispatchList(ctx, false)
			if err != nil {
				return fmt.Sprintf("Error: %s", err)
			}
			if len(dispatches) == 0 {
				return "No dispatches."
			}
			var b strings.Builder
			for _, d := range dispatches {
				name := d.DisplayName()
				b.WriteString(fmt.Sprintf("**%s** %s — %s\n", d.ID[:8], name, d.Status))
			}
			return b.String()
		}), nil

	case "spawn":
		if len(args) < 2 {
			return immediateResponse("Usage: /dispatch spawn <type> [name]"), nil
		}
		dispType := args[1]
		dispName := ""
		if len(args) > 2 {
			dispName = strings.Join(args[2:], " ")
		}
		return asyncResponse(func() string {
			runs, err := ic.RunList(ctx, true)
			if err != nil || len(runs) == 0 {
				return "No active sprint — create one first."
			}
			opts := []intercore.DispatchOption{intercore.WithDispatchType(dispType)}
			if dispName != "" {
				opts = append(opts, intercore.WithDispatchName(dispName))
			}
			id, err := ic.DispatchSpawn(ctx, runs[0].ID, opts...)
			if err != nil {
				return fmt.Sprintf("Spawn failed: %s", err)
			}
			return fmt.Sprintf("Spawned dispatch **%s** (type: %s)", id, dispType)
		}), nil

	default:
		return immediateResponse(fmt.Sprintf("Unknown: /dispatch %s\nAvailable: list, spawn", sub)), nil
	}
}

func (r *SprintCommandRouter) handleResearch(ctx context.Context, args []string) (<-chan pkgtui.StreamMsg, error) {
	topic := "current project"
	if len(args) > 0 {
		topic = strings.Join(args, " ")
	}
	return immediateResponse(fmt.Sprintf("Research requested for: %s\nUse the Sprint tab's \"Research Spec\" palette command for full research runs.", topic)), nil
}

// immediateResponse returns a channel with a single text response, then closes.
func immediateResponse(text string) <-chan pkgtui.StreamMsg {
	ch := make(chan pkgtui.StreamMsg, 2)
	ch <- pkgtui.TextDelta{Text: text}
	ch <- pkgtui.StreamDone{}
	close(ch)
	return ch
}

// asyncResponse runs fn in a goroutine and streams the result.
func asyncResponse(fn func() string) <-chan pkgtui.StreamMsg {
	ch := make(chan pkgtui.StreamMsg, 2)
	go func() {
		defer close(ch)
		result := fn()
		ch <- pkgtui.TextDelta{Text: result}
		ch <- pkgtui.StreamDone{}
	}()
	return ch
}
