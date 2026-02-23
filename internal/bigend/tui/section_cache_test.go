package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/icdata"
)

func TestSectionHashStability(t *testing.T) {
	// Same input must produce the same hash.
	state := aggregator.State{
		Sessions: []aggregator.TmuxSession{
			{Name: "a", UnifiedState: icdata.StatusActive},
			{Name: "b", UnifiedState: icdata.StatusWaiting},
		},
		Agents: []aggregator.Agent{
			{Name: "agent-1", Program: "claude"},
		},
	}
	h1 := hashSessions(state.Sessions, 5)
	h2 := hashSessions(state.Sessions, 5)
	if h1 != h2 {
		t.Errorf("same input produced different hashes: %d vs %d", h1, h2)
	}

	h3 := hashAgents(state.Agents, 5)
	h4 := hashAgents(state.Agents, 5)
	if h3 != h4 {
		t.Errorf("same agent input produced different hashes: %d vs %d", h3, h4)
	}
}

func TestSectionHashSensitivity(t *testing.T) {
	// Different input must produce different hashes.
	s1 := []aggregator.TmuxSession{{Name: "a", UnifiedState: icdata.StatusActive}}
	s2 := []aggregator.TmuxSession{{Name: "b", UnifiedState: icdata.StatusActive}}
	s3 := []aggregator.TmuxSession{{Name: "a", UnifiedState: icdata.StatusWaiting}}

	h1 := hashSessions(s1, 5)
	h2 := hashSessions(s2, 5)
	h3 := hashSessions(s3, 5)

	if h1 == h2 {
		t.Error("different session names produced same hash")
	}
	if h1 == h3 {
		t.Error("different session statuses produced same hash")
	}
}

func TestSectionHashWidthSensitivity(t *testing.T) {
	// Width changes must produce different stats hashes.
	state := aggregator.State{
		Sessions: []aggregator.TmuxSession{{Name: "a"}},
	}
	h1 := hashStats(state, 80)
	h2 := hashStats(state, 120)
	if h1 == h2 {
		t.Error("different widths produced same stats hash")
	}
}

func TestSectionCacheHitAndMiss(t *testing.T) {
	cache := newSectionCache()
	calls := 0
	renderFn := func() string {
		calls++
		return "rendered"
	}

	// First call: miss
	result := cache.getOrRender(sectionStats, 42, renderFn)
	if result != "rendered" || calls != 1 {
		t.Errorf("expected miss: result=%q calls=%d", result, calls)
	}

	// Second call same hash: hit
	result = cache.getOrRender(sectionStats, 42, renderFn)
	if result != "rendered" || calls != 1 {
		t.Errorf("expected hit: result=%q calls=%d", result, calls)
	}

	// Third call different hash: miss
	result = cache.getOrRender(sectionStats, 99, renderFn)
	if result != "rendered" || calls != 2 {
		t.Errorf("expected miss on new hash: result=%q calls=%d", result, calls)
	}
}

func TestSectionCacheInvalidate(t *testing.T) {
	cache := newSectionCache()
	calls := 0
	renderFn := func() string {
		calls++
		return "v" + string(rune('0'+calls))
	}

	cache.getOrRender(sectionStats, 42, renderFn)
	if calls != 1 {
		t.Fatal("expected 1 render call")
	}

	cache.invalidateAll()

	result := cache.getOrRender(sectionStats, 42, renderFn)
	if calls != 2 {
		t.Errorf("expected re-render after invalidate, calls=%d", calls)
	}
	if result != "v2" {
		t.Errorf("expected fresh render, got %q", result)
	}
}

func TestHashActivities(t *testing.T) {
	now := time.Now()
	a1 := []aggregator.Activity{{Time: now, Summary: "deployed", Source: "kernel"}}
	a2 := []aggregator.Activity{{Time: now, Summary: "deployed", Source: "tmux"}}
	h1 := hashActivities(a1, 10)
	h2 := hashActivities(a2, 10)
	if h1 == h2 {
		t.Error("different activity sources produced same hash")
	}
}

func TestHashRunsDeterministic(t *testing.T) {
	// Verify map key sorting produces stable hashes.
	kernel := &aggregator.KernelState{
		Runs: map[string][]icdata.Run{
			"/proj/alpha": {{ID: "r1", Status: "active", Phase: "plan"}},
			"/proj/beta":  {{ID: "r2", Status: "active", Phase: "exec"}},
			"/proj/gamma": {{ID: "r3", Status: "done", Phase: "ship"}},
		},
	}
	h1 := hashRuns(kernel, 120)
	// Call multiple times — if map ordering affects hash, these will differ.
	for i := 0; i < 100; i++ {
		h := hashRuns(kernel, 120)
		if h != h1 {
			t.Fatalf("hashRuns non-deterministic on iteration %d: %d vs %d", i, h1, h)
		}
	}
}

func TestResizeInvalidatesCache(t *testing.T) {
	agg := &fakeAggStatus{state: aggregator.State{
		Sessions: []aggregator.TmuxSession{
			{Name: "s1", ProjectPath: "/proj"},
		},
	}}
	m := New(agg, "test")
	m.width = 120
	m.height = 40

	// Prime the cache
	m.renderDashboard()

	if len(m.dashCache.entries) == 0 {
		t.Fatal("cache should have entries after render")
	}

	// Simulate resize
	m = m.applyResize(tea.WindowSizeMsg{Width: 80, Height: 30})

	if len(m.dashCache.entries) != 0 {
		t.Errorf("cache should be empty after resize, has %d entries", len(m.dashCache.entries))
	}
}

func TestDashboardCacheSkipsReRender(t *testing.T) {
	agg := &fakeAggStatus{state: aggregator.State{
		Sessions: []aggregator.TmuxSession{
			{Name: "s1", UnifiedState: icdata.StatusActive, ProjectPath: "/proj"},
		},
		Agents: []aggregator.Agent{
			{Name: "a1", Program: "claude", ProjectPath: "/proj"},
		},
	}}
	m := New(agg, "test")
	m.width = 120
	m.height = 40

	out1 := m.renderDashboard()
	out2 := m.renderDashboard()
	if out1 != out2 {
		t.Error("identical state produced different dashboard output")
	}
}

func TestHashDispatchesDeterministic(t *testing.T) {
	kernel := &aggregator.KernelState{
		Dispatches: map[string][]icdata.Dispatch{
			"/proj/alpha": {{ID: "d1", Status: "active", AgentType: "claude"}},
			"/proj/beta":  {{ID: "d2", Status: "queued", AgentType: "codex"}},
		},
	}
	h1 := hashDispatches(kernel, 120)
	for i := 0; i < 100; i++ {
		h := hashDispatches(kernel, 120)
		if h != h1 {
			t.Fatalf("hashDispatches non-deterministic on iteration %d: %d vs %d", i, h1, h)
		}
	}
}
