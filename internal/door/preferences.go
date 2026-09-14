package door

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
	"github.com/mistakeknot/autarch/pkg/review"
)

const preferenceLimit = 1 << 20

type WorkbenchPreference struct {
	Outcome           string                `json:"outcome"`
	Task              review.TaskIdentity   `json:"task"`
	Target            agenttransport.Target `json:"target"`
	AccountLabel      string                `json:"account_label"`
	Draft             string                `json:"draft"`
	RecoveryHandoffID string                `json:"recovery_handoff_id,omitempty"`
	RecoveryTarget    agenttransport.Target `json:"recovery_target,omitempty"`
}

func DefaultPreferencePath() string {
	if path := os.Getenv("AUTARCH_PREFERENCES_FILE"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".autarch", "preferences.json")
}

func preferenceProjectKey(project string) string {
	if canonical, err := filepath.EvalSymlinks(project); err == nil {
		return canonical
	}
	if absolute, err := filepath.Abs(project); err == nil {
		return absolute
	}
	return project
}

func readPreferenceRoot(path string) (map[string]json.RawMessage, error) {
	root := map[string]json.RawMessage{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return root, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, preferenceLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > preferenceLimit {
		return nil, errors.New("preferences exceed 1 MiB")
	}
	if len(data) == 0 {
		return root, nil
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func LoadWorkbenchPreference(path, project string) (WorkbenchPreference, error) {
	root, err := readPreferenceRoot(path)
	if err != nil {
		return WorkbenchPreference{}, err
	}
	var projects map[string]json.RawMessage
	if raw := root["projects"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &projects); err != nil {
			return WorkbenchPreference{}, err
		}
	}
	if projects == nil {
		return WorkbenchPreference{}, nil
	}
	var preference WorkbenchPreference
	if raw := projects[preferenceProjectKey(project)]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &preference); err != nil {
			return WorkbenchPreference{}, err
		}
	}
	return preference, nil
}

func SaveWorkbenchPreference(path, project string, preference WorkbenchPreference) error {
	root, err := readPreferenceRoot(path)
	if err != nil {
		return err
	}
	projects := map[string]json.RawMessage{}
	if raw := root["projects"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &projects); err != nil {
			return err
		}
	}
	projectFields := map[string]json.RawMessage{}
	key := preferenceProjectKey(project)
	if raw := projects[key]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &projectFields); err != nil {
			return err
		}
	}
	known, err := json.Marshal(preference)
	if err != nil {
		return err
	}
	var knownFields map[string]json.RawMessage
	if err := json.Unmarshal(known, &knownFields); err != nil {
		return err
	}
	for field, value := range knownFields {
		projectFields[field] = value
	}
	projectData, err := json.Marshal(projectFields)
	if err != nil {
		return err
	}
	projects[key] = projectData
	projectsData, err := json.Marshal(projects)
	if err != nil {
		return err
	}
	root["projects"] = projectsData
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > preferenceLimit {
		return errors.New("preferences exceed 1 MiB")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".preferences-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	_ = d.Close()
	return err
}
