package aggregator

import (
	"reflect"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/colony"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	"github.com/mistakeknot/autarch/internal/bigend/mcp"
	"github.com/mistakeknot/autarch/internal/icdata"
)

func strPtr(s string) *string { return &s }
func i64Ptr(i int64) *int64   { return &i }

// fullState builds a State in which every reachable slice, map and pointer is
// populated, so aliasing anywhere shows up.
func fullState() State {
	at := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	return State{
		Projects: []discovery.Project{{
			Path: "/p/a", Name: "a",
			TaskStats:       &discovery.TaskStats{Total: 3, Done: 1},
			PollardStats:    &discovery.PollardStats{Sources: 2, LastReport: "r"},
			GurgStats:       &discovery.GurgStats{Total: 4},
			InterspectStats: &discovery.InterspectStats{Sessions: 5},
		}},
		Agents:   []Agent{{Name: "ag", ProjectPath: "/p/a", LastActive: at}},
		Sessions: []TmuxSession{{Name: "s", Created: at, StateAt: at}},
		Colonies: []colony.Colony{{
			Name: "c", Root: "/p/a",
			Worktrees: []colony.Worktree{{Path: "/p/a/wt", Branch: "b"}},
			Members:   []colony.ColonyMember{{PID: 7, CWD: "/p/a", Source: "proc"}},
			Markers:   []string{".colony"},
		}},
		MCP: map[string][]mcp.ComponentStatus{
			"/p/a": {{ProjectPath: "/p/a", Component: "x", Pid: 9, StartedAt: at, LogTail: []string{"l1", "l2"}}},
		},
		Activities: []Activity{{Time: at, Type: "commit", ProjectPath: "/p/a", Summary: "s"}},
		Kernel: &KernelState{
			Runs: map[string][]icdata.Run{"/p/a": {{ID: "r1", Phases: []string{"plan", "build"}}}},
			Dispatches: map[string][]icdata.Dispatch{"/p/a": {{
				ID: "d1", Name: strPtr("n"), Model: strPtr("m"),
				StartedAt: i64Ptr(1), CompletedAt: i64Ptr(2), ScopeID: strPtr("sc"),
			}}},
			Events: map[string][]icdata.Event{"/p/a": {{ID: 1, RunID: "r1"}}},
			Metrics: KernelMetrics{
				ActiveRuns:   1,
				KernelErrors: map[string]string{"/p/a": "boom"},
			},
			CostBaseline: &icdata.CostBaseline{
				LandedChanges: 2,
				ByPhase:       map[string]icdata.CostTokenStats{"plan": {Total: 10}},
				ByAgent:       map[string]icdata.CostTokenStats{"ag": {Total: 20}},
			},
		},
		Items:     []Item{{ID: "a-1", Project: "/p/a", Status: "open", Age: time.Hour}},
		Flow:      []FlowCount{{Project: "/p/a", Stage: "open", Entered: 1}},
		UpdatedAt: at,
	}
}

var timeType = reflect.TypeOf(time.Time{})

// assertNoAlias fails for any pointer, non-empty slice or non-empty map that
// the two values share.
func assertNoAlias(t *testing.T, path string, a, b reflect.Value) {
	t.Helper()
	if a.Type() == timeType {
		return
	}
	switch a.Kind() {
	case reflect.Ptr:
		if a.IsNil() || b.IsNil() {
			return
		}
		if a.Pointer() == b.Pointer() {
			t.Errorf("%s: pointer shared between snapshot and clone", path)
			return
		}
		assertNoAlias(t, path+"*", a.Elem(), b.Elem())
	case reflect.Slice:
		if a.Len() == 0 || b.Len() == 0 {
			return
		}
		if a.Pointer() == b.Pointer() {
			t.Errorf("%s: slice backing array shared", path)
			return
		}
		for i := 0; i < a.Len() && i < b.Len(); i++ {
			assertNoAlias(t, path+"[]", a.Index(i), b.Index(i))
		}
	case reflect.Map:
		if a.Len() == 0 || b.Len() == 0 {
			return
		}
		if a.Pointer() == b.Pointer() {
			t.Errorf("%s: map shared", path)
			return
		}
		for _, k := range a.MapKeys() {
			if bv := b.MapIndex(k); bv.IsValid() {
				assertNoAlias(t, path+"{}", a.MapIndex(k), bv)
			}
		}
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			assertNoAlias(t, path+"."+a.Type().Field(i).Name, a.Field(i), b.Field(i))
		}
	}
}

func TestStateCloneSharesNothingReachable(t *testing.T) {
	orig := fullState()
	clone := orig.Clone()
	if !reflect.DeepEqual(orig, clone) {
		t.Fatal("clone differs from original")
	}
	assertNoAlias(t, "State", reflect.ValueOf(orig), reflect.ValueOf(clone))
}

func TestStateCloneMutationLeavesOriginalUnchanged(t *testing.T) {
	orig := fullState()
	c := orig.Clone()

	c.Projects[0].TaskStats.Total = 99
	c.Projects[0].PollardStats.LastReport = "x"
	c.Projects[0].GurgStats.Total = 99
	c.Projects[0].InterspectStats.Sessions = 99
	c.Colonies[0].Markers[0] = "x"
	c.Colonies[0].Worktrees[0].Branch = "x"
	c.Colonies[0].Members[0].PID = 99
	c.MCP["/p/a"][0].LogTail[0] = "x"
	c.Kernel.Metrics.KernelErrors["/p/a"] = "x"
	c.Kernel.Runs["/p/a"][0].Phases[0] = "x"
	*c.Kernel.Dispatches["/p/a"][0].Name = "x"
	*c.Kernel.Dispatches["/p/a"][0].StartedAt = 99
	c.Kernel.Events["/p/a"][0].RunID = "x"
	c.Kernel.CostBaseline.ByPhase["plan"] = icdata.CostTokenStats{Total: 99}
	c.Kernel.CostBaseline.ByAgent["ag"] = icdata.CostTokenStats{Total: 99}
	c.Items[0].Status = "x"
	c.Flow[0].Entered = 99

	if !reflect.DeepEqual(orig, fullState()) {
		t.Fatalf("mutating the clone changed the original:\n%+v", orig)
	}
}
