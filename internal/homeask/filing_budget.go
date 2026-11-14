package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const EnvCardWeeklyBudget = "AUTARCH_CARD_WEEKLY_BUDGET"
const EnvCardStateDir = "AUTARCH_CARD_STATE_DIR"

type CardBudget struct {
	Ordinary int `json:"ordinary"`
	Limit    int `json:"limit"`
	Exempt   int `json:"exempt"`
}

// BudgetStatus exposes ordinary and exempt volume for the current ISO week.
func (f *CardFiler) BudgetStatus(project string) (CardBudget, error) {
	limit, err := f.budget()
	if err != nil {
		return CardBudget{}, err
	}
	state, _, unlock, err := f.openState()
	if err != nil {
		return CardBudget{}, err
	}
	defer unlock()
	now := time.Now()
	if f.Now != nil {
		now = f.Now()
	}
	y, w := now.ISOWeek()
	week := fmt.Sprintf("%04d-W%02d", y, w)
	status := CardBudget{Limit: limit}
	for _, e := range state {
		if e.Project == strings.ToLower(project) && e.Week == week {
			if e.Request.Urgent || e.Request.KeepList != "" {
				status.Exempt++
			} else {
				status.Ordinary++
			}
		}
	}
	return status, nil
}

type filingEntry struct {
	Request    CardRequest `json:"request"`
	Identity   string      `json:"identity"`
	Project    string      `json:"project"`
	Week       string      `json:"week"`
	Attempted  bool        `json:"attempted,omitempty"`
	Held       bool        `json:"held"`
	Supersedes string      `json:"supersedes,omitempty"`
	Update     bool        `json:"update"`
}
type filingState map[string]filingEntry

