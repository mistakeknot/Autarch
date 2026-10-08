package homeask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AdoptResult is what adopting a move onto an existing card returns.
type AdoptResult struct {
	ID string `json:"card"`
	// Replay is true when the card already carried exactly this move.
	Replay bool `json:"replay"`
}

// AdoptMove attaches a home-move/v1 block to a card that already exists and labels it mk-move. Only the
// card's asking thread may adopt, the card must already parse as a card, and a card that carries a
// different move is refused (a changed move is a new generation, filed by editing the card on purpose).
// The text it writes is the old description plus the one block, and it is read back before it is sent.
func (f *CardFiler) AdoptMove(ctx context.Context, taskID, thread string, move map[string]any) (AdoptResult, error) {
	if thread == "" {
		return AdoptResult{}, fmt.Errorf("%w: BB_THREAD_ID is required: only the asking thread may adopt a move", ErrInvalid)
	}
	if strings.TrimSpace(taskID) == "" {
		return AdoptResult{}, fmt.Errorf("%w: a card id is required", ErrInvalid)
	}
	req := CardRequest{Move: move}
	blk, err := req.moveBlockChecked()
	if err != nil {
		return AdoptResult{}, err
	}
	s, err := f.show(ctx, taskID)
	if err != nil {
		return AdoptResult{}, err
	}
	card, err := ParseCard(s.Task.Description)
	if err != nil {
		return AdoptResult{}, fmt.Errorf("%w: card %s does not parse as a Home card (%v); nothing was changed", ErrInvalid, taskID, err)
	}
	if card.Pull != "" {
		return AdoptResult{}, fmt.Errorf("%w: card %s is a pull card; a move cannot be adopted onto it", ErrInvalid, taskID)
	}
	if asking := AskingThread(s.Comments); asking != thread {
		return AdoptResult{}, fmt.Errorf("%w: card %s was filed from %q, not this thread", ErrInvalid, taskID, asking)
	}
	if card.Move != nil {
		if moveEqual(card.Move, move) {
			return AdoptResult{ID: taskID, Replay: true}, nil
		}
		return AdoptResult{}, fmt.Errorf("%w: card %s already carries a different move", ErrInvalid, taskID)
	}
	desc := strings.TrimRight(s.Task.Description, "\r\n") + "\n\n" + blk
	back, err := ParseCard(desc)
	if err != nil || !moveEqual(back.Move, move) || back.Request != card.Request || back.Question != card.Question {
		return AdoptResult{}, fmt.Errorf("%w: the card does not read back the same with the move added; nothing was changed", ErrInvalid)
	}
	if err := f.ensureLabelNamed(ctx, s.Task.ProjectID, MoveLabel); err != nil {
		return AdoptResult{}, err
	}
	dir, err := os.MkdirTemp("", "autarch-needsmk-")
	if err != nil {
		return AdoptResult{}, fmt.Errorf("%w: %v", ErrHomeDown, err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "description.md")
	if err := os.WriteFile(file, []byte(desc), 0o600); err != nil {
		return AdoptResult{}, fmt.Errorf("%w: %v", ErrHomeDown, err)
	}
	r := f.exec(ctx, cardEnv(thread), "tasks", "update", taskID, "--description-file", file, "--add-label", MoveLabel, "--json")
	if r.failed() {
		return AdoptResult{}, fmt.Errorf("%w: bb tasks update %s: %s", ErrHomeDown, taskID, r.text())
	}
	return AdoptResult{ID: taskID}, nil
}
