package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	minEvidenceItems       = 2
	minEvidenceConfidence  = 0.35
	minQualityClarity      = 0.55
	minQualityCompleteness = 0.55
	minQualityGrounding    = 0.60
	minQualityConsistency  = 0.50
	minCrossPhaseAlignment = 0.60
)

func ValidatePhaseArtifact(phase string, raw []byte, lookup EvidenceLookup) ValidationResult {
	res := ValidationResult{}

	switch phase {
	case "vision":
		_, res = decodeStrict[VisionArtifact](raw)
	case "problem":
		_, res = decodeStrict[ProblemArtifact](raw)
	case "users":
		_, res = decodeStrict[UsersArtifact](raw)
	case "features":
		_, res = decodeStrict[FeaturesArtifact](raw)
	case "requirements":
		_, res = decodeStrict[RequirementsArtifact](raw)
	case "scope":
		_, res = decodeStrict[ScopeArtifact](raw)
	case "cujs":
		_, res = decodeStrict[CUJArtifact](raw)
	case "acceptance":
		_, res = decodeStrict[AcceptanceArtifact](raw)
	default:
		res.Errors = append(res.Errors, ValidationError{Code: "invalid_phase", Field: "phase", Message: "unknown phase"})
	}

	if len(res.Errors) > 0 {
		return finalizeResult(res)
	}

	var base ScanArtifactBase
	if err := decodeBase(raw, &base); err != nil {
		res.Errors = append(res.Errors, ValidationError{Code: "schema_invalid", Message: err.Error()})
		return finalizeResult(res)
	}

	validateEvidence(&res, base.Evidence, lookup)
	validateQuality(&res, base.Quality)
	if hasQualityFailures(res.Errors) && len(base.OpenQuestions) == 0 {
		res.Errors = append(res.Errors, ValidationError{Code: "missing_open_questions", Field: "open_questions", Message: "Add open questions when quality is below threshold"})
	}

	return finalizeResult(res)
}

func ValidateSynthesisArtifact(raw []byte) ValidationResult {
	res := ValidationResult{}
	artifact, decRes := decodeStrict[SynthesisArtifact](raw)
	res = decRes
	if len(res.Errors) > 0 {
		return finalizeResult(res)
	}

	if artifact.Quality.CrossPhaseAlignment < minCrossPhaseAlignment {
		res.Errors = append(res.Errors, ValidationError{Code: "quality_low", Field: "quality.cross_phase_alignment", Message: "Cross-phase alignment below threshold"})
	}

	return finalizeResult(res)
}

func decodeStrict[T any](raw []byte) (T, ValidationResult) {
	var out T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, ValidationResult{Errors: []ValidationError{{Code: "schema_invalid", Message: err.Error()}}}
	}
	if dec.More() {
		return out, ValidationResult{Errors: []ValidationError{{Code: "schema_invalid", Message: "extra data after JSON"}}}
	}
	return out, ValidationResult{}
}

func decodeBase(raw []byte, base *ScanArtifactBase) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(base); err != nil {
		return err
	}
	return nil
}

func validateEvidence(res *ValidationResult, evidence []EvidenceItem, lookup EvidenceLookup) {
	if len(evidence) < minEvidenceItems {
		res.Errors = append(res.Errors, ValidationError{Code: "missing_evidence", Field: "evidence", Message: "At least 2 evidence items required"})
		return
	}
	for i, ev := range evidence {
		if ev.Path == "" || ev.Quote == "" {
			res.Errors = append(res.Errors, ValidationError{Code: "evidence_invalid", Field: fmt.Sprintf("evidence[%d]", i), Message: "Evidence path and quote required"})
			continue
		}
		if ev.Confidence < minEvidenceConfidence {
			res.Errors = append(res.Errors, ValidationError{Code: "evidence_low_confidence", Field: fmt.Sprintf("evidence[%d].confidence", i), Message: "Evidence confidence too low"})
		}
		if lookup == nil {
			continue
		}
		if !lookup.Exists(ev.Path) {
			res.Errors = append(res.Errors, ValidationError{Code: "evidence_missing_file", Field: fmt.Sprintf("evidence[%d].path", i), Message: "Evidence file not found"})
			continue
		}
		if !lookup.ContainsQuote(ev.Path, ev.Quote) {
			res.Errors = append(res.Errors, ValidationError{Code: "evidence_quote_missing", Field: fmt.Sprintf("evidence[%d].quote", i), Message: "Evidence quote not found in file"})
		}
	}
}

func validateQuality(res *ValidationResult, quality QualityScores) {
	checkQuality(res, "quality.clarity", quality.Clarity, minQualityClarity)
	checkQuality(res, "quality.completeness", quality.Completeness, minQualityCompleteness)
	checkQuality(res, "quality.grounding", quality.Grounding, minQualityGrounding)
	checkQuality(res, "quality.consistency", quality.Consistency, minQualityConsistency)
}

func checkQuality(res *ValidationResult, field string, value, min float64) {
	if value < min {
		res.Errors = append(res.Errors, ValidationError{Code: "quality_low", Field: field, Message: "Quality below threshold"})
	}
}

func hasQualityFailures(errors []ValidationError) bool {
	for _, err := range errors {
		if err.Code == "quality_low" {
			return true
		}
	}
	return false
}

func finalizeResult(res ValidationResult) ValidationResult {
	res.OK = len(res.Errors) == 0
	return res
}
