package homeask

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceSharedVectors(t *testing.T) {
	data, err := os.ReadFile("../../testdata/evidence-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name     string          `json:"name"`
		Evidence json.RawMessage `json:"evidence"`
		Valid    bool            `json:"valid"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) < 50 {
		t.Fatal("missing parity vectors")
	}
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			data, _ := json.Marshal(base())
			var ask map[string]any
			json.Unmarshal(data, &ask)
			ask["evidence"] = v.Evidence
			data, _ = json.Marshal(ask)
			_, err := Parse(data)
			if (err == nil) != v.Valid {
				t.Fatalf("valid=%v, error=%v", v.Valid, err)
			}
		})
	}
}

func TestEvidenceRootOverride(t *testing.T) {
	t.Setenv("AUTARCH_HOME_EVIDENCE", "")
	want := EvidenceRoot()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if want != filepath.Join(home, ".autarch", "home-evidence") {
		t.Fatal("empty override did not select default")
	}
	t.Setenv("AUTARCH_HOME_EVIDENCE", "relative/evidence")
	if EvidenceRoot() != want {
		t.Fatal("relative override did not select default")
	}
	absolute := filepath.Join(t.TempDir(), "evidence")
	t.Setenv("AUTARCH_HOME_EVIDENCE", absolute)
	if EvidenceRoot() != absolute {
		t.Fatal("absolute override ignored")
	}
}
