package specs

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PraudemapFeature represents a feature in the roadmap
type PraudemapFeature struct {
	ID        string   `yaml:"id"`
	Title     string   `yaml:"title"`
	Status    string   `yaml:"status"`
	DependsOn []string `yaml:"depends_on,omitempty"`
}

// PraudemapVersion represents a version milestone in the roadmap
type PraudemapVersion struct {
	ID         string             `yaml:"id"`
	Title      string             `yaml:"title"`
	TargetDate string             `yaml:"target_date,omitempty"`
	Features   []PraudemapFeature `yaml:"features"`
}

// Praudemap represents the product roadmap visualization
type Praudemap struct {
	Name     string             `yaml:"name"`
	Versions []PraudemapVersion `yaml:"versions"`
}

// LoadPraudemap reads the praudemap from a project
func LoadPraudemap(projectPath string) (*Praudemap, error) {
	path := filepath.Join(projectPath, ".praude", "praudemap.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pm Praudemap
	if err := yaml.Unmarshal(data, &pm); err != nil {
		return nil, err
	}
	return &pm, nil
}

// Save writes the praudemap to a project
func (pm *Praudemap) Save(projectPath string) error {
	praudeDir := filepath.Join(projectPath, ".praude")
	if err := os.MkdirAll(praudeDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(pm)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(praudeDir, "praudemap.yaml"), data, 0644)
}

// GetActiveVersion returns the version currently in progress
func (pm *Praudemap) GetActiveVersion() *PraudemapVersion {
	for i := range pm.Versions {
		v := &pm.Versions[i]
		for _, f := range v.Features {
			if f.Status == "in_progress" {
				return v
			}
		}
	}
	// Return first version with non-done features
	for i := range pm.Versions {
		v := &pm.Versions[i]
		for _, f := range v.Features {
			if f.Status != "done" {
				return v
			}
		}
	}
	return nil
}

// GetFeatureDependencies returns features that a given feature depends on
func (pm *Praudemap) GetFeatureDependencies(featureID string) []string {
	for _, v := range pm.Versions {
		for _, f := range v.Features {
			if f.ID == featureID {
				return f.DependsOn
			}
		}
	}
	return nil
}

// GetDependentFeatures returns features that depend on the given feature
func (pm *Praudemap) GetDependentFeatures(featureID string) []string {
	var dependents []string
	for _, v := range pm.Versions {
		for _, f := range v.Features {
			for _, dep := range f.DependsOn {
				if dep == featureID {
					dependents = append(dependents, f.ID)
					break
				}
			}
		}
	}
	return dependents
}

// SyncFromPRDs updates the praudemap from PRD data
func (pm *Praudemap) SyncFromPRDs(prds []*PRD) {
	for _, prd := range prds {
		// Find or create version
		var version *PraudemapVersion
		for i := range pm.Versions {
			if pm.Versions[i].ID == prd.Version {
				version = &pm.Versions[i]
				break
			}
		}
		if version == nil {
			pm.Versions = append(pm.Versions, PraudemapVersion{
				ID:    prd.Version,
				Title: prd.Title,
			})
			version = &pm.Versions[len(pm.Versions)-1]
		}

		// Sync features
		for _, f := range prd.Features {
			found := false
			for i := range version.Features {
				if version.Features[i].ID == f.ID {
					version.Features[i].Title = f.Title
					version.Features[i].Status = string(f.Status)
					found = true
					break
				}
			}
			if !found {
				version.Features = append(version.Features, PraudemapFeature{
					ID:     f.ID,
					Title:  f.Title,
					Status: string(f.Status),
				})
			}
		}
	}
}

// NewPraudemap creates a new praudemap
func NewPraudemap(name string) *Praudemap {
	return &Praudemap{
		Name:     name,
		Versions: []PraudemapVersion{},
	}
}
