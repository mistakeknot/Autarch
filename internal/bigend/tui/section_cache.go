package tui

import (
	"encoding/binary"
	"hash/fnv"
	"sort"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	"github.com/mistakeknot/autarch/internal/icdata"
)

// sectionID identifies a cacheable dashboard section.
type sectionID int

const (
	sectionStats sectionID = iota
	sectionRuns
	sectionDispatches
	sectionSessions
	sectionAgents
	sectionActivity
	sectionInterspect
)

// sectionEntry holds a cached render result and its data hash.
type sectionEntry struct {
	rendered string
	hash     uint64
}

// sectionCache stores per-section render results keyed by sectionID.
type sectionCache struct {
	entries map[sectionID]sectionEntry
}

func newSectionCache() *sectionCache {
	return &sectionCache{entries: make(map[sectionID]sectionEntry, 7)}
}

// getOrRender returns cached output if hash matches, otherwise calls renderFn.
func (c *sectionCache) getOrRender(id sectionID, hash uint64, renderFn func() string) string {
	if entry, ok := c.entries[id]; ok && entry.hash == hash {
		return entry.rendered
	}
	s := renderFn()
	c.entries[id] = sectionEntry{rendered: s, hash: hash}
	return s
}

// invalidateAll clears the entire cache (used on resize).
func (c *sectionCache) invalidateAll() {
	for k := range c.entries {
		delete(c.entries, k)
	}
}

// --- Per-section hash functions ---
// Each hashes the fields that the corresponding renderDashboard section reads.
// Width is included because lipgloss layout depends on terminal width.

func hashStats(state aggregator.State, width int) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(width))
	h.Write(b)
	binary.LittleEndian.PutUint64(b, uint64(len(state.Projects)))
	h.Write(b)
	binary.LittleEndian.PutUint64(b, uint64(len(state.Sessions)))
	h.Write(b)
	binary.LittleEndian.PutUint64(b, uint64(len(state.Agents)))
	h.Write(b)
	var activeCount int
	for _, s := range state.Sessions {
		if s.UnifiedState == icdata.StatusActive || s.UnifiedState == icdata.StatusWaiting {
			activeCount++
		}
	}
	binary.LittleEndian.PutUint64(b, uint64(activeCount))
	h.Write(b)
	if state.Kernel != nil {
		km := state.Kernel.Metrics
		binary.LittleEndian.PutUint64(b, uint64(km.ActiveRuns))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(km.ActiveDispatches))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(km.BlockedAgents))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(km.TotalTokensIn))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(km.TotalTokensOut))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(len(km.KernelErrors)))
		h.Write(b)
		kernelCount := 0
		for _, p := range state.Projects {
			if p.HasIntercore {
				kernelCount++
			}
		}
		binary.LittleEndian.PutUint64(b, uint64(kernelCount))
		h.Write(b)
	}
	return h.Sum64()
}

func hashRuns(kernel *aggregator.KernelState, width int) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(width))
	h.Write(b)
	if kernel == nil {
		return h.Sum64()
	}
	// Sort map keys for deterministic hashing (Go map iteration is random).
	projects := make([]string, 0, len(kernel.Runs))
	for proj := range kernel.Runs {
		projects = append(projects, proj)
	}
	sort.Strings(projects)
	for _, proj := range projects {
		h.Write([]byte(proj))
		for _, r := range kernel.Runs[proj] {
			h.Write([]byte(r.ID))
			h.Write([]byte(r.Status))
			h.Write([]byte(r.Phase))
			h.Write([]byte(r.Goal))
		}
	}
	return h.Sum64()
}

func hashDispatches(kernel *aggregator.KernelState, width int) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(width))
	h.Write(b)
	if kernel == nil {
		return h.Sum64()
	}
	// Sort map keys for deterministic hashing (Go map iteration is random).
	projects := make([]string, 0, len(kernel.Dispatches))
	for proj := range kernel.Dispatches {
		projects = append(projects, proj)
	}
	sort.Strings(projects)
	for _, proj := range projects {
		h.Write([]byte(proj))
		for _, d := range kernel.Dispatches[proj] {
			h.Write([]byte(d.ID))
			h.Write([]byte(d.Status))
			h.Write([]byte(d.AgentType))
			binary.LittleEndian.PutUint64(b, uint64(d.InTokens))
			h.Write(b)
			binary.LittleEndian.PutUint64(b, uint64(d.OutTokens))
			h.Write(b)
		}
	}
	return h.Sum64()
}

func hashSessions(sessions []aggregator.TmuxSession, limit int) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(len(sessions)))
	h.Write(b)
	for i, s := range sessions {
		if i >= limit {
			break
		}
		h.Write([]byte(s.Name))
		h.Write([]byte(s.AgentName))
		h.Write([]byte(s.ProjectPath))
		binary.LittleEndian.PutUint64(b, uint64(s.UnifiedState))
		h.Write(b)
	}
	return h.Sum64()
}

func hashAgents(agents []aggregator.Agent, limit int) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(len(agents)))
	h.Write(b)
	for i, a := range agents {
		if i >= limit {
			break
		}
		h.Write([]byte(a.Name))
		h.Write([]byte(a.Program))
		h.Write([]byte(a.ProjectPath))
	}
	return h.Sum64()
}

func hashInterspect(projects []discovery.Project, width int) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(width))
	h.Write(b)
	for _, p := range projects {
		if p.InterspectStats == nil {
			continue
		}
		h.Write([]byte(p.Path))
		s := p.InterspectStats
		binary.LittleEndian.PutUint64(b, uint64(s.TotalEvents))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(s.Sessions))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(s.Dispatches))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(s.Advances))
		h.Write(b)
		binary.LittleEndian.PutUint64(b, uint64(s.Blocks))
		h.Write(b)
	}
	return h.Sum64()
}

func hashActivities(activities []aggregator.Activity, limit int) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(len(activities)))
	h.Write(b)
	for i, a := range activities {
		if i >= limit {
			break
		}
		h.Write([]byte(a.Summary))
		h.Write([]byte(a.Source))
		h.Write([]byte(a.AgentName))
		ts := a.Time.UnixNano()
		binary.LittleEndian.PutUint64(b, uint64(ts))
		h.Write(b)
	}
	return h.Sum64()
}
