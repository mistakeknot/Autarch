package agent

import (
	"strings"
	"testing"
)

type fakeLookup struct {
	files map[string]string
}

func (f fakeLookup) Exists(path string) bool {
	_, ok := f.files[path]
	return ok
}

func (f fakeLookup) ContainsQuote(path, quote string) bool {
	content, ok := f.files[path]
	if !ok {
		return false
	}
	return strings.Contains(content, quote)
}

func TestValidatePhaseArtifact_RejectsUnknownField(t *testing.T) {
	input := []byte(`{
"phase":"vision",
"version":"v1",
"summary":"This is a sufficiently long summary for validation.",
"goals":["g"],
"non_goals":[],
"evidence":[{"type":"file","path":"README.md","quote":"hello world","confidence":0.9},{"type":"doc","path":"docs/ARCHITECTURE.md","quote":"arch","confidence":0.9}],
"open_questions":[],
"quality":{"clarity":1,"completeness":1,"grounding":1,"consistency":1},
"extra":"nope"
}`)
	res := ValidatePhaseArtifact("vision", input, fakeLookup{files: map[string]string{"README.md": "hello world", "docs/ARCHITECTURE.md": "arch"}})
	if res.OK {
		t.Fatal("expected validation to fail")
	}
}

func TestValidatePhaseArtifact_RejectsMissingEvidence(t *testing.T) {
	input := []byte(`{
"phase":"vision",
"version":"v1",
"summary":"This is a sufficiently long summary for validation.",
"goals":["g"],
"non_goals":[],
"evidence":[{"type":"file","path":"README.md","quote":"hello world","confidence":0.9}],
"open_questions":[],
"quality":{"clarity":1,"completeness":1,"grounding":1,"consistency":1}
}`)
	res := ValidatePhaseArtifact("vision", input, fakeLookup{files: map[string]string{"README.md": "hello world"}})
	if res.OK {
		t.Fatal("expected validation to fail")
	}
}

func TestValidatePhaseArtifact_RejectsMissingQuote(t *testing.T) {
	input := []byte(`{
"phase":"vision",
"version":"v1",
"summary":"This is a sufficiently long summary for validation.",
"goals":["g"],
"non_goals":[],
"evidence":[{"type":"file","path":"README.md","quote":"missing","confidence":0.9},{"type":"doc","path":"docs/ARCHITECTURE.md","quote":"arch","confidence":0.9}],
"open_questions":[],
"quality":{"clarity":1,"completeness":1,"grounding":1,"consistency":1}
}`)
	res := ValidatePhaseArtifact("vision", input, fakeLookup{files: map[string]string{"README.md": "hello world", "docs/ARCHITECTURE.md": "arch"}})
	if res.OK {
		t.Fatal("expected validation to fail")
	}
}

func TestValidatePhaseArtifact_RejectsLowConfidence(t *testing.T) {
	input := []byte(`{
"phase":"vision",
"version":"v1",
"summary":"This is a sufficiently long summary for validation.",
"goals":["g"],
"non_goals":[],
"evidence":[{"type":"file","path":"README.md","quote":"hello world","confidence":0.1},{"type":"doc","path":"docs/ARCHITECTURE.md","quote":"arch","confidence":0.9}],
"open_questions":[],
"quality":{"clarity":1,"completeness":1,"grounding":1,"consistency":1}
}`)
	res := ValidatePhaseArtifact("vision", input, fakeLookup{files: map[string]string{"README.md": "hello world", "docs/ARCHITECTURE.md": "arch"}})
	if res.OK {
		t.Fatal("expected validation to fail")
	}
}

func TestValidatePhaseArtifact_QualityRequiresOpenQuestions(t *testing.T) {
	input := []byte(`{
"phase":"vision",
"version":"v1",
"summary":"This is a sufficiently long summary for validation.",
"goals":["g"],
"non_goals":[],
"evidence":[{"type":"file","path":"README.md","quote":"hello world","confidence":0.9},{"type":"doc","path":"docs/ARCHITECTURE.md","quote":"arch","confidence":0.9}],
"open_questions":[],
"quality":{"clarity":0.1,"completeness":1,"grounding":1,"consistency":1}
}`)
	res := ValidatePhaseArtifact("vision", input, fakeLookup{files: map[string]string{"README.md": "hello world", "docs/ARCHITECTURE.md": "arch"}})
	if res.OK {
		t.Fatal("expected validation to fail")
	}
}

