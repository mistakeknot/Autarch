package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	fileutil "github.com/mistakeknot/autarch/internal/file"
	"github.com/mistakeknot/autarch/pkg/yamlsafe"
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
	if spec == nil {
		return nil, fmt.Errorf("nil spec")
	}

	dir := historyDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating history dir: %w", err)
	}

	// Serialize revisions per spec ID so concurrent writers cannot pick the same version.
	lockPath := filepath.Join(dir, "."+StoryHash(spec.ID)+".history")
	lock, err := fileutil.LockFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("acquiring history lock: %w", err)
	}
	if lock != nil {
		defer func() {
			_ = lock.Unlock()
		}()
	}

	version, err := nextHistoryVersion(dir, spec.ID)
	if err != nil {
		return nil, fmt.Errorf("computing next version: %w", err)
	}

	rev := &SpecRevision{
		ID:        fmt.Sprintf("%s_v%d", spec.ID, version),
		SpecID:    spec.ID,
		Version:   version,
		Timestamp: time.Now(),
		Author:    author,
		Trigger:   trigger,
		Changes:   append([]Change(nil), changes...),
	}

	snapshot := *spec
	snapshot.Version = version

	// Save full snapshot
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshaling spec: %w", err)
	}
	snapPath := filepath.Join(dir, fmt.Sprintf("%s_v%d.yaml", spec.ID, version))
	if err := fileutil.AtomicWriteFile(snapPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("writing snapshot: %w", err)
	}

	// Save revision metadata alongside
	revData, err := yaml.Marshal(rev)
	if err != nil {
		return nil, fmt.Errorf("marshaling revision: %w", err)
	}
	revPath := filepath.Join(dir, fmt.Sprintf("%s_v%d_rev.yaml", spec.ID, version))
	if err := fileutil.AtomicWriteFile(revPath, revData, 0o644); err != nil {
		// Best-effort rollback so we do not keep a snapshot without metadata when metadata write fails.
		_ = os.Remove(snapPath)
		return nil, fmt.Errorf("writing revision: %w", err)
	}

	return rev, nil
}

func nextHistoryVersion(dir, specID string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	prefix := specID + "_v"
	maxVersion := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		version, ok := parseHistoryVersion(e.Name(), prefix)
		if !ok {
			continue
		}
		if version > maxVersion {
			maxVersion = version
		}
	}
	return maxVersion + 1, nil
}

func parseHistoryVersion(name, prefix string) (int, bool) {
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}

	versionPart := strings.TrimPrefix(name, prefix)
	switch {
	case strings.HasSuffix(versionPart, "_rev.yaml"):
		versionPart = strings.TrimSuffix(versionPart, "_rev.yaml")
	case strings.HasSuffix(versionPart, ".yaml"):
		versionPart = strings.TrimSuffix(versionPart, ".yaml")
	default:
		return 0, false
	}

	version, err := strconv.Atoi(versionPart)
	if err != nil || version <= 0 {
		return 0, false
	}
	return version, true
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
		var rev SpecRevision
		if _, err := yamlsafe.UnmarshalFile(filepath.Join(dir, name), &rev); err != nil {
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
