package spawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Seat is the model, effort and policy a dispatch runs under. Mycroft holds no seat table: every field comes
// from Clavain routing at dispatch time.
type Seat struct {
	Role       string `json:"role"`
	ProfileRef string `json:"profile_ref"`
	Backend    string `json:"backend"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
	PolicyHash string `json:"policy_hash"`
}

// SeatResolver turns a routing role into a seat. A resolver that cannot answer returns an error and the spawner
// dispatches nothing (fail closed).
type SeatResolver interface {
	Resolve(ctx context.Context, role string) (Seat, error)
}

// Runner executes a command and returns its standard output. Tests stub it.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// ICResolver resolves a role with `ic --json route dispatch` against a Clavain routing policy file.
type ICResolver struct {
	Policy string // path to the selected Clavain config/routing.yaml
	run    Runner
}

// NewICResolver reads the policy path from CLAVAIN_ROUTING_POLICY, or from CLAVAIN_SELECTED_ROOT/config/routing.yaml.
func NewICResolver() *ICResolver {
	p := os.Getenv("CLAVAIN_ROUTING_POLICY")
	if p == "" {
		if root := os.Getenv("CLAVAIN_SELECTED_ROOT"); root != "" {
			p = filepath.Join(root, "config", "routing.yaml")
		}
	}
	return &ICResolver{Policy: p, run: execRunner}
}

type routeReply struct {
	RequestedRole string `json:"requested_role"`
	ProfileRef    string `json:"profile_ref"`
	Profile       struct {
		Backend string `json:"backend"`
		Model   string `json:"model"`
		Effort  string `json:"reasoning_effort"`
	} `json:"profile"`
	PolicyHash        string `json:"policy_hash"`
	FrontierRequired  *bool  `json:"frontier_required"`
	ReviewRequirement string `json:"review_requirement"`
}

// Resolve asks routing for the role with a settled-work decision context.
func (r *ICResolver) Resolve(ctx context.Context, role string) (Seat, error) {
	if role == "" {
		return Seat{}, errors.New("no routing role configured")
	}
	if r.Policy == "" {
		return Seat{}, errors.New("no Clavain routing policy: set CLAVAIN_ROUTING_POLICY or CLAVAIN_SELECTED_ROOT")
	}
	f, err := os.CreateTemp("", "mycroft-route-*.json")
	if err != nil {
		return Seat{}, err
	}
	defer os.Remove(f.Name())
	ctxJSON, _ := json.Marshal(map[string]any{"reasons": []string{}, "rationale": "Mycroft backlog card work, settled and routine"})
	if _, err := f.Write(ctxJSON); err != nil {
		f.Close()
		return Seat{}, err
	}
	f.Close()
	out, err := r.run(ctx, "ic", "--json", "route", "dispatch", "--policy="+r.Policy, "--role="+role, "--context-file="+f.Name())
	if err != nil {
		return Seat{}, fmt.Errorf("route role %q: %w", role, err)
	}
	var rep routeReply
	if err := json.Unmarshal(out, &rep); err != nil {
		return Seat{}, fmt.Errorf("route role %q: unreadable reply: %w", role, err)
	}
	s := Seat{Role: rep.RequestedRole, ProfileRef: rep.ProfileRef, Backend: rep.Profile.Backend, Model: rep.Profile.Model, Effort: rep.Profile.Effort, PolicyHash: rep.PolicyHash}
	if s.Role != role || s.ProfileRef == "" || rep.FrontierRequired == nil || s.Backend == "" || s.Model == "" || s.Effort == "" || s.PolicyHash == "" {
		return Seat{}, fmt.Errorf("route role %q: incomplete answer %+v", role, s)
	}
	if *rep.FrontierRequired {
		return Seat{}, fmt.Errorf("route role %q: needs a frontier seat; a card worker does not dispatch it", role)
	}
	return s, nil
}
