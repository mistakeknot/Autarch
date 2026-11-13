package homeask

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// FileOrUpdateCard is the CLI's ask-key editing lane. Request lines and routing
// comments are immutable: Home uses them to recognize a valid new generation.
// Other filers retain FileCard's exact-request replay protocol.
func (f *CardFiler) FileOrUpdateCard(ctx context.Context, req CardRequest, supersedes string) (CardResult, error) {
	key, _ := req.Ask["ask_key"].(string)
	if key == "" && supersedes == "" {
		return f.FileCard(ctx, req)
	}
	if req.Owner != "" {
		if err := f.checkOwnerLive(ctx, req.Owner); err != nil {
			return CardResult{}, err
		}
	}
	desc, _, err := req.Description()
	if err != nil {
		return CardResult{}, err
	}
	var projects struct {
		Projects []struct{ ID, Name, Prefix string }
	}
	if err = f.call(ctx, "", &projects, "tasks", "project", "list", "--json"); err != nil {
		return CardResult{}, err
	}
	project := ""
	for _, p := range projects.Projects {
		if p.ID == req.Project || strings.EqualFold(p.Name, req.Project) || strings.EqualFold(p.Prefix, req.Project) {
			project = p.ID
			break
		}
	}
	if project == "" {
		return CardResult{}, fmt.Errorf("%w: unknown tasks project", ErrInvalid)
	}
	lockKey := fmt.Sprintf("edit-%x", sha256.Sum256([]byte(project)))
	path, err := f.lockPath(lockKey)
	if err != nil {
		return CardResult{}, err
	}
	unlock, err := lockFile(path)
	if err != nil {
		return CardResult{}, err
	}
	defer unlock()
	var candidates []shown
	if supersedes != "" {
		s, e := f.show(ctx, supersedes)
		if e != nil {
			return CardResult{}, fmt.Errorf("%w: --supersedes card unavailable: %v", ErrInvalid, e)
		}
		candidates = append(candidates, s)
	} else {
		cursor := ""
		seen := map[string]bool{}
		for page := 0; ; page++ {
			if page >= maxPages {
				return CardResult{}, fmt.Errorf("%w: tasks search did not finish", ErrHomeDown)
			}
			args := []string{"tasks", "list", "--status", searchStatuses, "--project", project, "--limit", pageLimit, "--json"}
			if cursor != "" {
				args = append(args, "--cursor", cursor)
			}
			var rows struct {
				Tasks      []taskRow
				NextCursor *string
			}
			if err = f.call(ctx, "", &rows, args...); err != nil {
				return CardResult{}, err
			}
			for _, r := range rows.Tasks {
				s, e := f.show(ctx, r.ID)
				if e != nil {
					return CardResult{}, e
				}
				c, e := ParseCard(s.Task.Description)
				if e != nil {
					continue
				}
				if c.Ask["ask_key"] == key && s.Task.ProjectID == project {
					candidates = append(candidates, s)
				}
			}
			if rows.NextCursor == nil || *rows.NextCursor == "" {
				break
			}
			cursor = *rows.NextCursor
			if seen[cursor] {
				return CardResult{}, fmt.Errorf("%w: tasks cursor did not advance", ErrHomeDown)
			}
			seen[cursor] = true
		}
	}
	closed := false
	for _, s := range candidates {
		c, e := ParseCard(s.Task.Description)
		if e != nil || c.Pull != "" || s.Task.ProjectID != project {
			return CardResult{}, fmt.Errorf("%w: --supersedes must name a Home card in the same project", ErrInvalid)
		}
		owner := AskingThread(s.Comments)
		if owner == "" || (owner != req.Thread && owner != req.Owner) {
			return CardResult{}, fmt.Errorf("%w: card must belong to the filing thread or --owner-thread", ErrInvalid)
		}
		v, e := f.Card(ctx, s.Task.ID)
		if e != nil {
			return CardResult{}, e
		}
		if v.State == "ruled" || v.DecisionState == "ruled" || v.DecisionState == "picked" {
			return CardResult{}, fmt.Errorf("%w: card %s", ErrAlreadyRuled, s.Task.ID)
		}
		if s.Task.Status == "done" || s.Task.Status == "canceled" || v.State == "closed" {
			if supersedes != "" {
				return CardResult{}, fmt.Errorf("%w: --supersedes requires an open card", ErrInvalid)
			}
			closed = true
			continue
		}
		if v.State != "open" {
			return CardResult{}, fmt.Errorf("%w: card state unavailable", ErrHomeDown)
		}
		// Preserve Home's immutable registry identity, even for --supersedes.
		fresh, _ := ParseCard(desc)
		desc = strings.Replace(desc, fmt.Sprintf("Request: %s sha256:%s", fresh.Request.Key, fresh.Request.Identity), fmt.Sprintf("Request: %s sha256:%s", c.Request.Key, c.Request.Identity), 1)
		if _, e = ParseCard(desc); e != nil {
			return CardResult{}, fmt.Errorf("%w: updated card does not parse: %v", ErrInvalid, e)
		}
		if req.Move != nil {
			if e = f.ensureLabelNamed(ctx, project, MoveLabel); e != nil {
				return CardResult{}, e
			}
		}
		dir, e := os.MkdirTemp("", "autarch-needsmk-")
		if e != nil {
			return CardResult{}, e
		}
		defer os.RemoveAll(dir)
		file := filepath.Join(dir, "description.md")
		if e = os.WriteFile(file, []byte(desc), 0600); e != nil {
			return CardResult{}, e
		}
		args := []string{"tasks", "update", s.Task.ID, "--title", req.Title, "--description-file", file, "--json"}
		if req.Move != nil {
			args = append(args, "--add-label", MoveLabel)
		} else if c.Move != nil {
			args = append(args, "--remove-label", MoveLabel)
		}
		if e = f.call(ctx, req.Thread, nil, args...); e != nil {
			return CardResult{}, e
		}
		res := CardResult{ID: s.Task.ID, Key: c.Request.Key, Project: project, Updated: true, Replay: true}
		if f.ReadbackTimeout > 0 {
			res.CardKey = f.awaitCardKey(ctx, res.ID)
		}
		return res, nil
	}
	if closed {
		req.Key = uuid.NewString()
	}
	return f.FileCard(ctx, req)
}
