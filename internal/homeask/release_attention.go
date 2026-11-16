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
	s, p, done, e := f.openState()
	if e != nil {
		return e
	}
	defer done()
	v, ok := s[key]
	if !ok || v.OwnerAttention == attention {
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
// or changing its delivery, ownership, terminal state or budget reservation.
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
