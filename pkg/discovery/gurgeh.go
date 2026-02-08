package discovery

import (
	"os"
	"path/filepath"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

// GurgDir is the config directory for gurgeh
const GurgDir = ".gurgeh"

// GurgSpec represents a PRD spec from Gurgeh.
type GurgSpec struct {
	ID           string   `yaml:"id"`
	Title        string   `yaml:"title"`
	Status       string   `yaml:"status"`
	Summary      string   `yaml:"summary"`
	Requirements []string `yaml:"requirements"`
	Acceptance   []struct {
		ID          string `yaml:"id"`
		Description string `yaml:"description"`
	} `yaml:"acceptance_criteria"`
	CriticalUserJourneys []struct {
		ID       string   `yaml:"id"`
		Title    string   `yaml:"title"`
		Priority string   `yaml:"priority"`
		Steps    []string `yaml:"steps"`
	} `yaml:"critical_user_journeys"`
}

// GurgPRD represents a PRD document (with features).
type GurgPRD struct {
	ID       string `yaml:"id"`
	Title    string `yaml:"title"`
	Version  string `yaml:"version"`
	Status   string `yaml:"status"`
	Features []struct {
		ID                 string   `yaml:"id"`
		Title              string   `yaml:"title"`
		Status             string   `yaml:"status"`
		Summary            string   `yaml:"summary"`
		Requirements       []string `yaml:"requirements"`
		AcceptanceCriteria []struct {
			ID          string `yaml:"id"`
			Description string `yaml:"description"`
		} `yaml:"acceptance_criteria"`
	} `yaml:"features"`
}

// GurgSpecs loads all specs from a project's .gurgeh/specs directory.
// Parse errors are silently ignored. Use GurgSpecsWithErrors for error details.
func GurgSpecs(root string) ([]GurgSpec, error) {
	specs, _ := GurgSpecsWithErrors(root)
	return specs, nil
}

// GurgSpecsWithErrors loads all specs and returns both successfully parsed specs
// and any parse errors encountered. This allows callers to handle partial results
// gracefully while still being informed about malformed files.
func GurgSpecsWithErrors(root string) ([]GurgSpec, []ParseError) {
	specsDir := filepath.Join(root, GurgDir, "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []GurgSpec{}, nil
		}
		return nil, []ParseError{{Path: specsDir, Err: err}}
	}

	var specs []GurgSpec
	var errs []ParseError
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(specsDir, entry.Name())
		data, err := yamlsafe.ReadFile(path)
		if err != nil {
			errs = append(errs, ParseError{Path: path, Err: err})
			continue
		}
		var spec GurgSpec
		if err := yamlsafe.Decode(data, &spec); err != nil {
			errs = append(errs, ParseError{Path: path, Err: err})
			continue
		}
		// Skip if it looks like a PRD (has version field)
		var prd GurgPRD
		if yamlsafe.Decode(data, &prd) == nil && prd.Version != "" {
			continue
		}
		if spec.ID != "" {
			specs = append(specs, spec)
		}
	}
	return specs, errs
}

// GurgPRDs loads all PRDs from a project's .gurgeh/specs directory.
// Parse errors are silently ignored. Use GurgPRDsWithErrors for error details.
func GurgPRDs(root string) ([]GurgPRD, error) {
	prds, _ := GurgPRDsWithErrors(root)
	return prds, nil
}

// GurgPRDsWithErrors loads all PRDs and returns both successfully parsed PRDs
// and any parse errors encountered.
func GurgPRDsWithErrors(root string) ([]GurgPRD, []ParseError) {
	specsDir := filepath.Join(root, GurgDir, "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []GurgPRD{}, nil
		}
		return nil, []ParseError{{Path: specsDir, Err: err}}
	}

	var prds []GurgPRD
	var errs []ParseError
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(specsDir, entry.Name())
		var prd GurgPRD
		if _, err := yamlsafe.UnmarshalFile(path, &prd); err != nil {
			errs = append(errs, ParseError{Path: path, Err: err})
			continue
		}
		// Only include files with the PRD structure (has Version field)
		if prd.Version != "" {
			prds = append(prds, prd)
		}
	}
	return prds, errs
}

// FindPRD finds a PRD by ID or version name.
func FindPRD(root, idOrVersion string) (*GurgPRD, error) {
	prds, err := GurgPRDs(root)
	if err != nil {
		return nil, err
	}
	for _, prd := range prds {
		if prd.ID == idOrVersion || prd.Version == idOrVersion {
			return &prd, nil
		}
	}
	return nil, nil
}

// FindSpec finds a spec by ID.
func FindSpec(root, id string) (*GurgSpec, error) {
	specs, err := GurgSpecs(root)
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		if spec.ID == id {
			return &spec, nil
		}
	}
	return nil, nil
}

// CountGurgSpecs returns the total number of specs.
func CountGurgSpecs(root string) int {
	specs, _ := GurgSpecs(root)
	return len(specs)
}

// CountGurgPRDs returns the total number of PRDs.
func CountGurgPRDs(root string) int {
	prds, _ := GurgPRDs(root)
	return len(prds)
}

// CountPRDFeatures returns the total number of features across all PRDs.
func CountPRDFeatures(root string) int {
	prds, _ := GurgPRDs(root)
	count := 0
	for _, prd := range prds {
		count += len(prd.Features)
	}
	return count
}

// GurgHasData returns true if Gurgeh has any specs or PRDs.
func GurgHasData(root string) bool {
	return CountGurgSpecs(root) > 0 || CountGurgPRDs(root) > 0
}

// GetApprovedFeatures returns all features with approved or in_progress status.
func GetApprovedFeatures(root string) ([]struct {
	PRDID   string
	Feature struct {
		ID      string
		Title   string
		Status  string
		Summary string
	}
}, error) {
	prds, err := GurgPRDs(root)
	if err != nil {
		return nil, err
	}

	var features []struct {
		PRDID   string
		Feature struct {
			ID      string
			Title   string
			Status  string
			Summary string
		}
	}

	for _, prd := range prds {
		for _, f := range prd.Features {
			if f.Status == "approved" || f.Status == "in_progress" {
				features = append(features, struct {
					PRDID   string
					Feature struct {
						ID      string
						Title   string
						Status  string
						Summary string
					}
				}{
					PRDID: prd.ID,
					Feature: struct {
						ID      string
						Title   string
						Status  string
						Summary string
					}{
						ID:      f.ID,
						Title:   f.Title,
						Status:  f.Status,
						Summary: f.Summary,
					},
				})
			}
		}
	}
	return features, nil
}
