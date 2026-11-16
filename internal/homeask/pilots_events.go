package homeask

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PilotMetric struct {
	Time           time.Time `json:"time"`
	Event          string    `json:"event"`
	RequestKey     string    `json:"request_key"`
	CardKey        string    `json:"card_key,omitempty"`
	Project        string    `json:"project"`
	CardType       string    `json:"card_type"`
	ExemptFlags    []string  `json:"exempt_flags"`
	Result         string    `json:"result,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	OptionKind     string    `json:"option_kind,omitempty"`
	Other          bool      `json:"other,omitempty"`
	FreeTextLength int       `json:"free_text_length,omitempty"`
	PastedError    bool      `json:"pasted_error,omitempty"`
}

func exemptions(r CardRequest) []string {
	v := []string{}
	if r.Urgent {
		v = append(v, "urgent")
	}
	if r.KeepList != "" {
		v = append(v, "keep-list")
	}
	if r.Type == "merge" {
		v = append(v, "merge")
	}
	if r.RootRun != nil {
		v = append(v, "root-run")
	}
	return v
}
func (f *CardFiler) pilotEvent(r CardRequest, event, result, reason string) {
	if !f.anyPilot() {
		return
	}
	d, e := f.stateDir()
	if e != nil {
		return
	}
	typ := r.Type
	if typ == "" {
		typ = "other"
	}
	m := PilotMetric{Time: f.clock(), Event: event, RequestKey: r.Key, Project: strings.ToLower(fmt.Sprint(r.Ask["project"])), CardType: typ, ExemptFlags: exemptions(r), Result: result, Reason: reason}
	s := filingState{}
	if b, e := os.ReadFile(filepath.Join(d, "filings.json")); e == nil && json.Unmarshal(b, &s) == nil {
		m.CardKey = s[r.Key].DeliveredCardKey
	}
	b, e := json.Marshal(m)
	if e == nil {
		var file *os.File
		file, e = os.OpenFile(filepath.Join(d, "pilots-metrics.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if e == nil {
			_, e = file.Write(append(b, '\n'))
			ce := file.Close()
			if e == nil {
				e = ce
			}
		}
	}
	if e != nil && f.Warn != nil {
		f.Warn("pilot metric unavailable: " + e.Error())
	}
}

type dayMetrics struct{ Delivered, Held, Released, Questions, Errors, ColdPass, ColdFail, ColdSkip, LinkPass, LinkFail int }

func (f *CardFiler) PilotReport(w io.Writer, since string) error {
	var from time.Time
	if since != "" {
		var e error
		from, e = time.ParseInLocation("2006-01-02", since, pacific)
		if e != nil {
			return fmt.Errorf("%w: --since requires YYYY-MM-DD (Pacific day)", ErrInvalid)
		}
	}
	d, e := f.stateDir()
	if e != nil {
		return e
	}
	file, e := os.Open(filepath.Join(d, "pilots-metrics.jsonl"))
	if os.IsNotExist(e) {
		fmt.Fprintln(w, "No pilot metrics.")
		fmt.Fprintln(w, "Time to playable build is not visible to the filer.")
		return nil
	}
	if e != nil {
		return e
	}
	defer file.Close()
	days := map[string]*dayMetrics{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var m PilotMetric
		if e = json.Unmarshal(scanner.Bytes(), &m); e != nil {
			return fmt.Errorf("metric line %d: %w", line, e)
		}
		if m.Time.Before(from) {
			continue
		}
		day := pacificDay(m.Time)
		v := days[day]
		if v == nil {
			v = &dayMetrics{}
			days[day] = v
		}
		switch m.Event {
		case "delivered":
			v.Delivered++
		case "held":
			v.Held++
		case "released":
			v.Released++
		case "picked":
			if m.OptionKind == "needs-context" {
				v.Questions++
			}
			if m.PastedError {
				v.Errors++
			}
		case "cold-read":
			switch m.Result {
			case "pass":
				v.ColdPass++
			case "fail":
				v.ColdFail++
			case "skip":
				v.ColdSkip++
			}
		case "link-check":
			if m.Result == "pass" {
				v.LinkPass++
			} else if m.Result == "fail" {
				v.LinkFail++
			}
		}
	}
	if e = scanner.Err(); e != nil {
		return e
	}
	keys := make([]string, 0, len(days))
	for k := range days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintln(w, "Pacific day (America/Los_Angeles) delivered held released questions-back pasted-errors cold-pass cold-fail cold-skip link-pass link-fail")
	for _, k := range keys {
		v := days[k]
		fmt.Fprintf(w, "%s %d %d %d %d %d %d %d %d %d %d\n", k, v.Delivered, v.Held, v.Released, v.Questions, v.Errors, v.ColdPass, v.ColdFail, v.ColdSkip, v.LinkPass, v.LinkFail)
	}
	fmt.Fprintln(w, "Pasted errors use the pasted-error-v1 heuristic; free text is never logged.")
	fmt.Fprintln(w, "Time to playable build is not visible to the filer.")
	return nil
}
