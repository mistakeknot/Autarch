package homeask

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// Keep the identity and routing of each admitted request even when its queued
// payload is replaced. A retry must neither undo a later edit nor charge again.
type heldRevision struct {
	Identity string `json:"identity"`
	Thread   string `json:"thread"`
	Owner    string `json:"owner"`
}

var errHeldTargetChanged = errors.New("held update target changed during admission")

// heldTarget resolves a known request first; only a new key may replace an
// unsent payload. Attempted deliveries cannot safely be edited in the queue.
func (s filingState) heldTarget(r CardRequest) string {
	if _, exists := s[r.Key]; exists {
		return r.Key
	}
	for key, e := range s {
		if _, exists := e.HeldRevisions[r.Key]; exists {
			return key
		}
	}
	askKey, _ := r.Ask["ask_key"].(string)
	if askKey == "" {
		return r.Key
	}
	project, _ := r.Ask["project"].(string)
	keys := make([]string, 0)
	for key, e := range s {
		priorKey, _ := e.Request.Ask["ask_key"].(string)
		if e.Held && !e.Attempted && !e.Refunded && priorKey == askKey && e.Project == strings.ToLower(project) && e.Request.Project == r.Project {
			keys = append(keys, key)
		}
	}
	// Existing queues normally have one target. Stable ordering also makes old
	// duplicate rows deterministic without rewriting attempted deliveries.
	if len(keys) > 0 {
		sort.Strings(keys)
		return keys[0]
	}
	return r.Key
}

func (e filingEntry) revisionReplay(key, identity string, r CardRequest) (CardResult, bool, error) {
	revision, known := e.HeldRevisions[key]
	if !known {
		return CardResult{}, false, nil
	}
	if revision.Identity != identity || revision.Thread != r.Thread || revision.Owner != r.Owner || e.Request.Project != r.Project {
		return CardResult{}, true, errReused
	}
	if e.Refunded {
		return CardResult{}, true, errReused
	}
	if e.Held {
		return e.heldResult(e.Request.Key, true), true, nil
	}
	return CardResult{ID: e.DeliveredCard, Key: e.Request.Key, Project: e.Request.Project, CardKey: e.DeliveredCardKey, Replay: true}, true, nil
}

// Delivered revisions still use the normal registry/readback protocol. Remembered
// state alone must not report success for a deleted or changed card.
func (f *CardFiler) replayHeldRevision(ctx context.Context, e filingEntry, r CardRequest, identity string) (CardResult, bool, error) {
	result, known, err := e.revisionReplay(r.Key, identity, r)
	if !known || err != nil || result.Queued {
		return result, known, err
	}
	path, err := f.lockPath(e.Request.Key)
	if err != nil {
		return CardResult{}, true, err
	}
	done, err := lockFile(path)
	if err != nil {
		return CardResult{}, true, err
	}
	defer done()
	candidate, err := f.locate(ctx, e.Request.Key, e.Identity)
	if err != nil {
		return CardResult{}, true, err
	}
	if candidate == nil {
		return CardResult{}, true, ErrOutcomeUnknown
	}
	result, err = f.replay(ctx, e.Request, candidate)
	if err == nil && !e.Request.Pull && f.ReadbackTimeout > 0 {
		result, err = f.withCardKey(ctx, result)
	}
	return result, true, err
}
