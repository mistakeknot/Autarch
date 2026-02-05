package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ScanResult contains extracted information from a codebase scan.
type ScanResult struct {
	ProjectName      string            `json:"project_name"`
	Description      string            `json:"description"`
	Vision           string            `json:"vision"`
	Users            string            `json:"users"`
	Problem          string            `json:"problem"`
	Platform         string            `json:"platform"`
	Language         string            `json:"language"`
	Requirements     []string          `json:"requirements"`
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
	PhaseArtifacts   *PhaseArtifacts   `json:"phase_artifacts,omitempty"`
}

// ScanProgress reports progress during codebase scanning.
type ScanProgress struct {
	Step              string            // Current step name
	Details           string            // What's happening
	Files             []string          // Files found/being analyzed
	AgentLine         string            // Live output line from agent (if streaming)
	ValidationErrors  []ValidationError // Validation errors on completion
	PhaseArtifacts    *PhaseArtifacts   // Structured scan artifacts
	ExplorationResult    map[string]any // Raw Claude Code exploration output for phase transitions
	ExplorationSessionID string         // Claude Code session ID for reuse in later phases
}

// ScanProgressFunc is called to report scan progress.
type ScanProgressFunc func(ScanProgress)

// ScanCodebase uses the coding agent to analyze an existing codebase and extract project info.
func ScanCodebase(ctx context.Context, agent *Agent, path string) (*ScanResult, error) {
	return ScanCodebaseWithProgress(ctx, agent, path, nil)
}

// ScanCodebaseWithProgress is like ScanCodebase but reports progress.
func ScanCodebaseWithProgress(ctx context.Context, agent *Agent, path string, progress ScanProgressFunc) (*ScanResult, error) {
	report := func(step, details string, files []string) {
		if progress != nil {
			progress(ScanProgress{Step: step, Details: details, Files: files})
		}
	}

	// Step 0: Deterministic tech stack detection (fast, no LLM)
	techEvidence := detectTechStack(path)
	if len(techEvidence) > 0 {
		// Report tech stack immediately for progress UX
		var techSummary []string
		for _, ev := range techEvidence {
			techSummary = append(techSummary, ev.Quote)
		}
		report("Tech stack", strings.Join(techSummary, " + "), nil)
	}

	// Step 1: Gather context from the codebase
	report("Scanning", "Looking for project files...", nil)
	files, err := gatherRelevantFiles(path)
	if err != nil {
		return nil, fmt.Errorf("failed to gather files: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no relevant files found in %s", path)
	}

	// Report which files were found
	var fileNames []string
	for name := range files {
		fileNames = append(fileNames, name)
	}
	report("Found files", fmt.Sprintf("Found %d files to analyze", len(files)), fileNames)

	// Step 2: Build the prompt
	report("Preparing", "Building analysis prompt...", nil)
	prompt := buildScanPrompt(path, files)

	// Step 3: Call the agent with streaming output
	report("Analyzing", fmt.Sprintf("Asking %s to analyze codebase...", agent.Type), nil)

	// Set a reasonable timeout
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Stream agent output to progress callback
	outputCallback := func(line string) {
		if progress != nil {
			progress(ScanProgress{
				Step:      "Analyzing",
				Details:   fmt.Sprintf("%s is working...", agent.Type),
				AgentLine: line,
			})
		}
	}

	resp, err := agent.GenerateWithOutput(ctx, GenerateRequest{
		Prompt: prompt,
	}, outputCallback)
	if err != nil {
		return nil, fmt.Errorf("agent generation failed: %w", err)
	}

	// Step 4: Parse the response
	report("Parsing", "Extracting project information...", nil)
	result, err := parseScanResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scan response: %w", err)
	}

	// Step 5: Merge deterministic tech stack evidence into Vision phase
	if result.PhaseArtifacts != nil && result.PhaseArtifacts.Vision != nil && len(techEvidence) > 0 {
		// Prepend deterministic evidence (high confidence) before LLM evidence
		result.PhaseArtifacts.Vision.Evidence = append(techEvidence, result.PhaseArtifacts.Vision.Evidence...)
	}

	if result.PhaseArtifacts != nil {
		result.ValidationErrors = ValidateStructuredScanArtifacts(result, files)
	} else {
		result.ValidationErrors = ValidateLegacyScanResult(result, files)
	}

	report("Complete", fmt.Sprintf("Found: %s", result.ProjectName), nil)
	return result, nil
}

