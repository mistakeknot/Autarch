package homeask

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func (f *CardFiler) recordOwnerAttention(key, attention string) error {
	return f.updateOwnerAttention(key, func(*filingEntry) string { return attention })
}

// Build the remediation and clear the reservation from the same locked entry.
func (f *CardFiler) recordProjectOwnerAttention(key, owner string, cause error) (*releaseOwnerError, error) {
	attention := &releaseOwnerError{owner: owner, cause: cause}
	err := f.updateOwnerAttention(key, func(v *filingEntry) string {
		safe := !v.Update && v.CreationKey == "" && v.DeliveredCard == "" && v.DeliveredAt == "" && v.DeliveryPendingAt == ""
		switch {
		case v.Update:
			attention.remediation = "ask the vizier to check the existing card and pending update before cancelling or submitting another update"
		case !safe:
			attention.remediation = "ask the vizier to check whether the card exists before cancelling or filing another ask"
		default:
			v.Attempted = false
			attention.remediation = "run needs-mk cancel-held " + key + ", then file the corrected ask against a live project"
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
	attempted := v.Attempted
	attention := build(&v)
	if v.OwnerAttention == attention && v.Attempted == attempted {
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