func TestValidateSynthesisArtifact_RejectsLowAlignment(t *testing.T) {
	input := []byte(`{
"version":"v1",
"inputs":["vision@v1"],
"consistency_notes":[],
"updates_suggested":[],
"quality":{"cross_phase_alignment":0.1}
}`)
	res := ValidateSynthesisArtifact(input)
	if res.OK {
		t.Fatal("expected validation to fail")
	}
}

func TestSchemaRegistry(t *testing.T) {
	data, ok := SchemaFor("vision")
	if !ok {
		t.Fatal("expected vision schema")
	}
	if len(data) == 0 {
		t.Fatal("expected schema data")
	}
	if synth := SynthesisSchema(); len(synth) == 0 {
		t.Fatal("expected synthesis schema data")
	}
}

func TestValidateLegacyScanResult_ReportsValidationErrors(t *testing.T) {
	res := ValidateLegacyScanResult(&ScanResult{
		ProjectName:  "Test",
		Description:  "Desc",
		Vision:       "vision text",
		Users:        "users text",
		Problem:      "problem text",
		Platform:     "CLI",
		Language:     "Go",
		Requirements: []string{"req1"},
	}, map[string]string{
		"README.md": "only one file for evidence",
	})

	if len(res) == 0 {
		t.Fatal("expected validation errors")
	}
}

func TestValidatePhaseArtifact_QuoteMatchesNormalizedWhitespace(t *testing.T) {
	input := []byte(`{
"phase":"vision",
"version":"v1",
"summary":"This is a sufficiently long summary for validation.",
"goals":["g"],
"non_goals":[],
"evidence":[
  {"type":"file","path":"README.md","quote":"Autarch is a platform for a suite","confidence":0.9},
  {"type":"doc","path":"docs/ARCHITECTURE.md","quote":"Architecture docs","confidence":0.9}
],
"open_questions":[],
"quality":{"clarity":1,"completeness":1,"grounding":1,"consistency":1}
}`)
	lookup := fileEvidenceLookup{files: map[string]string{
		"README.md":            "Autarch is a platform\nfor a suite",
		"docs/ARCHITECTURE.md": "Architecture docs",
	}}
	res := ValidatePhaseArtifact("vision", input, lookup)
	if !res.OK {
		t.Fatalf("expected validation to pass, got errors: %+v", res.Errors)
	}
}

func TestValidatePhaseArtifact_QuoteMatchesIgnoringPunctuation(t *testing.T) {
	input := []byte(`{
"phase":"vision",
"version":"v1",
"summary":"This is a sufficiently long summary for validation.",
"goals":["g"],
"non_goals":[],
"evidence":[
  {"type":"file","path":"README.md","quote":"Autarch platform for agents","confidence":0.9},
  {"type":"doc","path":"docs/ARCHITECTURE.md","quote":"Architecture docs","confidence":0.9}
],
"open_questions":[],
"quality":{"clarity":1,"completeness":1,"grounding":1,"consistency":1}
}`)
	lookup := fileEvidenceLookup{files: map[string]string{
		"README.md":            "Autarch—platform for agents.",
		"docs/ARCHITECTURE.md": "Architecture docs",
	}}
	res := ValidatePhaseArtifact("vision", input, lookup)
	if !res.OK {
		t.Fatalf("expected validation to pass, got errors: %+v", res.Errors)
	}
}

func TestValidatePhaseArtifact_QuoteMissingIncludesPath(t *testing.T) {
	input := []byte(`{
"phase":"vision",
"version":"v1",
"summary":"This is a sufficiently long summary for validation.",
"goals":["g"],
"non_goals":[],
"evidence":[
  {"type":"file","path":"README.md","quote":"missing quote","confidence":0.9},
  {"type":"doc","path":"docs/ARCHITECTURE.md","quote":"Architecture docs","confidence":0.9}
],
"open_questions":[],
"quality":{"clarity":1,"completeness":1,"grounding":1,"consistency":1}
}`)
	lookup := fileEvidenceLookup{files: map[string]string{
		"README.md":            "Autarch platform for agents",
		"docs/ARCHITECTURE.md": "Architecture docs",
	}}
	res := ValidatePhaseArtifact("vision", input, lookup)
	if res.OK {
		t.Fatal("expected validation to fail")
	}
	found := false
	for _, err := range res.Errors {
		if err.Code == "evidence_quote_missing" && strings.Contains(err.Message, "README.md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing quote error to include path, got: %+v", res.Errors)
	}
}
