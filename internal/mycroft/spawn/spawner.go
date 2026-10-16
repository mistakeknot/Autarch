// Package spawn starts card workers as bb threads. It replaces the earlier tmux session spawner: a worker is a
// thread in the card's project and environment, with the card attached, run under a seat that Clavain routing
// chooses at dispatch time.
package spawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// DefaultRole is the routing role a card worker runs under. Routing decides the model and effort for it.
const DefaultRole = "routine-execution"

// providers maps the backend routing names to the bb provider id that runs it. It is a transport name map, not a seat.
var providers = map[string]string{"claude": "claude-code", "codex": "codex"}

// Receipt records one dispatch: the seat that routing gave and the thread it started.
type Receipt struct {
	At       time.Time `json:"at"`
	Thread   string    `json:"thread"`
	Bead     string    `json:"bead"`
	Card     string    `json:"card,omitempty"`
	Project  string    `json:"project"`
	Seat     Seat      `json:"seat"`
	Provider string    `json:"provider"`
}

// BBSpawner creates card workers as bb threads through the bb CLI.
type BBSpawner struct {
	Project     string // bb project id the thread belongs to
	Environment string // existing environment id or workspace path
	Role        string // routing role; DefaultRole when empty
	Resolver    SeatResolver
	ReceiptFile string // JSONL receipts, one line per dispatch; empty keeps them in memory only

	run     Runner
	mu      sync.Mutex
	details map[string]Receipt
}

// NewBBSpawner builds a spawner that resolves seats with ic against the selected Clavain routing policy.
func NewBBSpawner(project, environment, receiptFile string) *BBSpawner {
	return &BBSpawner{Project: project, Environment: environment, Role: DefaultRole, Resolver: NewICResolver(), ReceiptFile: receiptFile, run: execRunner, details: map[string]Receipt{}}
}

// Title names the card and the bead.
func Title(bead mycroft.BeadView) string {
	if bead.Card != "" {
		return fmt.Sprintf("%s | %s | %s", bead.Card, bead.ID, bead.Title)
	}
	return fmt.Sprintf("%s | %s", bead.ID, bead.Title)
}

// Brief is the opening prompt for the worker, built from the card and bead.
func Brief(bead mycroft.BeadView, cardText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are a card worker started by Mycroft for bead %s", bead.ID)
	if bead.Card != "" {
		fmt.Fprintf(&b, " and card %s", bead.Card)
	}
	fmt.Fprintf(&b, ".\n\nTitle: %s\nType: %s  Priority: P%d\n", bead.Title, bead.Type, bead.Priority)
	if len(bead.Labels) > 0 {
		fmt.Fprintf(&b, "Labels: %s\n", strings.Join(bead.Labels, ", "))
	}
	if cardText != "" {
		b.WriteString("\nCard description (from the task board):\n\n")
		b.WriteString(cardText)
		b.WriteString("\n")
	}
	b.WriteString("\nRead the card and the bead first (bb tasks show, bd show) in case the description above is cut short. Work only what they describe, in this environment. ")
	b.WriteString("Publication, merges, deploys and anything that needs mk follow the repository's rules: report instead of acting. ")
	b.WriteString("End with a short report of what changed and the evidence.\n")
	return b.String()
}

// Plan resolves the seat and builds the thread title and brief without starting anything. It is what a suggestion shows.
func (s *BBSpawner) Plan(bead mycroft.BeadView) (title string, seat Seat, brief string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	role := s.Role
	if role == "" {
		role = DefaultRole
	}
	seat, err = s.Resolver.Resolve(ctx, role)
	if err != nil {
		return "", Seat{}, "", fmt.Errorf("seat unresolved: %w", err)
	}
	return Title(bead), seat, Brief(bead, ""), nil
}

