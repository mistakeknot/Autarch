package discovery

import (
	"os"
	"path/filepath"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

// ColdwineDir is the config directory for coldwine
const ColdwineDir = ".coldwine"

// ColdwineEpic represents an epic from Coldwine.
type ColdwineEpic struct {
	ID                 string   `yaml:"id"`
	Title              string   `yaml:"title"`
	Summary            string   `yaml:"summary"`
	Status             string   `yaml:"status"`
	Priority           string   `yaml:"priority"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria"`
	Stories            []struct {
		ID                 string   `yaml:"id"`
		Title              string   `yaml:"title"`
		Status             string   `yaml:"status"`
		Priority           string   `yaml:"priority"`
		AcceptanceCriteria []string `yaml:"acceptance_criteria"`
	} `yaml:"stories"`
}

// ColdwineEpics loads all epics from a project's .coldwine/specs directory.
// Parse errors are silently ignored. Use ColdwineEpicsWithErrors for error details.
func ColdwineEpics(root string) ([]ColdwineEpic, error) {
	epics, _ := ColdwineEpicsWithErrors(root)
	return epics, nil
}

// ColdwineEpicsWithErrors loads all epics and returns both successfully parsed epics
// and any parse errors encountered.
func ColdwineEpicsWithErrors(root string) ([]ColdwineEpic, []ParseError) {
	specsDir := filepath.Join(root, ColdwineDir, "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ColdwineEpic{}, nil
		}
		return nil, []ParseError{{Path: specsDir, Err: err}}
	}

	var epics []ColdwineEpic
	var errs []ParseError
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(specsDir, entry.Name())
		var epic ColdwineEpic
		if _, err := yamlsafe.UnmarshalFile(path, &epic); err != nil {
			errs = append(errs, ParseError{Path: path, Err: err})
			continue
		}
		if epic.ID != "" {
			epics = append(epics, epic)
		}
	}
	return epics, errs
}

// FindEpic finds an epic by ID.
func FindEpic(root, id string) (*ColdwineEpic, error) {
	epics, err := ColdwineEpics(root)
	if err != nil {
		return nil, err
	}
	for _, epic := range epics {
		if epic.ID == id {
			return &epic, nil
		}
	}
	return nil, nil
}

// CountColdwineEpics returns the total number of epics.
func CountColdwineEpics(root string) int {
	epics, _ := ColdwineEpics(root)
	return len(epics)
}

// CountColdwineStories returns the total number of stories across all epics.
func CountColdwineStories(root string) int {
	epics, _ := ColdwineEpics(root)
	count := 0
	for _, e := range epics {
		count += len(e.Stories)
	}
	return count
}

// ColdwineHasData returns true if Coldwine has any epics.
func ColdwineHasData(root string) bool {
	return CountColdwineEpics(root) > 0
}

// EpicsByStatus returns epics grouped by status.
func EpicsByStatus(root string) (map[string][]ColdwineEpic, error) {
	epics, err := ColdwineEpics(root)
	if err != nil {
		return nil, err
	}

	byStatus := make(map[string][]ColdwineEpic)
	for _, e := range epics {
		status := e.Status
		if status == "" {
			status = "unknown"
		}
		byStatus[status] = append(byStatus[status], e)
	}
	return byStatus, nil
}

// BlockedEpics returns epics with blocked status.
func BlockedEpics(root string) ([]ColdwineEpic, error) {
	epics, err := ColdwineEpics(root)
	if err != nil {
		return nil, err
	}

	var blocked []ColdwineEpic
	for _, e := range epics {
		if e.Status == "blocked" {
			blocked = append(blocked, e)
		}
	}
	return blocked, nil
}

// EpicsWithoutAcceptanceCriteria returns epics missing acceptance criteria.
func EpicsWithoutAcceptanceCriteria(root string) ([]ColdwineEpic, error) {
	epics, err := ColdwineEpics(root)
	if err != nil {
		return nil, err
	}

	var missing []ColdwineEpic
	for _, e := range epics {
		if len(e.AcceptanceCriteria) == 0 {
			missing = append(missing, e)
		}
	}
	return missing, nil
}
