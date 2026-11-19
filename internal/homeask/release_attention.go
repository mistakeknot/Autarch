package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"
)

func (f *CardFiler) recordOwnerAttention(key, attention string) error {
	return f.updateOwnerAttention(key, func(*filingEntry) string { return attention })
}

type projectAbsenceProof struct {
	snapshot filingEntry
	found    *found
	complete bool
	result   CardResult
}

// Lookup is unscoped and outside the state lock. Only an unchanged fresh entry
// can consume the proof below; a timeout never proves absence.
func (f *CardFiler) reconcileMissingProject(ctx context.Context, key, owner string, cause error) (CardResult, error) {
	state, _, done, err := f.openState()
	if err != nil {
		return CardResult{}, err
	}
	done()
	entry, ok := state[key]
	proof := &projectAbsenceProof{snapshot: entry}
	var lookupErr error
	if ok && entry.Attempted {
		keys := []string{key}
		if entry.CreationKey != "" && entry.CreationKey != key {
			keys = append([]string{entry.CreationKey}, keys...)
		}
		if entry.DeliveredKey != "" && entry.DeliveredKey != key && entry.DeliveredKey != entry.CreationKey {
			keys = append(keys, entry.DeliveredKey)
		}
		for _, requestKey := range keys {
			proof.found, lookupErr = f.locate(ctx, requestKey, entry.Identity)
			if lookupErr != nil || proof.found != nil {
				break
			}
		}
		if lookupErr == nil && proof.found == nil && entry.DeliveredCard != "" {
			card, showErr := f.show(ctx, entry.DeliveredCard)
			if showErr == nil {
				request, valid := requestOf(card.Task.Description)
				if !valid {
					lookupErr = fmt.Errorf("%w: recorded delivery has no readable request", ErrInvalid)
				} else {
					proof.found = &found{ID: card.Task.ID, ProjectID: card.Task.ProjectID, Req: request}
				}
			} else if !errors.Is(showErr, errTaskGone) {
				lookupErr = showErr
			}
		}
		proof.complete = lookupErr == nil
	}
	attention, recordErr := f.recordProjectOwnerAttention(key, owner, errors.Join(cause, lookupErr), proof)
	if proof.result.ID != "" {
		proof.result.ownerAttention = attention.Error()
		return proof.result, recordErr
	}
	return CardResult{}, errors.Join(attention, recordErr)
}

// Build the remediation and clear the reservation from the same locked entry.
func (f *CardFiler) recordProjectOwnerAttention(key, owner string, cause error, proofs ...*projectAbsenceProof) (*releaseOwnerError, error) {
	attention := &releaseOwnerError{owner: owner, cause: cause, remediation: "request no longer exists in held state; inspect release results before filing another ask"}
	err := f.updateOwnerAttention(key, func(v *filingEntry) string {
		var proof *projectAbsenceProof
		if len(proofs) > 0 {
			proof = proofs[0]
		}
		safe := !v.Attempted && proof != nil && reflect.DeepEqual(*v, proof.snapshot)
		verified := proof != nil && proof.complete && reflect.DeepEqual(*v, proof.snapshot)
		switch {
		case verified && proof.found != nil:
			card := proof.found
			v.Held = false
			v.DeliveredCard = card.ID
			v.DeliveredKey = card.Req.Key
			v.DeliveredProject = card.ProjectID
			if f.anyPilot() && v.DeliveredAt == "" {
				v.DeliveredAt = f.clock().Format(time.RFC3339Nano)
			}
			v.DeliveryPendingAt = ""
			proof.result = CardResult{ID: card.ID, Key: card.Req.Key, Project: card.ProjectID, Replay: true}
			attention.remediation = "card " + card.ID + " was already delivered; ask the vizier to arrange handoff of the existing card; do not cancel-held or duplicate it"
		case verified || safe:
			v.Attempted = false
			attention.remediation = "run needs-mk cancel-held " + key + ", then file the corrected ask against a live project"
			if v.Update {
				attention.remediation = "run needs-mk cancel-held " + key + ", then inspect the existing card before submitting a corrected update; do not create a duplicate card"
			}
		case v.Attempted && proof != nil:
			attention.remediation = "run needs-mk release once bb is reachable to retry the project-wide card check; do not cancel or refile before it completes"
		case v.Update:
			attention.remediation = "ask the vizier to check the existing card and pending update before cancelling or submitting another update"
		default:
			attention.remediation = "ask the vizier to check whether the card exists before cancelling or filing another ask"
		}
		return attention.Error()
	})
	return attention, err
}

func (f *CardFiler) updateOwnerAttention(key string, build func(*filingEntry) string) error {
	s, p, done, err := f.openState()
	if err != nil {
		return err
	}
	defer done()
	v, ok := s[key]
	if !ok {
		return nil
	}
	before := v
	attention := build(&v)
	if v.OwnerAttention == attention && reflect.DeepEqual(v, before) {
		return nil
	}
	v.OwnerAttention = attention
	s[key] = v
	return saveFilingState(p, s)
}

// PrintReleaseOwnerAttention reads the release's persisted notices without taking a lock or writing state.
func (f *CardFiler) PrintReleaseOwnerAttention(w io.Writer) error {
	d, e := f.stateDir()
	if e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(d, "filings.json"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var s filingState
	if e = json.Unmarshal(b, &s); e != nil {
		return e
	}
	keys := []string{}
	for k, v := range s {
		if v.OwnerAttention != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil
	}
	if _, e = fmt.Fprintln(w, "Release owner attention:"); e != nil {
		return e
	}
	for _, key := range keys {
		if _, e = fmt.Fprintf(w, "  %s: %s\n", key, s[key].OwnerAttention); e != nil {
			return e
		}
	}
	return nil
}

// DismissReleaseOwnerAttention acknowledges a notice without cancelling a request
// or changing its delivery, ownership, terminal state or reporting record.
func (f *CardFiler) DismissReleaseOwnerAttention(key string) error {
	if err := CheckRequestKey(key); err != nil {
		return err
	}
	dir, err := f.stateDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	keyDone, err := lockFile(filepath.Join(dir, "delivery-"+key+".lock"))
	if err != nil {
		return err
	}
	defer keyDone()
	state, path, done, err := f.openState()
	if err != nil {
		return err
	}
	defer done()
	entry, ok := state[key]
	if !ok {
		return fmt.Errorf("%w: no request %s", ErrInvalid, key)
	}
	if entry.OwnerAttention == "" {
		return nil
	}
	entry.OwnerAttention = ""
	state[key] = entry
	return saveFilingState(path, state)
}
