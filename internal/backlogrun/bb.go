package backlogrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner runs a command and returns its stdout. Tests replace it.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner runs a real command; a failure carries its stderr.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// BB is the bb CLI as a BoardSource, Notifier and Pool.
type BB struct {
	Run        Runner
	VizierTell string // path of vizier-tell
	TempDir    string // where message files are written
}

type trackerProject struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	BBProj string `json:"linkedBbProjectId"`
}

func (b BB) projects(ctx context.Context) ([]trackerProject, error) {
	out, err := b.Run(ctx, "bb", "tasks", "project", "list", "--json")
	if err != nil {
		return nil, err
	}
	var r struct {
		Projects []trackerProject `json:"projects"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("bb tasks project list: %w", err)
	}
	return r.Projects, nil
}

type listedTask struct {
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Desc      string   `json:"description"`
	Status    string   `json:"status"`
	Priority  string   `json:"priority"`
	UpdatedAt string   `json:"updatedAt"`
	Labels    []string `json:"labels"`
	Working   int      `json:"agentsWorking"`
}

func (b BB) list(ctx context.Context, prefix, statuses string) ([]listedTask, error) {
	var all []listedTask
	cursor := ""
	for page := 0; page < 100; page++ {
		args := []string{"tasks", "list", "--project", prefix, "--status", statuses, "--limit", "500", "--json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		out, err := b.Run(ctx, "bb", args...)
		if err != nil {
			return nil, err
		}
		var r struct {
			Tasks      []listedTask `json:"tasks"`
			NextCursor string       `json:"nextCursor"`
		}
		if err := json.Unmarshal(out, &r); err != nil {
			return nil, fmt.Errorf("bb tasks list %s: %w", prefix, err)
		}
		all = append(all, r.Tasks...)
		if r.NextCursor == "" || len(r.Tasks) == 0 {
			return all, nil
		}
		cursor = r.NextCursor
	}
	return nil, fmt.Errorf("bb tasks list %s: more than 100 pages", prefix)
}

// Cards returns every backlog and todo card on every board. A board that fails to list fails the run: a partial
// snapshot would make cards look gone.
func (b BB) Cards(ctx context.Context) ([]Card, error) {
	ps, err := b.projects(ctx)
	if err != nil {
		return nil, err
	}
	var cards []Card
	for _, p := range ps {
		ts, err := b.list(ctx, p.Prefix, "backlog,todo")
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			cards = append(cards, Card{Key: t.Key, Board: p.Prefix, Title: t.Title, Description: t.Desc, Status: t.Status,
				Priority: t.Priority, UpdatedAt: t.UpdatedAt, Labels: t.Labels, Working: t.Working})
		}
	}
	return cards, nil
}

// InFlight counts, per board, cards in progress or in review.
func (b BB) InFlight(ctx context.Context) (map[string]int, error) {
	ps, err := b.projects(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, p := range ps {
		ts, err := b.list(ctx, p.Prefix, "in_progress,in_review")
		if err != nil {
			return nil, err
		}
		if len(ts) > 0 {
			out[p.Prefix] = len(ts)
		}
	}
	return out, nil
}

// Coordinator finds the one live thread titled "<board name> | coordinator". Zero or several matches give "".
func (b BB) Coordinator(ctx context.Context, board string) (string, error) {
	ps, err := b.projects(ctx)
	if err != nil {
		return "", err
	}
	name := ""
	for _, p := range ps {
		if p.Prefix == board {
			name = p.Name
		}
	}
	if name == "" {
		return "", nil
	}
	out, err := b.Run(ctx, "bb", "thread", "list", "--json")
	if err != nil {
		return "", err
	}
	var ts []struct {
		ID         string  `json:"id"`
		Title      *string `json:"title"`
		ArchivedAt *int64  `json:"archivedAt"`
		DeletedAt  *int64  `json:"deletedAt"`
	}
	if err := json.Unmarshal(out, &ts); err != nil {
		return "", fmt.Errorf("bb thread list: %w", err)
	}
	want := strings.ToLower(name + " | coordinator")
	found := ""
	for _, t := range ts {
		if t.Title == nil || t.ArchivedAt != nil || t.DeletedAt != nil || strings.ToLower(strings.TrimSpace(*t.Title)) != want {
			continue
		}
		if found != "" {
			return "", nil
		}
		found = t.ID
	}
	return found, nil
}

func (b BB) msgFile(message string) (string, error) {
	f, err := os.CreateTemp(b.TempDir, "backlog-msg-*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(message); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// Tell sends a message to a coordinator thread.
func (b BB) Tell(ctx context.Context, thread, message string) error {
	p, err := b.msgFile(message)
	if err != nil {
		return err
	}
	defer os.Remove(p)
	_, err = b.Run(ctx, "bb", "thread", "tell", thread, "--message-file", p)
	return err
}

// TellVizier sends the one per-run message to the vizier.
func (b BB) TellVizier(ctx context.Context, message string) error {
	p, err := b.msgFile(message)
	if err != nil {
		return err
	}
	defer os.Remove(p)
	_, err = b.Run(ctx, b.VizierTell, "--message-file", p)
	return err
}

// Headroom reads `bb pool status --json`. Low headroom is: nothing accepting, or every enabled account at or over
// 85 percent of its five-hour or seven-day window, or held.
func (b BB) Headroom(ctx context.Context) (bool, string, error) {
	out, err := b.Run(ctx, "bb", "pool", "status", "--json")
	if err != nil {
		return false, "", err
	}
	return headroomFrom(out)
}

func headroomFrom(raw []byte) (bool, string, error) {
	var s struct {
		Accepting *bool `json:"accepting"`
		Accounts  []struct {
			Enabled bool     `json:"enabled"`
			Five    *float64 `json:"fiveHourUtilization"`
			Seven   *float64 `json:"sevenDayUtilization"`
			Held    any      `json:"heldUntil"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return false, "", fmt.Errorf("bb pool status: %w", err)
	}
	if s.Accepting == nil {
		return false, "", fmt.Errorf("bb pool status: no accepting field")
	}
	if !*s.Accepting {
		return false, "pool is not accepting work", nil
	}
	roomy := 0
	for _, a := range s.Accounts {
		if !a.Enabled || a.Held != nil {
			continue
		}
		if (a.Five != nil && *a.Five >= 0.85) || (a.Seven != nil && *a.Seven >= 0.85) {
			continue
		}
		roomy++
	}
	if roomy == 0 {
		return false, "no enabled account is under 85 percent of its windows", nil
	}
	return true, "", nil
}
