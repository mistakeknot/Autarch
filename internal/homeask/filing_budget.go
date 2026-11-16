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
		if e.Project == strings.ToLower(project) && e.Week == week && !e.Uncharged && !e.Refunded {
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
	DeliveredCardKey  string      `json:"delivered_card_key,omitempty"`
	FiledAt           string      `json:"filed_at,omitempty"`
	DeliveredAt       string      `json:"delivered_at,omitempty"`
	DeliveryPendingAt string      `json:"delivery_pending_at,omitempty"`
	LinkFailureSet    string      `json:"link_failure_set,omitempty"`
	HeldReason        string      `json:"held_reason,omitempty"`
	DeliveredCard     string      `json:"delivered_card,omitempty"`
	DeliveredKey      string      `json:"delivered_key,omitempty"`
	DeliveredProject  string      `json:"delivered_project,omitempty"`
	CreationKey       string      `json:"creation_key,omitempty"`
	Uncharged         bool        `json:"uncharged,omitempty"`
	Refunded          bool        `json:"refunded,omitempty"`
	TerminalReason    string      `json:"terminal_reason,omitempty"`
	Request           CardRequest `json:"request"`
	Identity          string      `json:"identity"`
	Project           string      `json:"project"`
	Week              string      `json:"week"`
	Attempted         bool        `json:"attempted,omitempty"`
	Held              bool        `json:"held"`
	Supersedes        string      `json:"supersedes,omitempty"`
	Update            bool        `json:"update"`
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

// defaultWeeklyBudget is the ordinary cards per Home project per ISO week when
// neither CardFiler.WeeklyBudget nor AUTARCH_CARD_WEEKLY_BUDGET is set (mk-bzi8s).
const defaultWeeklyBudget = 50

func (f *CardFiler) budget() (int, error) {
	n := f.WeeklyBudget
	if n == 0 {
		n = defaultWeeklyBudget
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
	if _, _, err := r.Description(); err != nil {
		return CardResult{}, err
	}
	if err := CheckStopped(r.Ask); err != nil {
		return CardResult{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !r.Pull && r.Thread == "" {
		return CardResult{}, fmt.Errorf("%w: BB_THREAD_ID is required", ErrInvalid)
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
	identity, err := RequestIdentity(r)
	if err != nil {
		return CardResult{}, err
	}
	if f.anyPilot() {
		// Per-request serialization makes a held retry independent of volatile proofs.
		prior, _, priorDone, priorErr := f.openState()
		if priorErr == nil {
			oldHeld, hadHeld := prior[r.Key]
			priorDone()
			if hadHeld && oldHeld.Held && !oldHeld.Attempted {
				if oldHeld.Identity != identity || oldHeld.Request.Thread != r.Thread || oldHeld.Request.Owner != r.Owner {
					return CardResult{}, errReused
				}
				return CardResult{Key: r.Key, Queued: true, Replay: true}, nil
			}
		}
		// Other state errors/refusals remain after canonical registry replay, as before.
	}
	askKey, _ := r.Ask["ask_key"].(string)
	updateLane := update && (askKey != "" || supersedes != "")
	uncharged := updateLane
	if !updateLane {
		lockPath, e := f.lockPath(r.Key)
		if e != nil {
			return CardResult{}, e
		}
		lookupUnlock, e := lockFile(lockPath)
		if e != nil {
			return CardResult{}, e
		}
		candidate, lookupErr := f.locate(ctx, r.Key, identity)
		if lookupErr != nil {
			lookupUnlock()
			return CardResult{}, lookupErr
		}
		if candidate != nil {
			result, e := f.replay(ctx, r, candidate)
			lookupUnlock()
			if e == nil && !r.Pull && f.ReadbackTimeout > 0 {
				return f.withCardKey(ctx, result)
			}
			return result, e
		}
		lookupUnlock()
	}
	if f.pilot("linkcheck") {
		if v := f.checkLinks(ctx, r); len(v) > 0 {
			return CardResult{}, fmt.Errorf("%w: link-check: %s", ErrInvalid, failureText(v))
		}
	}
	if err := f.checkColdRead(ctx, r); err != nil {
		return CardResult{}, err
	}
	if updateLane && r.NonBlocking {
		uncharged, err = f.hasUpdateTarget(ctx, r, supersedes)
		if err != nil {
			return CardResult{}, err
		}
	}
	if !updateLane || r.NonBlocking {
		if err := f.CheckUI(r); err != nil {
			return CardResult{}, err
		}
	}
	// Slow validation above never holds the fleet gate. Only admission and delivery do.
	gateDone, e := f.deliveryGate()
	if e != nil {
		return CardResult{}, e
	}
	defer gateDone()
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
	identity, err = RequestIdentity(r)
	if err != nil {
		return CardResult{}, err
	}
	old, exists := state[r.Key]
	if exists && old.Refunded {
		return CardResult{}, fmt.Errorf("%w: request retired: %s; file the corrected ask with a new request key", ErrInvalid, old.TerminalReason)
	}
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
			if entry.Project == project && entry.Week == wk && !entry.Uncharged && !entry.Refunded && !entry.Request.Urgent && entry.Request.KeepList == "" {
				used++
			}
		}
		if !uncharged && used >= limit && !r.Urgent && r.KeepList == "" {
			return CardResult{}, fmt.Errorf("%w: weekly card budget of %d reached; decide by your stated default, log the decision on your bead, show mk the result in the next build; use --urgent or --keep-list only for an exception", ErrInvalid, limit)
		}
		full, e := f.dailyFull(state, r)
		if e != nil {
			return CardResult{}, e
		}
		held := r.NonBlocking || full
		reason := ""
		if full {
			reason = "daily-cap"
		} else if held && f.anyPilot() {
			reason = "non-blocking"
		}
		state[r.Key] = filingEntry{Request: r, Identity: identity, Project: project, Week: wk, Held: held, HeldReason: reason, Supersedes: supersedes, Update: update, Uncharged: uncharged}
		if f.anyPilot() {
			v := state[r.Key]
			v.FiledAt = f.clock().Format(time.RFC3339Nano)
			state[r.Key] = v
		}
		if err = saveFilingState(path, state); err != nil {
			return CardResult{}, err
		}
		f.pilotEvent(r, "filed", "", "")
		if held {
			f.pilotEvent(r, "held", "", reason)
			if full && f.Warn != nil {
				f.Warn("held: daily-cap reached; queued for ranked release")
			}
			return CardResult{Key: r.Key, Queued: true}, nil
		}
	}

	unlock()
	locked = false
	var result CardResult
	r.beforeCreate = func(key string) error { return f.chargeCreation(r, key) }
	r.afterUpdate = func(result CardResult) error { return f.recordWrittenEdit(r.Key, result) }
	if update {
		result, err = f.fileOrUpdateCard(ctx, r, supersedes)
	} else {
		result, err = f.fileCardReadback(ctx, r)
	}
	if !exists && errors.Is(err, errNoCardWritten) && (errors.Is(err, ErrInvalid) || errors.Is(err, ErrAlreadyRuled)) {
		current, p, done, e := f.openState()
		if e != nil {
			return result, errors.Join(err, e)
		}
		delete(current, r.Key)
		e = saveFilingState(p, current)
		done()
		err = errors.Join(err, e)
	}
	err = errors.Join(err, f.noteDelivery(r.Key, result))
	return result, err
}

// ReleaseReport includes delivered groups and a diagnostic for every failed request.
type ReleaseReport struct {
	OwnerAttention map[string]string       `json:"owner_attention,omitempty"`
	Groups         map[string][]CardResult `json:"groups"`
	Failures       map[string]string       `json:"failures"`
}

// Release sends held cards in type order. Terminal refusals are recorded and
// refunded after an absence proof; uncertain deliveries remain held for retry.
func (f *CardFiler) Release(ctx context.Context) (map[string][]CardResult, error) {
	report, err := f.ReleaseDetailed(ctx)
	return report.Groups, err
}

func (f *CardFiler) ReleaseDetailed(ctx context.Context) (ReleaseReport, error) {
	report := ReleaseReport{Groups: map[string][]CardResult{}, Failures: map[string]string{}}
	// One configuration read per release; expiry is still evaluated at each admission.
	scoped := *f
	pilots, pilotErr := f.Pilots()
	scoped.pilotSnapshot = pilots
	if pilotErr != nil {
		scoped.pilotSnapshot = Pilots{}
		if f.Warn != nil {
			f.Warn("pilots disabled: " + pilotErr.Error())
		}
	}
	f = &scoped
	state, _, unlock, err := f.openState()
	if err != nil {
		return report, err
	}
	unlock()
	keys := []string{}
	for key, e := range state {
		if e.Held {
			keys = append(keys, key)
		}
	}
	rankByBlocks := f.pilot("daycap")
	sort.Slice(keys, func(i, j int) bool {
		if rankByBlocks {
			a, b := state[keys[i]], state[keys[j]]
			if len(a.Request.Blocks) != len(b.Request.Blocks) {
				return len(a.Request.Blocks) > len(b.Request.Blocks)
			}
			if a.FiledAt != b.FiledAt {
				return a.FiledAt < b.FiledAt
			}
			return keys[i] < keys[j]
		}
		a, b := state[keys[i]].Request.Type, state[keys[j]].Request.Type
		if a == b {
			return keys[i] < keys[j]
		}
		return a < b
	})
	groups := report.Groups
	var failures []error
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		result, typ, e := f.releaseOne(ctx, key)
		if result.ID != "" {
			if e == nil {
				f.pilotEvent(state[key].Request, "released", "", "")
			}
			if typ == "" {
				typ = state[key].Request.Type
			}
			if typ == "" {
				typ = "other"
			}
			groups[typ] = append(groups[typ], result)
		}
		if e != nil {
			report.Failures[key] = e.Error()
			var ownerErr *releaseOwnerError
			if errors.As(e, &ownerErr) {
				if report.OwnerAttention == nil {
					report.OwnerAttention = map[string]string{}
				}
				report.OwnerAttention[key] = ownerErr.Error()
			}
			failures = append(failures, fmt.Errorf("request %s: %w", key, e))
			continue
		}
	}
	if len(failures) > 0 {
		return report, &ReleaseEntryErrors{Errors: failures}
	}
	return report, nil
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
	// Advisory early-out avoids checks and wakes for cards that cannot be displayed.
	// Admission is still checked authoritatively under the fleet gate below.
	fullBeforeChecks, capErr := f.dailyFull(state, e.Request)
	if capErr != nil {
		unlock()
		return CardResult{}, "", capErr
	}
	if fullBeforeChecks {
		unlock()
		return CardResult{}, "", nil
	}
	unlock()
	r := e.Request
	if r.Owner != "" {
		if ownerErr := f.checkOwnerLive(ctx, r.Owner); ownerErr != nil {
			if errors.Is(ownerErr, ErrInvalid) {
				attention := &releaseOwnerError{owner: r.Owner, cause: ownerErr}
				if e.Attempted {
					recovered, recoveryErr := f.resolveTerminal(ctx, key, attention)
					return recovered, r.Type, errors.Join(attention, recoveryErr)
				}
				return CardResult{}, r.Type, attention
			}
			return CardResult{}, r.Type, ownerErr
		}
	}
	if f.pilot("linkcheck") {
		if v := f.checkLinks(ctx, r); len(v) > 0 {
			wakeErr := f.wakeLinkOwner(ctx, key, r, v)
			return CardResult{}, r.Type, errors.Join(fmt.Errorf("link-check: %s", failureText(v)), wakeErr)
		}
	}
	if f.Check != nil {
		if _, _, err = f.PrepareAsk(ctx, r.Project, fmt.Sprint(r.Ask["project"]), fmt.Sprint(r.Ask["project_root"])); err != nil {
			if errors.Is(err, ErrInvalid) {
				delivered, resolveErr := f.resolveTerminal(ctx, key, err)
				if delivered.ID != "" {
					return delivered, r.Type, resolveErr
				}
				err = errors.Join(errNoCardWritten, err, resolveErr)
			}
			return CardResult{}, "", err
		}
	}
	gateDone, err := f.deliveryGate()
	if err != nil {
		return CardResult{}, "", err
	}
	defer gateDone()
	var result CardResult
	state, path, unlock, err = f.openState()
	if err != nil {
		return CardResult{}, "", err
	}
	e, ok = state[key]
	if !ok || !e.Held {
		unlock()
		return CardResult{}, "", nil
	}
	full, capErr := f.dailyFull(state, e.Request)
	if capErr != nil {
		unlock()
		return CardResult{}, "", capErr
	}
	if full {
		unlock()
		return CardResult{}, "", nil
	}
	e.Attempted = true
	state[key] = e
	err = saveFilingState(path, state)
	unlock()
	if err != nil {
		return CardResult{}, "", err
	}
	r.beforeCreate = func(key string) error { return f.chargeCreation(r, key) }
	r.afterUpdate = func(result CardResult) error { return f.recordWrittenEdit(r.Key, result) }
	if e.Update {
		result, err = f.fileOrUpdateCard(ctx, r, e.Supersedes)
	} else {
		result, err = f.fileCardReadback(ctx, r)
	}
	if err != nil {
		var gone *registeredCardGoneError
		if (errors.Is(err, errNoCardWritten) && (errors.Is(err, ErrInvalid) || errors.Is(err, ErrAlreadyRuled))) || (e.Attempted && errors.As(err, &gone)) {
			delivered, resolveErr := f.resolveTerminal(ctx, key, err)
			if delivered.ID != "" {
				return delivered, r.Type, resolveErr
			}
			err = errors.Join(err, resolveErr)
		}
		return result, "", err
	}

	state, path, unlock, err = f.openState()
	if err != nil {
		return result, "", err
	}
	defer unlock()
	e = state[key] // Preserve creation admission saved by the delivery callback.
	e.Held = false
	e.DeliveredCard = result.ID
	if f.anyPilot() && e.DeliveredAt == "" {
		e.DeliveredAt = f.clock().Format(time.RFC3339Nano)
	}
	e.DeliveryPendingAt = ""
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
	if ok && !e.Held && e.DeliveredCard != "" {
		return fmt.Errorf("%w: %w as card %s; nothing to cancel", ErrInvalid, ErrAlreadyDelivered, e.DeliveredCard)
	}
	if ok && e.Refunded && e.TerminalReason != "" {
		return nil
	}
	if !ok || !e.Held || e.Attempted {
		return fmt.Errorf("%w: request is not held", ErrInvalid)
	}
	delete(state, key)
	return saveFilingState(path, state)
}

// resolveTerminal completes a known delivery, or refunds a proven absent one.
// Finding the card supersedes a refusal based on stale project or ruling state.
func (f *CardFiler) resolveTerminal(ctx context.Context, key string, reason error) (CardResult, error) {
	current, _, done, readErr := f.openState()
	if readErr != nil {
		return CardResult{}, readErr
	}
	e := current[key]
	done()
	removed := false
	lookup := func(requestKey string) (*found, error) {
		candidate, err := f.locate(ctx, requestKey, e.Identity)
		var gone *registeredCardGoneError
		if errors.As(err, &gone) {
			removed = true
			return &found{ID: gone.CardID, ProjectID: e.Request.Project, Req: RequestLine{Key: gone.Key, Identity: e.Identity}}, nil
		}
		return candidate, err
	}
	var delivered *found
	if e.DeliveredCard != "" {
		deliveredKey := e.DeliveredKey
		if deliveredKey == "" {
			deliveredKey = key
		}
		project := e.DeliveredProject
		if project == "" {
			project = e.Request.Project
		}
		delivered = &found{ID: e.DeliveredCard, ProjectID: project, Req: RequestLine{Key: deliveredKey, Identity: e.Identity}}
	}
	if delivered == nil && e.CreationKey != "" && e.CreationKey != key {
		candidate, err := lookup(e.CreationKey)
		if err != nil {
			return CardResult{}, err
		}
		delivered = candidate
	}
	if delivered == nil {
		candidate, err := lookup(key)
		if err != nil {
			return CardResult{}, err
		}
		delivered = candidate
	}
	state, path, unlock, err := f.openState()
	if err != nil {
		return CardResult{}, err
	}
	defer unlock()
	entry := state[key]
	entry.Held = false
	result := CardResult{}
	if delivered != nil {
		entry.DeliveredCard = delivered.ID
		if removed {
			entry.TerminalReason = fmt.Sprintf("delivered-then-removed: card %s", delivered.ID)
		}
		result = CardResult{ID: delivered.ID, Key: delivered.Req.Key, Project: delivered.ProjectID, Replay: true}
	} else {
		entry.Refunded = true
		entry.TerminalReason = reason.Error()
	}
	state[key] = entry
	return result, saveFilingState(path, state)
}

// An update target can disappear after the read-only admission probe. Recheck the
// budget at the actual creation boundary before allowing that fallback to create.
func (f *CardFiler) chargeCreation(r CardRequest, creationKey string) error {
	state, path, unlock, err := f.openState()
	if err != nil {
		return err
	}
	defer unlock()
	entry, ok := state[r.Key]
	if !ok {
		return fmt.Errorf("%w: missing creation reservation", ErrInvalid)
	}
	entry.CreationKey = creationKey
	if f.pilot("daycap") && entry.DeliveredAt == "" {
		entry.DeliveryPendingAt = f.clock().Format(time.RFC3339Nano)
	}
	if !entry.Uncharged {
		state[r.Key] = entry
		return saveFilingState(path, state)
	}
	limit, err := f.budget()
	if err != nil {
		return err
	}
	used := 0
	for _, e := range state {
		if e.Project == entry.Project && e.Week == entry.Week && !e.Uncharged && !e.Refunded && !e.Request.Urgent && e.Request.KeepList == "" {
			used++
		}
	}
	if used >= limit && !r.Urgent && r.KeepList == "" {
		return fmt.Errorf("%w: weekly card budget reached before creating replacement; decide by your stated default, log the decision on your bead, show mk the result in the next build", ErrInvalid)
	}
	entry.Uncharged = false
	state[r.Key] = entry
	return saveFilingState(path, state)
}

func (f *CardFiler) hasUpdateTarget(ctx context.Context, r CardRequest, supersedes string) (bool, error) {
	if supersedes != "" {
		_, err := f.show(ctx, supersedes)
		if errors.Is(err, errTaskGone) {
			return false, nil
		}
		return err == nil, err
	}
	key, _ := r.Ask["ask_key"].(string)
	if key == "" {
		return false, nil
	}
	encoded, _ := json.Marshal(key)
	searchText := string(encoded[1 : len(encoded)-1])
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		args := []string{"tasks", "list", "--status", searchStatuses, "--project", r.Project, "--search", searchText, "--limit", pageLimit, "--json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		var rows struct {
			Tasks      []taskRow
			NextCursor *string
		}
		if err := f.call(ctx, "", &rows, args...); err != nil {
			return false, err
		}
		for _, row := range rows.Tasks {
			card, err := f.show(ctx, row.ID)
			if errors.Is(err, errTaskGone) {
				continue
			}
			if err != nil {
				return false, err
			}
			parsed, err := ParseCard(card.Task.Description)
			if err == nil && parsed.Ask["ask_key"] == key && card.Task.Status != "done" && card.Task.Status != "canceled" {
				return true, nil
			}
		}
		if rows.NextCursor == nil || *rows.NextCursor == "" {
			return false, nil
		}
		cursor = *rows.NextCursor
		if seen[cursor] {
			return false, fmt.Errorf("%w: tasks cursor did not advance", ErrHomeDown)
		}
		seen[cursor] = true
	}
	return false, fmt.Errorf("%w: tasks search did not finish", ErrHomeDown)
}

// recordWrittenEdit stores the successful mutation before any later read can fail.
// In-place edits retain their original uncharged status.
func (f *CardFiler) recordWrittenEdit(key string, result CardResult) error {
	state, path, unlock, err := f.openState()
	if err != nil {
		return err
	}
	defer unlock()
	entry, ok := state[key]
	if !ok {
		return fmt.Errorf("%w: missing edit delivery entry", ErrHomeDown)
	}
	entry.DeliveredCard = result.ID
	entry.DeliveredKey = result.Key
	entry.DeliveredProject = result.Project
	state[key] = entry
	return saveFilingState(path, state)
}
