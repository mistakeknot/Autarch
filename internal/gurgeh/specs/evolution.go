package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SpecRevision records a single version of a spec with its changelog.
type SpecRevision struct {
	ID        string    `yaml:"id"`
	SpecID    string    `yaml:"spec_id"`
	Version   int       `yaml:"version"`
	Timestamp time.Time `yaml:"timestamp"`
	Author    string    `yaml:"author"`  // "user", "arbiter", "pollard"
	Trigger   string    `yaml:"trigger"` // "manual", "signal:competitive", "signal:research", "agent_recommendation"
	Changes   []Change  `yaml:"changes"`
}

// Change describes a single field mutation in a spec revision.
type Change struct {
	Field      string `yaml:"field"`
	Before     string `yaml:"before"`
	After      string `yaml:"after"`
	Reason     string `yaml:"reason"`
	InsightRef string `yaml:"insight_ref,omitempty"` // Pollard insight ID
}

// historyDir returns the path to specs/history/ under the resolved data directory.
func historyDir(root string) string {
	return filepath.Join(resolveSpecsDir(root), "history")
}

// SaveRevision persists a spec revision as a full snapshot.
func SaveRevision(root string, spec *Spec, author, trigger string, changes []Change) (*SpecRevision, error) {
	dir := historyDir(root)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating history dir: %w", err)
	}

	version := spec.Version + 1
	spec.Version = version

	rev := &SpecRevision{
		ID:        fmt.Sprintf("%s_v%d", spec.ID, version),
		SpecID:    spec.ID,
		Version:   version,
		Timestamp: time.Now(),
		Author:    author,
		Trigger:   trigger,
		Changes:   changes,
	}

	// Save full snapshot
	data, err := yaml.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshaling spec: %w", err)
	}
	snapPath := filepath.Join(dir, fmt.Sprintf("%s_v%d.yaml", spec.ID, version))
	if err := os.WriteFile(snapPath, data, 0644); err != nil {
		return nil, fmt.Errorf("writing snapshot: %w", err)
	}

	// Save revision metadata alongside
	revData, err := yaml.Marshal(rev)
	if err != nil {
		return nil, fmt.Errorf("marshaling revision: %w", err)
	}
	revPath := filepath.Join(dir, fmt.Sprintf("%s_v%d_rev.yaml", spec.ID, version))
	if err := os.WriteFile(revPath, revData, 0644); err != nil {
		return nil, fmt.Errorf("writing revision: %w", err)
	}

	return rev, nil
}

// LoadHistory returns all revisions for a spec, ordered by version.
func LoadHistory(root, specID string) ([]SpecRevision, error) {
	dir := historyDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	prefix := specID + "_v"
	var revisions []SpecRevision
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, "_rev.yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var rev SpecRevision
		if err := yaml.Unmarshal(data, &rev); err != nil {
			continue
		}
		revisions = append(revisions, rev)
	}

	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].Version < revisions[j].Version
	})
	return revisions, nil
}

// LoadRevisionSpec loads the full spec snapshot for a given version.
func LoadRevisionSpec(root, specID string, version int) (Spec, error) {
	path := filepath.Join(historyDir(root), fmt.Sprintf("%s_v%d.yaml", specID, version))
	return LoadSpec(path)
}

// CheckAssumptionDecay evaluates assumptions and returns those that have decayed.
// Confidence drops one level when assumption age exceeds DecayDays without validation.
func CheckAssumptionDecay(spec *Spec) []Assumption {
	now := time.Now()
	var decayed []Assumption

	for i := range spec.Assumptions {
		a := &spec.Assumptions[i]
		decayDays := a.DecayDays
		if decayDays == 0 {
			decayDays = 30
		}

		refTime := spec.CreatedAt
		if a.ValidatedAt != "" {
			refTime = a.ValidatedAt
		}

		t, err := time.Parse(time.RFC3339, refTime)
		if err != nil {
			continue
		}

		age := now.Sub(t)
		if age > time.Duration(decayDays)*24*time.Hour {
			oldConf := a.Confidence
			switch a.Confidence {
			case "high":
				a.Confidence = "medium"
			case "medium":
				a.Confidence = "low"
			}
			if a.Confidence != oldConf {
				decayed = append(decayed, *a)
			}
		}
	}
	return decayed
}

// ParseVersion extracts a version number from "v3" or "3".
func ParseVersion(s string) (int, error) {
	s = strings.TrimPrefix(s, "v")
	return strconv.Atoi(s)
}
