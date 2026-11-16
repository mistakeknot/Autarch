package homeask

import (
	"context"
	"errors"
	"fmt"
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

// knownHeldRequest preserves exact request replay before ask-key matching.
func (s filingState) knownHeldRequest(r CardRequest) (string, bool) {
	if _, exists := s[r.Key]; exists {
		return r.Key, true
	}
	for key, e := range s {
		if _, exists := e.HeldRevisions[r.Key]; exists {
			return key, true
		}
	}
	return r.Key, false
}

// heldTarget merges new keys only while the incoming request still needs holding.
func (s filingState) heldTarget(r CardRequest, coalesce bool) string {
	if key, known := s.knownHeldRequest(r); known {
		return key
	}
	if coalesce {
		if keys := s.matchingHeld(r); len(keys) > 0 {
			return keys[0]
		}
	}
	return r.Key
}

// Coalescing and retirement share scope, but an incoming request that can be
// delivered now must never become a held revision (mk-bzi8s.6).
func (s filingState) retiringHeld(r CardRequest, coalesce bool) []string {
	if _, known := s.knownHeldRequest(r); known || coalesce {
		return nil
	}
	return s.matchingHeld(r)
}

func (s filingState) matchingHeld(r CardRequest) []string {
	askKey, _ := r.Ask["ask_key"].(string)
	if askKey == "" {
		return nil
	}
	project, _ := r.Ask["project"].(string)
	var keys []string
	for key, e := range s {
		priorKey, _ := e.Request.Ask["ask_key"].(string)
		if e.Held && !e.Attempted && !e.Refunded && ordinary(e.Request) && priorKey == askKey && e.Project == strings.ToLower(project) && e.Request.Project == r.Project && e.Request.Thread == r.Thread && e.Request.Owner == r.Owner {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
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
		return CardResult{}, true, fmt.Errorf("%w: request retired: %s", ErrInvalid, e.TerminalReason)
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