// gatherRelevantFiles finds README, docs, and config files for analysis.
func gatherRelevantFiles(path string) (map[string]string, error) {
	files := make(map[string]string)

	// Priority files to look for
	priorities := []string{
		"README.md",
		"README",
		"readme.md",
		"CLAUDE.md",
		"AGENTS.md",
		"docs/README.md",
		"docs/index.md",
		"PRD.md",
		"SPEC.md",
		"package.json",
		"go.mod",
		"Cargo.toml",
		"pyproject.toml",
		"requirements.txt",
	}

	for _, f := range priorities {
		fullPath := filepath.Join(path, f)
		content, err := os.ReadFile(fullPath)
		if err == nil {
			// Truncate long files
			contentStr := string(content)
			if len(contentStr) > 4000 {
				contentStr = contentStr[:4000] + "\n... (truncated)"
			}
			files[f] = contentStr
		}
	}

	// If no README found, look in docs/ directory
	if len(files) == 0 {
		docsPath := filepath.Join(path, "docs")
		entries, err := os.ReadDir(docsPath)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if strings.HasSuffix(name, ".md") {
					fullPath := filepath.Join(docsPath, name)
					content, err := os.ReadFile(fullPath)
					if err == nil {
						contentStr := string(content)
						if len(contentStr) > 2000 {
							contentStr = contentStr[:2000] + "\n... (truncated)"
						}
						files["docs/"+name] = contentStr
					}
					// Limit to 3 doc files
					if len(files) >= 3 {
						break
					}
				}
			}
		}
	}

	return files, nil
}

func buildScanPrompt(path string, files map[string]string) string {
	var sb strings.Builder

	sb.WriteString(`You are analyzing an existing codebase to understand the project and extract key information.

CODEBASE PATH: `)
	sb.WriteString(path)
	sb.WriteString("\n\nFILES FOUND:\n")

	for name, content := range files {
		sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", name, content))
	}

	sb.WriteString(`

Based on these files, extract information about this project.

Output ONLY valid JSON in this exact format (no markdown, no explanation):
{
  "project_name": "Name of the project",
  "description": "One-sentence description of what this project does",
  "platform": "Web|CLI|Desktop|Mobile|API/Backend",
  "language": "Go|TypeScript|Python|Rust|Other",
  "requirements": ["Requirement 1", "Requirement 2", "Requirement 3"],
  "artifacts": {
    "vision": {
      "phase": "vision",
      "version": "v1",
      "summary": "Vision summary (>= 20 chars)",
      "goals": ["Goal 1"],
      "non_goals": [],
      "evidence": [
        {"type":"readme","path":"README.md","quote":"Copy the EXACT sentence from README that describes project purpose","confidence":0.9},
        {"type":"doc","path":"CLAUDE.md","quote":"Copy VERBATIM text that supports the vision","confidence":0.7}
      ],
      "open_questions": [],
      "quality": {"clarity":0.7,"completeness":0.7,"grounding":0.7,"consistency":0.7}
    },
    "problem": {
      "phase": "problem",
      "version": "v1",
      "summary": "Problem summary (>= 20 chars)",
      "pain_points": ["Pain 1"],
      "impact": "Impact text",
      "evidence": [
        {"type":"readme","path":"README.md","quote":"Copy EXACT sentence describing the problem or pain point","confidence":0.9},
        {"type":"doc","path":"AGENTS.md","quote":"Copy VERBATIM text about challenges or issues","confidence":0.7}
      ],
      "open_questions": [],
      "quality": {"clarity":0.7,"completeness":0.7,"grounding":0.7,"consistency":0.7}
    },
    "users": {
      "phase": "users",
      "version": "v1",
      "personas": [
        {"name":"Primary user","needs":["Need 1"],"context":"Context text"}
      ],
      "evidence": [
        {"type":"readme","path":"README.md","quote":"Copy EXACT sentence describing who uses this","confidence":0.9},
        {"type":"doc","path":"CLAUDE.md","quote":"Copy VERBATIM text about target users or audience","confidence":0.7}
      ],
      "open_questions": [],
      "quality": {"clarity":0.7,"completeness":0.7,"grounding":0.7,"consistency":0.7}
    }
  }
}

CRITICAL EVIDENCE INSTRUCTIONS:
- For each artifact, find VERBATIM QUOTES from the provided files
- Copy exact sentences/phrases - do NOT paraphrase or summarize
- Include the FULL sentence that provides evidence
- Vision evidence: sentences describing what the project does or its purpose
- Problem evidence: sentences describing pain points, challenges, or what the project solves
- Users evidence: sentences describing who uses the project or target audience
- Set confidence based on how directly the quote supports the claim:
  - 0.9: Quote explicitly states the claim
  - 0.7: Quote strongly implies the claim
  - 0.5: Quote is tangentially related

If you cannot determine a field, use a reasonable guess based on the context.
For "platform" and "language", choose the most appropriate option from the list.
List 3-7 key requirements/features based on the documentation.
Every artifact must include at least 2 evidence items with VERBATIM QUOTES from the provided files.

Generate the JSON now:`)

	return sb.String()
}

