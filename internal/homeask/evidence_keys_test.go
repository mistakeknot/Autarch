package homeask

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareEvidenceExactKeys(t *testing.T) {
	t.Setenv("AUTARCH_HOME_EVIDENCE", filepath.Join(t.TempDir(), "store"))
	data, err := os.ReadFile("../../testdata/evidence-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name     string          `json:"name"`
		Evidence json.RawMessage `json:"evidence"`
	}
	json.Unmarshal(data, &vectors)
	for _, v := range vectors {
		if !strings.HasPrefix(v.Name, "casing ") {
			continue
		}
		t.Run(v.Name, func(t *testing.T) {
			var evidence any
			if err := json.Unmarshal(v.Evidence, &evidence); err != nil {
				t.Fatal(err)
			}
			if err := PrepareEvidence(map[string]any{"evidence": evidence}); err == nil {
				t.Fatal("accepted non-exact evidence keys")
			}
		})
	}
}

func TestCardOptionKeyCasing(t *testing.T) {
	for _, name := range []string{"Label", "LABEL"} {
		t.Run(name, func(t *testing.T) {
			r := testReq()
			option := r.Ask["options"].([]any)[0].(map[string]any)
			option[name] = option["label"]
			if name == "Label" {
				delete(option, "label")
			}
			if _, _, err := r.Description(); err == nil {
				t.Fatal("filed non-exact nested option key")
			}
		})
	}
}

// The typed v1 filing route already re-encodes non-evidence fields from structs.
func TestTypedAskCanonicalKeyCasing(t *testing.T) {
	raw, _ := json.Marshal(base())
	var m map[string]any
	json.Unmarshal(raw, &m)
	m["Question"] = m["question"]
	delete(m, "question")
	option := m["options"].([]any)[0].(map[string]any)
	option["Label"] = option["label"]
	delete(option, "label")
	raw, _ = json.Marshal(m)
	a, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := wireFromAsk(a)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(wire)
	if strings.Contains(string(raw), `"Question"`) || strings.Contains(string(raw), `"Label"`) {
		t.Fatal("typed filing retained caller key casing")
	}
}
