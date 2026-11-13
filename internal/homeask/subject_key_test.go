package homeask

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type subjectVector struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata"`
	Valid    bool           `json:"valid"`
	RootRun  *RootRun       `json:"root_run"`
}

func subjectVectors(t *testing.T) []subjectVector {
	raw, err := os.ReadFile("../../testdata/subject-key-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v []subjectVector
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v) < 60 {
		t.Fatal("too few vectors")
	}
	return v
}
func vectorDescription(t *testing.T, v subjectVector) string {
	req := testReq()
	req.Ask = map[string]any{"schema": "home-ask/v2", "project": "fixture", "project_root": "/fixture", "question": "Choice?", "options": []any{map[string]any{"id": "a", "label": "A", "kind": "ruling-only"}, map[string]any{"id": "b", "label": "B", "kind": "ruling-only"}}}
	req.RootRun = v.RootRun
	for k, x := range v.Metadata {
		req.Ask[k] = x
	}
	raw, _ := json.Marshal(req.Ask)
	desc := "Request: fixture sha256:0123456789abcdef\n```home-ask\n" + string(raw) + "\n```"
	if v.RootRun != nil {
		r := v.RootRun
		desc += "\n```root-run\nscript: " + r.Script + "\nsha256: " + r.SHA256 + "\ntimeout: 60\nset: fixture\n```"
	}
	return desc
}
func TestSubjectKeyVectors(t *testing.T) {
	for _, v := range subjectVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			c, err := ParseCard(vectorDescription(t, v))
			if (err == nil) != v.Valid {
				t.Fatalf("valid=%v err=%v", v.Valid, err)
			}
			if err == nil {
				for key, missing := range map[string]bool{"subject_key": c.SubjectKey == nil, "signals": c.Signals == nil} {
					if _, present := v.Metadata[key]; present && missing {
						t.Errorf("Card lacks %s", key)
					}
				}
			}
		})
	}
}
func TestMetadataFrozenHashes(t *testing.T) {
	meta := subjectVectors(t)[1].Metadata
	delete(meta, "subject")
	for _, row := range hashGoldens(t) {
		if row.Description == "" {
			continue
		}
		original, err := ParseCard(row.Description)
		if err != nil {
			t.Fatal(err)
		}
		ask, err := original.ToV1(row.Thread)
		if err != nil {
			t.Fatal(err)
		}
		explicitNone := false
		for _, option := range ask.Options {
			if len(option.OnPick) == 0 {
				continue
			}
			var declaration struct{ Subject struct{ Kind string } }
			if err := json.Unmarshal(option.OnPick, &declaration); err != nil {
				t.Fatal(err)
			}
			explicitNone = explicitNone || declaration.Subject.Kind == "none"
		}
		row.Description = goldenMetadata(t, row.Description, meta)
		// A participating explicit-none declaration contradicts newly attached
		// artifact metadata. It must refuse rather than acquire unchanged hashes.
		if explicitNone {
			if _, err := ParseCard(row.Description); err == nil {
				t.Errorf("accepted contradictory artifact: %s", row.Name)
			}
			continue
		}
		got := goldenHashes(t, row)
		if !reflect.DeepEqual(got, row.Expected) {
			t.Errorf("metadata changed hashes: %s", row.Name)
		}
	}
}
func TestV1MetadataRejectedAndLegacySubjectHashed(t *testing.T) {
	row := hashGoldens(t)[0]
	c, err := ParseCard(row.Description)
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.ToV1(row.Thread)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(a)
	var m map[string]any
	json.Unmarshal(raw, &m)
	for _, key := range []string{"subject_key", "signals"} {
		m[key] = nil
		raw, _ = json.Marshal(m)
		if _, err := Parse(raw); err == nil {
			t.Errorf("v1 accepts %s", key)
		}
		delete(m, key)
	}
	a.Subject = "legacy A"
	raw, _ = json.Marshal(a)
	a, err = Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	b := a
	b.Subject = "legacy B"
	if Identity(a) == Identity(b) {
		t.Fatal("subject lost from Identity")
	}
	c.Ask["subject"] = "legacy A"
	h1, _ := RequestIdentity(CardRequest{Ask: c.Ask, Pull: c.Pull != ""})
	c.Ask["subject"] = "legacy B"
	h2, _ := RequestIdentity(CardRequest{Ask: c.Ask, Pull: c.Pull != ""})
	if h1 == h2 {
		t.Fatal("subject lost from H1")
	}
}

func TestMetadataValuesRoundTrip(t *testing.T) {
	for _, v := range subjectVectors(t) {
		if !v.Valid {
			continue
		}
		t.Run(v.Name, func(t *testing.T) {
			c, err := ParseCard(vectorDescription(t, v))
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range map[string]any{"subject_key": reflect.ValueOf(c).FieldByName("SubjectKey").Interface(), "signals": reflect.ValueOf(c).FieldByName("Signals").Interface()} {
				if want, ok := v.Metadata[key]; ok {
					if !reflect.DeepEqual(generic(t, value), want) {
						t.Errorf("%s roundtrip: got %v want %v", key, generic(t, value), want)
					}
				}
			}
		})
	}
}
func TestRequestIdentityDropsMetadata(t *testing.T) {
	for _, row := range hashGoldens(t) {
		if row.Description == "" {
			continue
		}
		c, err := ParseCard(row.Description)
		if err != nil {
			t.Fatal(err)
		}
		req := CardRequest{Title: "Fixture", Ask: c.Ask, Pull: c.Pull != ""}
		before, err := RequestIdentity(req)
		if err != nil {
			continue
		}
		for k, v := range subjectVectors(t)[1].Metadata {
			if k != "subject" {
				req.Ask[k] = v
			}
		}
		after, err := RequestIdentity(req)
		if err != nil || before != after {
			t.Fatalf("metadata changed RequestIdentity %s: %s %s %v", row.Name, before, after, err)
		}
	}
}
