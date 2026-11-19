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
	return f.recordOwnerAttentionState(key, attention, false)
}

// clearAttempt is used only with a proven missing project before creation.
func (f *CardFiler) recordOwnerAttentionState(key, attention string, clearAttempt bool) error {
	s, p, done, e := f.openState()
	if e != nil {
		return e
	}
	defer done()
	v, ok := s[key]
	// The caller's proof used an earlier snapshot. A fresh creation, update or
	// delivery marker must prevent clearing the reservation under this lock.
	clearAttempt = clearAttempt && !v.Update && v.CreationKey == "" && v.DeliveredCard == "" && v.DeliveredAt == "" && v.DeliveryPendingAt == ""
	if !ok || (v.OwnerAttention == attention && (!clearAttempt || !v.Attempted)) {
		return nil
	}
	if clearAttempt {
		v.Attempted = false
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
