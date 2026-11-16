package homeask

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
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

	project, _ := r.Ask["project"].(string)
	var keys []string
	for key, e := range s {
		priorKey, _ := e.Request.Ask["ask_key"].(string)
		if e.Held && !e.Attempted && !e.Refunded && ordinary(e.Request) && ((askKey != "" && priorKey == askKey) || key == r.replacesHeld) && e.Project == strings.ToLower(project) && e.Request.Project == r.Project && e.Request.Thread == r.Thread && e.Request.Owner == r.Owner {
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
	result, err = f.replayDeliveredEntry(ctx, e)
	return result, true, err
}

func (f *CardFiler) replayDeliveredEntry(ctx context.Context, e filingEntry) (CardResult, error) {
	key := e.DeliveredKey
	if key == "" {
		key = e.Request.Key
	}
	path, err := f.lockPath(key)
	if err != nil {
		return CardResult{}, err
	}
	done, err := lockFile(path)
	if err != nil {
		return CardResult{}, err
	}
	defer done()
	identity := e.Identity
	if e.DeliveredCard != "" {
		shown, err := f.show(ctx, e.DeliveredCard)
		if err != nil {
			return CardResult{}, err
		}
		line, ok := requestOf(shown.Task.Description)
		if !ok || line.Key != key {
			return CardResult{}, fmt.Errorf("%w: delivered card request changed", ErrInvalid)
		}
		identity = line.Identity
	}
	candidate, err := f.locate(ctx, key, identity)
	if err != nil {
		return CardResult{}, err
	}
	if candidate == nil {
		return CardResult{}, ErrOutcomeUnknown
	}
	if e.DeliveredCard != "" && candidate.ID != e.DeliveredCard {
		return CardResult{}, fmt.Errorf("%w: delivered card changed", ErrInvalid)
	}
	req := e.Request
	req.Key = key
	result, err := f.replay(ctx, req, candidate)
	if err == nil && !req.Pull && f.ReadbackTimeout > 0 {
		result, err = f.withCardKey(ctx, result)
	}
	return result, err
}

// An internal retry releases the current delivery lock before switching keys,
// preserving the delivery-key -> retirement-key -> state lock order.
type heldRequestRetry struct {
	request    CardRequest
	supersedes string
	update     bool
}

func (*heldRequestRetry) Error() string { return "follow held request replacement" }
func retryHeld(e filingEntry) *heldRequestRetry {
	request := e.Request
	if !e.Held && !e.Refunded && e.DeliveredCard != "" {
		request.replayEntry = &e
	}
	return &heldRequestRetry{request: request, update: e.DeliveredCard == "" && e.Update, supersedes: e.Supersedes}
}

func (e filingEntry) canPromote(r CardRequest, coalesce bool) bool {
	return e.Held && !e.Attempted && !e.Refunded && ordinary(e.Request) && !r.NonBlocking && !coalesce && (!ordinary(r) || e.HeldReason == "weekday-window")
}

func (e filingEntry) matchesHeldRequest(r CardRequest, identity string) error {
	want := heldRevision{Identity: e.Identity, Thread: e.Request.Thread, Owner: e.Request.Owner}
	if revision, ok := e.HeldRevisions[r.Key]; ok {
		want = revision
	} else if r.Key != e.Request.Key {
		return errReused
	}
	if want.Identity != identity || want.Thread != r.Thread || want.Owner != r.Owner || e.Request.Project != r.Project {
		return errReused
	}
	return nil
}

func (e filingEntry) promote(r CardRequest, identity string) (*heldRequestRetry, error) {
	// Adding the root-run envelope is an escalation; changing the ask body under
	// an existing request key remains a reuse error.
	if r.RootRun != nil && e.Request.RootRun == nil {
		base := r
		base.RootRun = nil
		var err error
		identity, err = RequestIdentity(base)
		if err != nil {
			return nil, err
		}
	}
	if err := e.matchesHeldRequest(r, identity); err != nil {
		return nil, err
	}
	next := e.Request
	next.Urgent, next.KeepList, next.NonBlocking = r.Urgent, r.KeepList, r.NonBlocking
	if r.Type != "" {
		next.Type = r.Type
	}
	if r.RootRun != nil {
		next.RootRun = r.RootRun
	}
	nextIdentity, err := RequestIdentity(next)
	if err != nil {
		return nil, err
	}
	next.Key = uuid.NewSHA1(needsMkNS, []byte(fmt.Sprintf("held-escalation\x00%s\x00%s\x00%t\x00%s\x00%s", e.Request.Key, nextIdentity, next.Urgent, next.KeepList, next.Type))).String()
	next.replacesHeld = e.Request.Key
	return &heldRequestRetry{request: next, supersedes: e.Supersedes, update: e.Update}, nil
}

func (s filingState) retiredReplay(e filingEntry, r CardRequest, identity string) (*heldRequestRetry, error) {
	if !e.Refunded {
		return nil, nil
	}
	key := e.ReplacedBy
	if key == "" && strings.HasPrefix(e.TerminalReason, "replaced by request ") {
		key = strings.TrimPrefix(e.TerminalReason, "replaced by request ")
	}
	if key == "" {
		return nil, nil
	}
	target, ok := s[key]
	if !ok {
		return nil, ErrOutcomeUnknown
	}
	if r.RootRun != nil && e.Request.RootRun == nil && rootRunEqual(r.RootRun, target.Request.RootRun) {
		base := r
		base.RootRun = nil
		var err error
		identity, err = RequestIdentity(base)
		if err != nil {
			return nil, err
		}
	}
	if err := e.matchesHeldRequest(r, identity); err != nil {
		return nil, err
	}
	return retryHeld(target), nil
}

// A release or another escalation may have won while this request switched
// delivery locks. Follow that result rather than create a second card.
func (s filingState) escalationSource(r CardRequest) (*heldRequestRetry, error) {
	if r.replacesHeld == "" {
		return nil, nil
	}
	source, ok := s[r.replacesHeld]
	if !ok {
		return nil, ErrOutcomeUnknown
	}
	if source.Held && !source.Attempted && !source.Refunded {
		return nil, nil
	}
	if source.Refunded && source.ReplacedBy != "" {
		target, ok := s[source.ReplacedBy]
		if !ok {
			return nil, ErrOutcomeUnknown
		}
		return retryHeld(target), nil
	}
	if !source.Held && source.DeliveredCard != "" {
		return retryHeld(source), nil
	}
	return nil, ErrOutcomeUnknown
}
