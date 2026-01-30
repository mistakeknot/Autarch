package agent

import (
	"encoding/json"
	"sort"
	"strings"
)

const legacyArtifactVersion = "v1"

func ValidateLegacyScanResult(result *ScanResult, files map[string]string) []ValidationError {
	if result == nil {
		return nil
	}
	if result.PhaseArtifacts != nil {
		return nil
	}

	evidence := buildEvidenceFromFiles(files)
	lookup := fileEvidenceLookup{files: files}
	quality := QualityScores{Clarity: 0.6, Completeness: 0.6, Grounding: 0.6, Consistency: 0.6}

	errors := []ValidationError{}

	vision := VisionArtifact{
		ScanArtifactBase: ScanArtifactBase{
			Phase:         "vision",
			Version:       legacyArtifactVersion,
			Evidence:      evidence,
			OpenQuestions: []string{},
			Quality:       quality,
		},
		Summary:  firstNonEmpty(result.Vision, result.Description),
		Goals:    legacyGoals(result),
		NonGoals: []string{},
	}
	errors = append(errors, validatePhaseArtifact("vision", vision, lookup)...)

	problemSummary := firstNonEmpty(result.Problem, result.Description)
	problem := ProblemArtifact{
		ScanArtifactBase: ScanArtifactBase{
			Phase:         "problem",
			Version:       legacyArtifactVersion,
			Evidence:      evidence,
			OpenQuestions: []string{},
			Quality:       quality,
		},
		Summary:    problemSummary,
		PainPoints: legacyPainPoints(result, problemSummary),
		Impact:     problemSummary,
	}
	errors = append(errors, validatePhaseArtifact("problem", problem, lookup)...)

	users := UsersArtifact{
		ScanArtifactBase: ScanArtifactBase{
			Phase:         "users",
			Version:       legacyArtifactVersion,
			Evidence:      evidence,
			OpenQuestions: []string{},
			Quality:       quality,
		},
		Personas: legacyPersonas(result, problemSummary),
	}
	errors = append(errors, validatePhaseArtifact("users", users, lookup)...)

	return errors
}

func validatePhaseArtifact(phase string, artifact any, lookup EvidenceLookup) []ValidationError {
	data, err := json.Marshal(artifact)
	if err != nil {
		return []ValidationError{{Code: "marshal_failed", Message: err.Error()}}
	}
	res := ValidatePhaseArtifact(phase, data, lookup)
	return res.Errors
}

type fileEvidenceLookup struct {
	files map[string]string
}

func (l fileEvidenceLookup) Exists(path string) bool {
	_, ok := l.files[path]
	return ok
}

func (l fileEvidenceLookup) ContainsQuote(path, quote string) bool {
	content, ok := l.files[path]
	if !ok {
		return false
	}
	if strings.Contains(content, quote) {
		return true
	}
	normalizedContent := normalizeEvidenceText(content)
	normalizedQuote := normalizeEvidenceText(quote)
	if normalizedQuote == "" {
		return false
	}
	return strings.Contains(normalizedContent, normalizedQuote)
}

func buildEvidenceFromFiles(files map[string]string) []EvidenceItem {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]EvidenceItem, 0, 2)
	for _, key := range keys {
		if len(items) >= 2 {
			break
		}
		content := files[key]
		quote := firstEvidenceQuote(content)
		if len(quote) < 5 {
			continue
		}
		itemType := "file"
		if strings.HasPrefix(key, "docs/") || strings.HasSuffix(key, ".md") {
			itemType = "doc"
		}
		items = append(items, EvidenceItem{
			Type:       itemType,
			Path:       key,
			Quote:      quote,
			Confidence: 0.7,
		})
	}
	return items
}

func firstEvidenceQuote(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncateQuote(line, 300)
		}
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	return truncateQuote(trimmed, 300)
}

func truncateQuote(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func normalizeEvidenceText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	builder := strings.Builder{}
	builder.Grow(len(trimmed))
	space := false
	for _, r := range strings.ToLower(trimmed) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if space && builder.Len() > 0 {
				builder.WriteByte(' ')
				space = false
			}
			builder.WriteRune(r)
			continue
		}
		space = true
	}
	return strings.TrimSpace(builder.String())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func legacyGoals(result *ScanResult) []string {
	if result == nil {
		return nil
	}
	if len(result.Requirements) > 0 {
		return result.Requirements
	}
	if result.Vision != "" {
		return []string{result.Vision}
	}
	if result.Description != "" {
		return []string{result.Description}
	}
	return nil
}

func legacyPainPoints(result *ScanResult, fallback string) []string {
	if result == nil {
		return nil
	}
	if result.Problem != "" {
		return []string{result.Problem}
	}
	if fallback != "" {
		return []string{fallback}
	}
	return nil
}

func legacyPersonas(result *ScanResult, fallback string) []Persona {
	if result == nil {
		return nil
	}
	context := firstNonEmpty(result.Users, fallback, result.Description)
	if context == "" {
		return nil
	}
	needs := []string{}
	if result.Problem != "" {
		needs = append(needs, result.Problem)
	}
	if len(needs) == 0 {
		needs = append(needs, "Needs a clear outcome")
	}
	return []Persona{{
		Name:    "Primary user",
		Needs:   needs,
		Context: context,
	}}
}
