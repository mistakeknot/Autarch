package homeask

import (
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
func (f *CardFiler) anyPilot() bool {
	return f.pilot("coldread") || f.pilot("linkcheck")
}

// The retired daycap pilot remains accepted as a compatibility no-op.
func (f *CardFiler) deliveryGate() (func(), error)                    { return func() {}, nil }
func (f *CardFiler) dailyFull(filingState, CardRequest) (bool, error) { return false, nil }
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