// detectTechStack extracts tech stack evidence from manifest files.
// Returns []EvidenceItem suitable for injection into PhaseArtifacts.Vision.Evidence.
func detectTechStack(root string) []EvidenceItem {
	var evidence []EvidenceItem

	// Go: go.mod
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		lang := "Go"
		var frameworks []string
		var version string

		// Extract Go version (simple line parsing)
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "go ") {
				version = strings.TrimPrefix(line, "go ")
				break
			}
		}

		// Detect frameworks using simple bytes.Contains
		if bytes.Contains(data, []byte("bubbletea")) {
			frameworks = append(frameworks, "Bubble Tea")
		}
		if bytes.Contains(data, []byte("gorm.io")) {
			frameworks = append(frameworks, "GORM")
		}
		if bytes.Contains(data, []byte("cobra")) {
			frameworks = append(frameworks, "Cobra")
		}
		if bytes.Contains(data, []byte("gin-gonic")) {
			frameworks = append(frameworks, "Gin")
		}
		if bytes.Contains(data, []byte("echo")) && bytes.Contains(data, []byte("labstack")) {
			frameworks = append(frameworks, "Echo")
		}
		if bytes.Contains(data, []byte("fiber")) {
			frameworks = append(frameworks, "Fiber")
		}

		quote := lang
		if version != "" {
			quote = fmt.Sprintf("%s %s", lang, version)
		}
		if len(frameworks) > 0 {
			quote = fmt.Sprintf("%s with %s", quote, strings.Join(frameworks, ", "))
		}

		evidence = append(evidence, EvidenceItem{
			Type:       "tech_stack",
			Path:       "go.mod",
			Quote:      quote,
			Confidence: 1.0, // deterministic
		})
	}

	// JavaScript/TypeScript: package.json
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		lang := "JavaScript"
		var frameworks []string

		// Detect TypeScript
		if bytes.Contains(data, []byte(`"typescript"`)) {
			lang = "TypeScript"
		}

		// Detect frameworks
		if bytes.Contains(data, []byte(`"next"`)) {
			frameworks = append(frameworks, "Next.js")
		}
		if bytes.Contains(data, []byte(`"react"`)) && !bytes.Contains(data, []byte(`"next"`)) {
			frameworks = append(frameworks, "React")
		}
		if bytes.Contains(data, []byte(`"vue"`)) {
			frameworks = append(frameworks, "Vue")
		}
		if bytes.Contains(data, []byte(`"svelte"`)) {
			frameworks = append(frameworks, "Svelte")
		}
		if bytes.Contains(data, []byte(`"express"`)) {
			frameworks = append(frameworks, "Express")
		}
		if bytes.Contains(data, []byte(`"fastify"`)) {
			frameworks = append(frameworks, "Fastify")
		}
		if bytes.Contains(data, []byte(`"tailwindcss"`)) {
			frameworks = append(frameworks, "Tailwind")
		}

		quote := lang
		if len(frameworks) > 0 {
			quote = fmt.Sprintf("%s with %s", lang, strings.Join(frameworks, ", "))
		}

		evidence = append(evidence, EvidenceItem{
			Type:       "tech_stack",
			Path:       "package.json",
			Quote:      quote,
			Confidence: 1.0,
		})
	}

	// Rust: Cargo.toml
	if data, err := os.ReadFile(filepath.Join(root, "Cargo.toml")); err == nil {
		lang := "Rust"
		var frameworks []string

		if bytes.Contains(data, []byte("tokio")) {
			frameworks = append(frameworks, "Tokio")
		}
		if bytes.Contains(data, []byte("actix")) {
			frameworks = append(frameworks, "Actix")
		}
		if bytes.Contains(data, []byte("axum")) {
			frameworks = append(frameworks, "Axum")
		}
		if bytes.Contains(data, []byte("tauri")) {
			frameworks = append(frameworks, "Tauri")
		}
		if bytes.Contains(data, []byte("serde")) {
			frameworks = append(frameworks, "Serde")
		}

		quote := lang
		if len(frameworks) > 0 {
			quote = fmt.Sprintf("%s with %s", lang, strings.Join(frameworks, ", "))
		}

		evidence = append(evidence, EvidenceItem{
			Type:       "tech_stack",
			Path:       "Cargo.toml",
			Quote:      quote,
			Confidence: 1.0,
		})
	}

	// Python: pyproject.toml or requirements.txt
	if data, err := os.ReadFile(filepath.Join(root, "pyproject.toml")); err == nil {
		lang := "Python"
		var frameworks []string

		if bytes.Contains(data, []byte("django")) {
			frameworks = append(frameworks, "Django")
		}
		if bytes.Contains(data, []byte("fastapi")) {
			frameworks = append(frameworks, "FastAPI")
		}
		if bytes.Contains(data, []byte("flask")) {
			frameworks = append(frameworks, "Flask")
		}
		if bytes.Contains(data, []byte("pytorch")) || bytes.Contains(data, []byte("torch")) {
			frameworks = append(frameworks, "PyTorch")
		}

		quote := lang
		if len(frameworks) > 0 {
			quote = fmt.Sprintf("%s with %s", lang, strings.Join(frameworks, ", "))
		}

		evidence = append(evidence, EvidenceItem{
			Type:       "tech_stack",
			Path:       "pyproject.toml",
			Quote:      quote,
			Confidence: 1.0,
		})
	} else if data, err := os.ReadFile(filepath.Join(root, "requirements.txt")); err == nil {
		lang := "Python"
		var frameworks []string

		if bytes.Contains(data, []byte("django")) || bytes.Contains(data, []byte("Django")) {
			frameworks = append(frameworks, "Django")
		}
		if bytes.Contains(data, []byte("fastapi")) {
			frameworks = append(frameworks, "FastAPI")
		}
		if bytes.Contains(data, []byte("flask")) || bytes.Contains(data, []byte("Flask")) {
			frameworks = append(frameworks, "Flask")
		}

		quote := lang
		if len(frameworks) > 0 {
			quote = fmt.Sprintf("%s with %s", lang, strings.Join(frameworks, ", "))
		}

		evidence = append(evidence, EvidenceItem{
			Type:       "tech_stack",
			Path:       "requirements.txt",
			Quote:      quote,
			Confidence: 1.0,
		})
	}

	return evidence
}

func parseScanResponse(content string) (*ScanResult, error) {
	// Clean up the response
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Try to find JSON in the response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	var structured structuredScanResponse
	if err := json.Unmarshal([]byte(content), &structured); err == nil && (structured.Artifacts != nil || structured.ProjectName != "" || structured.Description != "") {
		result := &ScanResult{
			ProjectName:      structured.ProjectName,
			Description:      structured.Description,
			Vision:           structured.Vision,
			Users:            structured.Users,
			Problem:          structured.Problem,
			Platform:         structured.Platform,
			Language:         structured.Language,
			Requirements:     structured.Requirements,
			PhaseArtifacts:   structured.Artifacts,
			ValidationErrors: nil,
		}
		applyStructuredDefaults(result)
		return result, nil
	}

	var result ScanResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w\nContent: %s", err, content[:min(500, len(content))])
	}

	return &result, nil
}
