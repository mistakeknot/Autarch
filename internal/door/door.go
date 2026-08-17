// Package door is the thin project switcher from the 2026-08-13 universal
// interface brainstorm: one row per project (decision 2 / ruling 13), ordered
// by ruling 11 (funded, then pins, then weakest card first), with the card
// verdict and strength read from card-check.py rather than recomputed.
//
// The package deliberately contains no card rules. Every verdict and every
// score on screen is a transcription of `card-check.py --json` output; the
// checker is the single rule implementation shared with the LSP and the
// /write-plan gate, and the door consuming its JSON is what keeps a fourth
// consumer from becoming a second implementation.
package door

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Verdict mirrors card-check.py's four exit codes, plus one state the checker
// cannot express: Unchecked, meaning the door failed to run the checker at
// all. Unchecked is never rendered as Absent -- a query whose failure and
// empty result look alike fires on the wrong condition.
type Verdict string

const (
	VerdictConfirmed   Verdict = "confirmed"   // exit 0
	VerdictProvisional Verdict = "provisional" // exit 1
	VerdictInvalid     Verdict = "invalid"     // exit 2
	VerdictAbsent      Verdict = "absent"      // exit 3
	VerdictUnchecked   Verdict = "unchecked"   // checker missing, crashed, or unparseable
)

// Strength is card-check.py's strength block, verbatim. Only confirmed fields
// count toward Score -- that is the checker's rule (ruling 11), not ours.
type Strength struct {
	Score     int `json:"score"`
	Of        int `json:"of"`
	Confirmed int `json:"confirmed"`
	Drafted   int `json:"drafted"`
	Declined  int `json:"declined"`
}

// Project is one row: a git repository directly under a scan root.
type Project struct {
	Name     string
	Root     string
	CardPath string // Root/docs/why.md -- where the card is, or would go

	Verdict  Verdict
	Strength Strength
	Reason   string // checker's stated reason (shown for invalid cards)
	Err      error  // why the check itself failed; set only when Unchecked

	Funded   bool
	FundedAt int // position in the ranking file's funded list
	Pinned   bool
	PinnedAt int
}

// DiscoverProjects enumerates the estate: every immediate child of each root
// that is a git repository. This is intentionally not the bigend scanner --
// that one finds projects *with Autarch tooling*, which would silently drop
// most of the estate, and the whole point of the door is that a project with
// nothing in it still gets a row showing exactly that.
func DiscoverProjects(roots []string) ([]Project, error) {
	var projects []Project
	seen := make(map[string]bool)
	var lastErr error

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			lastErr = fmt.Errorf("scan root %s: %w", root, err)
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || e.Name()[0] == '.' {
				continue
			}
			path := filepath.Join(root, e.Name())
			// A .git *file* is a worktree or submodule pointer -- still a repo.
			if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
				continue
			}
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				path = resolved
			}
			if seen[path] {
				continue
			}
			seen[path] = true
			projects = append(projects, Project{
				Name:     e.Name(),
				Root:     path,
				CardPath: filepath.Join(path, "docs", "why.md"),
				Verdict:  VerdictUnchecked,
			})
		}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })

	if len(projects) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return projects, nil
}

// Ranking is the file feeding ruling 11's first two tiers. `funded` belongs to
// the GSV funded-attention mechanism (decision 6) and the door never writes
// it; `pins` are mk's manual picks and the door owns them (the p key).
type Ranking struct {
	Funded []string `yaml:"funded"`
	Pins   []string `yaml:"pins"`
}

// DefaultRankingPath is ~/.autarch/door.yaml.
func DefaultRankingPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".autarch", "door.yaml")
}

// LoadRanking reads the ranking file. An absent file is an empty ranking --
// the order honestly degrades to weakest-first. A malformed file is an error,
// never an empty ranking: silently dropping mk's funded list would reorder
// the entire estate while looking like a working door.
func LoadRanking(path string) (Ranking, error) {
	var r Ranking
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err := yaml.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("ranking file %s: %w", path, err)
	}
	return r, nil
}