func (f *CardFiler) stateDir() (string, error) {
	if f.StateDir != "" {
		return f.StateDir, nil
	}
	if p := os.Getenv(EnvCardStateDir); p != "" {
		return p, nil
	}
	if f.LockDir != "" {
		return filepath.Join(f.LockDir, "state"), nil
	}
	p := os.Getenv("XDG_STATE_HOME")
	if p == "" {
		h, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		p = filepath.Join(h, ".local", "state")
	}
	return filepath.Join(p, "autarch", "needsmk"), nil
}
func (f *CardFiler) openState() (filingState, string, func(), error) {
	dir, err := f.stateDir()
	if err != nil {
		return nil, "", nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, "", nil, err
	}
	unlock, err := lockFile(filepath.Join(dir, "filing.lock"))
	if err != nil {
		return nil, "", nil, err
	}
	path := filepath.Join(dir, "filings.json")
	state := filingState{}
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &state)
		if state == nil && err == nil {
			err = fmt.Errorf("empty filing state")
		}
	}
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		unlock()
		return nil, "", nil, err
	}
	return state, path, unlock, nil
}
func saveFilingState(path string, state filingState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "filings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(raw); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
func (f *CardFiler) budget() (int, error) {
	n := f.WeeklyBudget
	if n == 0 {
		n = 25
		if v := os.Getenv(EnvCardWeeklyBudget); v != "" {
			var err error
			n, err = strconv.Atoi(v)
			if err != nil {
				return 0, fmt.Errorf("%w: %s must be a positive integer", ErrInvalid, EnvCardWeeklyBudget)
			}
		}
	}
	if n < 1 {
		return 0, fmt.Errorf("%w: weekly card budget must be positive", ErrInvalid)
	}
	return n, nil
}
func checkKeepList(value string) error {
	switch value {
	case "", "spend", "credentials", "sign-ins/payments", "tags/releases/publishes", "force-pushes", "standing-approvals":
		return nil
	}
	return fmt.Errorf("%w: --keep-list must be spend, credentials, sign-ins/payments, tags/releases/publishes, force-pushes or standing-approvals", ErrInvalid)
}
func (f *CardFiler) submit(ctx context.Context, r CardRequest, supersedes string, update bool) (CardResult, error) {
	if err := checkKeepList(r.KeepList); err != nil {
		return CardResult{}, err
	}
	if err := f.CheckUI(r); err != nil {
		return CardResult{}, err
	}
	if _, _, err := r.Description(); err != nil {
		return CardResult{}, err
	}
	r.Attachments = append([]string(nil), r.Attachments...)
	for i, p := range r.Attachments {
		absolute, e := filepath.Abs(p)
		if e != nil {
			return CardResult{}, e
		}
		r.Attachments[i] = absolute
	}
	limit, err := f.budget()
	if err != nil {
		return CardResult{}, err
	}
	dir, e := f.stateDir()
	if e != nil {
		return CardResult{}, e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return CardResult{}, e
	}
	deliveryUnlock, e := lockFile(filepath.Join(dir, "delivery-"+r.Key+".lock"))
	if e != nil {
		return CardResult{}, e
	}
	defer deliveryUnlock()
	state, path, unlock, err := f.openState()
	if err != nil {
		return CardResult{}, fmt.Errorf("%w: filing state: %v", ErrHomeDown, err)
	}
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()
	identity, err := RequestIdentity(r)
	if err != nil {
		return CardResult{}, err
	}
	old, exists := state[r.Key]
	if exists && old.Held && (old.Identity != identity || old.Request.Thread != r.Thread || old.Request.Owner != r.Owner) {
		return CardResult{}, errReused
	}
	if exists && old.Held {
		return CardResult{Key: r.Key, Queued: true, Replay: true}, nil
	}
	if !exists {
		now := time.Now()
		if f.Now != nil {
			now = f.Now()
		}
		year, week := now.ISOWeek()
		wk := fmt.Sprintf("%04d-W%02d", year, week)
		project := strings.ToLower(fmt.Sprint(r.Ask["project"]))
		used := 0
		for _, entry := range state {
			if entry.Project == project && entry.Week == wk && !entry.Request.Urgent && entry.Request.KeepList == "" {
				used++
			}
		}
		if used >= limit && !r.Urgent && r.KeepList == "" {
			return CardResult{}, fmt.Errorf("%w: weekly card budget of %d reached; decide by your stated default, log the decision on your bead, show mk the result in the next build; use --urgent or --keep-list only for an exception", ErrInvalid, limit)
		}
		state[r.Key] = filingEntry{Request: r, Identity: identity, Project: project, Week: wk, Held: r.NonBlocking, Supersedes: supersedes, Update: update}
		if err = saveFilingState(path, state); err != nil {
			return CardResult{}, err
		}
		if r.NonBlocking {
			return CardResult{Key: r.Key, Queued: true}, nil
		}
	}

	unlock()
	locked = false
	var result CardResult
	if update {
		result, err = f.fileOrUpdateCard(ctx, r, supersedes)
	} else {
		result, err = f.fileCardReadback(ctx, r)
	}
	if !exists && errors.Is(err, errNoCardWritten) && errors.Is(err, ErrInvalid) {
		current, p, done, e := f.openState()
		if e != nil {
			return result, errors.Join(err, e)
		}
		delete(current, r.Key)
		e = saveFilingState(p, current)
		done()
		err = errors.Join(err, e)
	}
	return result, err
}

// Release sends accepted held cards in type order. Reserved slots are never checked again.
// A failed or uncertain delivery remains held; the same Request makes retries idempotent.
func (f *CardFiler) Release(ctx context.Context) (map[string][]CardResult, error) {
	state, _, unlock, err := f.openState()
	if err != nil {
		return nil, err
	}
	unlock()
	keys := []string{}
	for key, e := range state {
		if e.Held {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := state[keys[i]].Request.Type, state[keys[j]].Request.Type
		if a == b {
			return keys[i] < keys[j]
		}
		return a < b
	})
	groups := map[string][]CardResult{}
	var failures []error
	for _, key := range keys {
		result, typ, e := f.releaseOne(ctx, key)
		if e != nil {
			failures = append(failures, fmt.Errorf("request %s: %w", key, e))
			continue
		}
		if result.ID != "" {
			groups[typ] = append(groups[typ], result)
		}
	}
	return groups, errors.Join(failures...)
}
func (f *CardFiler) releaseOne(ctx context.Context, key string) (CardResult, string, error) {
	dir, err := f.stateDir()
	if err != nil {
		return CardResult{}, "", err
	}
	keyUnlock, err := lockFile(filepath.Join(dir, "delivery-"+key+".lock"))
	if err != nil {
		return CardResult{}, "", err
	}
	defer keyUnlock()
	state, path, unlock, err := f.openState()
	if err != nil {
		return CardResult{}, "", err
	}
	e, ok := state[key]
	if !ok || !e.Held {
		unlock()
		return CardResult{}, "", nil
	}
	unlock()
	r := e.Request
	if err = f.CheckUI(r); err != nil {
		return CardResult{}, "", err
	}
	if f.Check != nil {
		if _, _, err = f.PrepareAsk(ctx, r.Project, fmt.Sprint(r.Ask["project"]), fmt.Sprint(r.Ask["project_root"])); err != nil {
			return CardResult{}, "", err
		}
	}
	var result CardResult
	previousAttempt := e.Attempted
	state, path, unlock, err = f.openState()
	if err != nil {
		return CardResult{}, "", err
	}
	e.Attempted = true
	state[key] = e
	err = saveFilingState(path, state)
	unlock()
	if err != nil {
		return CardResult{}, "", err
	}
	if e.Update {
		result, err = f.fileOrUpdateCard(ctx, r, e.Supersedes)
	} else {
		result, err = f.fileCardReadback(ctx, r)
	}
	if err != nil {
		if !previousAttempt && errors.Is(err, errNoCardWritten) && errors.Is(err, ErrInvalid) {
			current, p, done, saveErr := f.openState()
			if saveErr != nil {
				return result, "", errors.Join(err, saveErr)
			}
			entry := current[key]
			entry.Attempted = false
			current[key] = entry
			saveErr = saveFilingState(p, current)
			done()
			err = errors.Join(err, saveErr)
		}
		return result, "", err
	}

	state, path, unlock, err = f.openState()
	if err != nil {
		return result, "", err
	}
	defer unlock()
	e.Held = false
	state[key] = e
	err = saveFilingState(path, state)
	typ := r.Type
	if typ == "" {
		typ = "other"
	}
	return result, typ, err
}

// CancelHeld cancels an unsent card and refunds its reserved slot.
func (f *CardFiler) CancelHeld(key string) error {
	if err := CheckRequestKey(key); err != nil {
		return err
	}
	dir, err := f.stateDir()
	if err != nil {
		return err
	}
	keyUnlock, err := lockFile(filepath.Join(dir, "delivery-"+key+".lock"))
	if err != nil {
		return err
	}
	defer keyUnlock()
	state, path, unlock, err := f.openState()
	if err != nil {
		return err
	}
	defer unlock()
	e, ok := state[key]
	if !ok || !e.Held || e.Attempted {
		return fmt.Errorf("%w: request is not held", ErrInvalid)
	}
	delete(state, key)
	return saveFilingState(path, state)
}