// Spawn resolves the seat, creates the thread and attaches the card. If routing cannot resolve the role it returns
// an error before any bb call, so nothing is dispatched. contextFile is unused (kept for the AgentSpawner interface).
func (s *BBSpawner) Spawn(agentName string, bead mycroft.BeadView, _ string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if s.Project == "" || s.Environment == "" {
		return "", errors.New("spawn: project and environment are required")
	}
	if bead.Card == "" {
		return "", errors.New("spawn refused: a card worker needs the card key, so the thread can be attached to it")
	}
	role := s.Role
	if role == "" {
		role = DefaultRole
	}
	seat, err := s.Resolver.Resolve(ctx, role)
	if err != nil {
		return "", fmt.Errorf("spawn refused, seat unresolved: %w", err)
	}
	provider, ok := providers[seat.Backend]
	if !ok {
		return "", fmt.Errorf("spawn refused: no bb provider for backend %q", seat.Backend)
	}
	cardText, err := s.cardText(ctx, bead.Card)
	if err != nil {
		return "", fmt.Errorf("spawn refused, card unreadable: %w", err)
	}
	f, err := os.CreateTemp("", "mycroft-brief-*.md")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(Brief(bead, cardText)); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	out, err := s.run(ctx, "bb", "thread", "create", "--json",
		"--project", s.Project, "--environment", s.Environment,
		"--provider", provider, "--model", seat.Model, "--reasoning-level", seat.Effort,
		"--title", Title(bead), "--prompt-file", f.Name())
	if err != nil {
		// The thread may exist even though the command failed (interrupted CLI, lost reply). Stop it if the output
		// names it; otherwise the outcome is unknown and the caller must reconcile before retrying.
		if id, ierr := threadID(out); ierr == nil {
			return "", s.stopAfter(id, fmt.Errorf("thread create failed: %w", err))
		}
		return "", fmt.Errorf("thread create failed, outcome unknown (check the project for a thread titled %q before retrying): %w", Title(bead), err)
	}
	id, err := threadID(out)
	if err != nil {
		return "", fmt.Errorf("thread create answered without a thread id, outcome unknown (check the project for a thread titled %q before retrying): %w", Title(bead), err)
	}
	if _, err := s.run(ctx, "bb", "tasks", "attach", bead.Card, "--thread", id); err != nil {
		return "", s.stopAfter(id, fmt.Errorf("attach card %s to %s: %w", bead.Card, id, err))
	}
	r := Receipt{At: time.Now().UTC(), Thread: id, Bead: bead.ID, Card: bead.Card, Project: s.Project, Seat: seat, Provider: provider}
	if err := s.appendReceipt(r); err != nil {
		// No durable seat receipt means no accepted dispatch: the worker is stopped, not left running.
		return "", s.stopAfter(id, fmt.Errorf("receipt not written: %w", err))
	}
	s.mu.Lock()
	s.details[id] = r
	s.mu.Unlock()
	return id, nil
}

// stopAfter stops a thread that must not keep running, on a fresh context so an expired dispatch deadline does not
// skip the stop, and says whether the stop worked.
func (s *BBSpawner) stopAfter(id string, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := s.run(ctx, "bb", "thread", "stop", id); err != nil {
		return fmt.Errorf("%w; thread %s could NOT be stopped and is still running: %v", cause, id, err)
	}
	return fmt.Errorf("%w (thread %s stopped)", cause, id)
}

// maxCardText bounds the card description placed in the brief.
const maxCardText = 6000

// cardText reads the card from the task board. A card that cannot be read is not dispatched.
func (s *BBSpawner) cardText(ctx context.Context, key string) (string, error) {
	out, err := s.run(ctx, "bb", "tasks", "show", key, "--json")
	if err != nil {
		return "", err
	}
	var v struct {
		Task struct {
			Key         string `json:"key"`
			Description string `json:"description"`
		} `json:"task"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("unreadable reply: %w", err)
	}
	if !strings.EqualFold(v.Task.Key, key) {
		return "", fmt.Errorf("reply is for card %q, not %q", v.Task.Key, key)
	}
	d := []rune(v.Task.Description)
	if len(d) > maxCardText {
		return string(d[:maxCardText]) + "\n[cut]", nil
	}
	return v.Task.Description, nil
}

// Detail describes a dispatched thread for the dispatch log: role, model, effort and policy hash.
func (s *BBSpawner) Detail(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.details[sessionID]
	if !ok {
		return ""
	}
	return fmt.Sprintf("thread %s role=%s model=%s effort=%s policy=%s", r.Thread, r.Seat.Role, r.Seat.Model, r.Seat.Effort, r.Seat.PolicyHash)
}

// Kill stops a worker thread.
func (s *BBSpawner) Kill(sessionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := s.run(ctx, "bb", "thread", "stop", sessionID)
	return err
}

func (s *BBSpawner) appendReceipt(r Receipt) error {
	if s.ReceiptFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.ReceiptFile), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.ReceiptFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, _ := json.Marshal(r)
	_, err = f.Write(append(line, '\n'))
	return err
}

func threadID(out []byte) (string, error) {
	var v struct {
		ID     string `json:"id"`
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("bb thread create: unreadable reply: %w", err)
	}
	if v.ID != "" {
		return v.ID, nil
	}
	if v.Thread.ID != "" {
		return v.Thread.ID, nil
	}
	return "", errors.New("bb thread create: reply has no thread id")
}
