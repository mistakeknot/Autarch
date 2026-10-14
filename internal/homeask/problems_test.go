package homeask

import (
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// faults each break one independent field of a valid ask and name the field
// Problems must report for it.
var faults = []struct {
	field string
	apply func(*Ask)
}{
	{"v", func(a *Ask) { a.V = 2 }},
	{"project", func(a *Ask) { a.Project = "" }},
	{"project_root", func(a *Ask) { a.ProjectRoot = "relative/path" }},
	{"request_id", func(a *Ask) { a.RequestID = "not a valid id!" }},
	{"thread", func(a *Ask) { a.Thread = "bad thread!" }},
	{"subject", func(a *Ask) { a.Subject = strings.Repeat("s", 121) }},
	{"mention_of", func(a *Ask) { a.MentionOf = "bad mention!" }},
	{"options[0].label", func(a *Ask) { a.Options[0].Label = strings.Repeat("l", 81) }},
}

func baseAsk() Ask {
	return Ask{
		V: 1, Kind: "decide", Project: "Autarch", ProjectRoot: "/home/dev/projects/Autarch",
		Asker: "thread", Thread: "thr_abc123", Question: "Collapse per project or per day?",
		Options: []Option{{Label: "Per project"}, {Label: "Per day"}},
	}
}

func problemFields(ps []Problem) map[string]bool {
	m := map[string]bool{}
	for _, p := range ps {
		m[p.Field] = true
	}
	return m
}

// k independent faults give a problem for each of the k fields, never fewer, and
// the ask handed in is not changed.
func TestProblemsReportsEveryFault(t *testing.T) {
	prop := func(mask uint8) bool {
		a := baseAsk()
		want := []string{}
		for i, f := range faults {
			if mask&(1<<i) != 0 {
				f.apply(&a)
				want = append(want, f.field)
			}
		}
		before := a
		before.Options = append([]Option(nil), a.Options...)
		ps := Problems(a)
		if !reflect.DeepEqual(before, a) {
			t.Logf("mask %08b: Problems changed its input", mask)
			return false
		}
		got := problemFields(ps)
		for _, f := range want {
			if !got[f] {
				t.Logf("mask %08b: no problem for %s (got %v)", mask, f, ps)
				return false
			}
		}
		if len(want) == 0 {
			return len(ps) == 0
		}
		_, err := Normalize(a)
		ve, ok := err.(*ValidationError)
		return ok && len(ve.Problems) == len(ps) && len(ps) >= len(want)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 512}); err != nil {
		t.Fatal(err)
	}
}

// The first problem is the message Normalize has always led with.
func TestNormalizeErrorLeadsWithFirstProblem(t *testing.T) {
	a := baseAsk()
	a.V = 2
	a.Project = ""
	_, err := Normalize(a)
	if err == nil || !strings.HasPrefix(err.Error(), "v must be 1") || !strings.Contains(err.Error(), "also: project: project is required") {
		t.Fatalf("err = %v", err)
	}
}
