package agent

import (
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
}

// ScanProgress reports progress during codebase scanning.
type ScanProgress struct {
	Step             string            // Current step name
	Details          string            // What's happening
	Files            []string          // Files found/being analyzed
	AgentLine        string            // Live output line from agent (if streaming)
	ValidationErrors []ValidationError // Validation errors on completion
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
	result.ValidationErrors = ValidateLegacyScanResult(result, files)

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
  "vision": "What the project aims to achieve",
  "users": "Who the primary users are",
  "problem": "What problem it solves",
  "platform": "Web|CLI|Desktop|Mobile|API/Backend",
  "language": "Go|TypeScript|Python|Rust|Other",
  "requirements": ["Requirement 1", "Requirement 2", "Requirement 3"]
}

If you cannot determine a field, use a reasonable guess based on the context.
For "platform" and "language", choose the most appropriate option from the list.
List 3-7 key requirements/features based on the documentation.

Generate the JSON now:`)

	return sb.String()
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

	var result ScanResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w\nContent: %s", err, content[:min(500, len(content))])
	}

	return &result, nil
}
