package homeask

import (
	"fmt"
	"time"
)

type budgetWeek struct{ start, end time.Time }

func pacificBudgetWeek(at time.Time) budgetWeek {
	local := at.In(pacific)
	days := (int(local.Weekday()) - int(time.Thursday) + 7) % 7
	start := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, pacific).AddDate(0, 0, -days)
	if local.Before(start) {
		start = start.AddDate(0, 0, -7)
	}
	// Calendar arithmetic preserves Pacific noon across both DST transitions.
	return budgetWeek{start: start, end: start.AddDate(0, 0, 7)}
}

func (w budgetWeek) key() string { return w.start.Format("2006-01-02") }

// weeklyHold lives in weekday_hold.go so its signature requires a parsed setting.

// reservationWeeks reads both Pacific start-date keys and legacy ISO keys.
// Timestamped legacy rows map exactly. Without a timestamp, report
// the record in both overlapping Pacific periods; it ages out after the
// last overlapping period. Reading or releasing never rewrites the old key.
func reservationWeeks(e filingEntry) []budgetWeek {
	if day, err := time.ParseInLocation("2006-01-02", e.Week, pacific); err == nil && day.Weekday() == time.Thursday {
		return []budgetWeek{pacificBudgetWeek(day.Add(12 * time.Hour))}
	}
	var year, week int
	if _, err := fmt.Sscanf(e.Week, "%04d-W%02d", &year, &week); err != nil || year < 1 || year > 9999 || week < 1 || week > 53 || fmt.Sprintf("%04d-W%02d", year, week) != e.Week {
		return nil
	}
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, pacific)
	monday := jan4.AddDate(0, 0, -(int(jan4.Weekday())+6)%7+7*(week-1))
	if y, w := monday.ISOWeek(); y != year || w != week {
		return nil
	}
	if at, err := time.Parse(time.RFC3339Nano, e.FiledAt); err == nil {
		return []budgetWeek{pacificBudgetWeek(at)}
	}
	return []budgetWeek{pacificBudgetWeek(monday), pacificBudgetWeek(monday.AddDate(0, 0, 6))}
}

func reservedInWeek(e filingEntry, week budgetWeek) bool {
	for _, candidate := range reservationWeeks(e) {
		if candidate.start.Equal(week.start) {
			return true
		}
	}
	return false
}
