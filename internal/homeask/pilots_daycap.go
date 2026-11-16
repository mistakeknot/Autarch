package homeask

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const CardDailyCap = 40
const EnvCardDailyCap = "AUTARCH_CARD_DAILY_CAP"

var pacific = func() *time.Location {
	l, e := time.LoadLocation("America/Los_Angeles")
	if e != nil {
		panic(e)
	}
	return l
}()

func pacificDay(t time.Time) string { return t.In(pacific).Format("2006-01-02") }
func ordinary(r CardRequest) bool {
	return !r.Urgent && r.KeepList == "" && r.Type != "merge" && r.RootRun == nil
}
func dailyCap() (int, error) {
	n := CardDailyCap
	if s := os.Getenv(EnvCardDailyCap); s != "" {
		v, e := strconv.Atoi(s)
		if e != nil || v < 1 {
			return 0, fmt.Errorf("%w: %s must be a positive integer", ErrInvalid, EnvCardDailyCap)
		}
		n = v
	}
	return n, nil
}
func (f *CardFiler) anyPilot() bool {
	return f.pilot("coldread") || f.pilot("daycap") || f.pilot("linkcheck")
}

// A global delivery lock covers admission through readback, including release.
func (f *CardFiler) deliveryGate() (func(), error) {
	if !f.pilot("daycap") {
		return func() {}, nil
	}
	d, e := f.stateDir()
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(d, 0700); e != nil {
		return nil, e
	}
	return lockFile(filepath.Join(d, "pilot-delivery.lock"))
}
func (f *CardFiler) dailyFull(s filingState, r CardRequest) (bool, error) {
	if !f.pilot("daycap") || !ordinary(r) {
		return false, nil
	}
	limit, e := dailyCap()
	if e != nil {
		return false, e
	}
	day := pacificDay(f.clock())
	n := 0
	seen := map[string]bool{}
	for key, v := range s {
		// Recovery reuses its already-admitted slot; it does not consume a second delivery.
		if key == r.Key && v.Held && (v.DeliveredAt != "" || v.DeliveryPendingAt != "") {
			continue
		}
		if v.Uncharged || !ordinary(v.Request) {
			continue
		}
		at := v.DeliveredAt
		if at == "" {
			at = v.DeliveryPendingAt
		}
		t, e := time.Parse(time.RFC3339Nano, at)
		if e != nil || pacificDay(t) != day {
			continue
		}
		id := v.DeliveredCard
		if id == "" {
			id = key
		}
		if !seen[id] {
			seen[id] = true
			n++
		}
	}
	return n >= limit, nil
}
func (f *CardFiler) noteDelivery(key string, result CardResult) error {
	if !f.anyPilot() || result.ID == "" {
		return nil
	}
	s, p, done, e := f.openState()
	if e != nil {
		return e
	}
	defer done()
	v, ok := s[key]
	if !ok {
		return nil
	}
	first := v.DeliveredAt == ""
	if v.DeliveredAt == "" {
		v.DeliveredAt = f.clock().Format(time.RFC3339Nano)
	}
	v.DeliveryPendingAt = ""
	v.DeliveredCard = result.ID
	if result.CardKey != "" {
		v.DeliveredCardKey = result.CardKey
	}
	v.DeliveredKey = result.Key
	v.DeliveredProject = result.Project
	s[key] = v
	err := saveFilingState(p, s)
	if err == nil && first {
		f.pilotEvent(v.Request, "delivered", "", "")
	}
	return err
}

// defaultUnderVetoWindowRule is deliberately a no-op pending a later rule.
func defaultUnderVetoWindowRule(CardRequest) {}