// SaveRanking writes the ranking file. Comments do not survive a save, so the
// header says who owns what instead of leaving that to folklore.
func SaveRanking(path string, r Ranking) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	header := "# autarch door ranking (ruling 11, tiers 1-2).\n" +
		"# funded: owned by the GSV funded-attention mechanism -- the door never writes it.\n" +
		"# pins: owned by the door's p key. Hand-edits are fine while the door is closed.\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}

// TogglePin flips a project in the pins list and reports the new state.
func (r *Ranking) TogglePin(name string) bool {
	for i, p := range r.Pins {
		if p == name {
			r.Pins = append(r.Pins[:i], r.Pins[i+1:]...)
			return false
		}
	}
	r.Pins = append(r.Pins, name)
	return true
}

// Apply stamps funded/pin membership onto the projects. Funded outranks
// pinned when a project is both, so it is counted once, in the higher tier.
func (r Ranking) Apply(projects []Project) {
	funded := make(map[string]int, len(r.Funded))
	for i, name := range r.Funded {
		funded[name] = i
	}
	pins := make(map[string]int, len(r.Pins))
	for i, name := range r.Pins {
		pins[name] = i
	}
	for i := range projects {
		p := &projects[i]
		p.FundedAt, p.Funded = funded[p.Name]
		if p.Funded {
			p.Pinned = false
		} else {
			p.PinnedAt, p.Pinned = pins[p.Name]
		}
	}
}

// verdictWeight orders verdicts within a score tie, weakest first. Invalid
// outranks absent because it is a regression: the card was presumably fine
// once and is now lying, where absent is a gap nobody has opened yet.
func verdictWeight(v Verdict) int {
	switch v {
	case VerdictInvalid:
		return 0
	case VerdictAbsent:
		return 1
	case VerdictProvisional:
		return 2
	case VerdictConfirmed:
		return 3
	default: // VerdictUnchecked -- grouped last by Rank before weight applies
		return 4
	}
}

// Rank sorts in place by ruling 11: funded (file order), then pins (file
// order), then the tail weakest card first by strength.score, tie-broken by
// verdictWeight then name.
//
// Unchecked rows sort to the very end as a visibly separate group rather than
// being ranked: placing them "weakest first" would assert a weakness nobody
// measured, and hiding them among absent rows would be the intermux 1-of-21
// failure this surface exists to replace. The footer counts them.
func Rank(projects []Project) {
	sort.SliceStable(projects, func(i, j int) bool {
		a, b := projects[i], projects[j]
		if a.Funded != b.Funded {
			return a.Funded
		}
		if a.Funded {
			return a.FundedAt < b.FundedAt
		}
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if a.Pinned {
			return a.PinnedAt < b.PinnedAt
		}
		au, bu := a.Verdict == VerdictUnchecked, b.Verdict == VerdictUnchecked
		if au != bu {
			return bu // unchecked group last
		}
		if a.Strength.Score != b.Strength.Score {
			return a.Strength.Score < b.Strength.Score
		}
		if wa, wb := verdictWeight(a.Verdict), verdictWeight(b.Verdict); wa != wb {
			return wa < wb
		}
		return a.Name < b.Name
	})
}

// Coverage is the cards-axis disclosure for the footer: the estate total and
// every verdict's share of it, unchecked included so a dead checker cannot
// present as a fully-absent estate.
type Coverage struct {
	Total       int
	Confirmed   int
	Provisional int
	Invalid     int
	Absent      int
	Unchecked   int
}

// Cover tallies verdicts across the estate.
func Cover(projects []Project) Coverage {
	c := Coverage{Total: len(projects)}
	for _, p := range projects {
		switch p.Verdict {
		case VerdictConfirmed:
			c.Confirmed++
		case VerdictProvisional:
			c.Provisional++
		case VerdictInvalid:
			c.Invalid++
		case VerdictAbsent:
			c.Absent++
		default:
			c.Unchecked++
		}
	}
	return c
}
