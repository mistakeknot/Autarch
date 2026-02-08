package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/internal/coldwine/agent"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/explore"
	"github.com/mistakeknot/autarch/internal/coldwine/initflow"
	"github.com/mistakeknot/autarch/internal/coldwine/project"
	gurgehProject "github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/internal/pollard/insights"
	"github.com/mistakeknot/autarch/pkg/agenttargets"
	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

type initOptions struct {
	Agent       string
	Existing    string
	ExistingSet bool
	Depth       int
	DepthSet    bool
	UseTUI      bool
}

type lineReader struct {
	scanner *bufio.Scanner
}

func newLineReader(in io.Reader) *lineReader {
	return &lineReader{scanner: bufio.NewScanner(in)}
}

func (r *lineReader) NextLine() (string, bool) {
	if r == nil || r.scanner == nil {
		return "", false
	}
	if !r.scanner.Scan() {
		return "", false
	}
	return r.scanner.Text(), true
}

var initGeneratorFactory = func(root, agentName string, out io.Writer) initflow.Generator {
	return &agentGenerator{root: root, agentName: agentName, out: out}
}

func runInit(cmdOut io.Writer, in io.Reader, opts initOptions) error {
	if err := project.Init("."); err != nil {
		return err
	}
	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}

	reader := newLineReader(in)
	depth := opts.Depth
	if depth <= 0 && !opts.DepthSet {
		depth = promptDepth(reader, cmdOut, 2)
	}
	if depth <= 0 {
		depth = 2
	}

	planDir := filepath.Join(root, ".tandemonium", "plan")
	_, err = explore.Run(root, planDir, explore.Options{
		Depth: depth,
		EmitProgress: func(msg string) {
			fmt.Fprintln(cmdOut, msg)
		},
	})
	if err != nil {
		return err
	}

	generator := initGeneratorFactory(root, opts.Agent, cmdOut)
	result, err := initflow.GenerateEpics(generator, initflow.Input{
		Summary:         loadSummary(planDir),
		Depth:           depth,
		Repo:            root,
		ResearchContext: loadPollardContext(root),
		SpecContext:     loadGurgehContext(root),
	})
	if err != nil {
		fmt.Fprintf(cmdOut, "⚠  Agent generation failed: %v\n", err)
		if !promptConfirm(reader, cmdOut, "Use fallback starter backlog? [Y/n]", true) {
			return err
		}
		result = initflow.Result{Epics: initflow.FallbackEpics()}
	}

	if !promptConfirm(reader, cmdOut, "Write epic specs now? [Y/n]", true) {
		return nil
	}

	specsDir := project.SpecsDir(root)
	existingMode := opts.Existing
	if existingMode == "" {
		existingMode = "skip"
	}
	if !opts.ExistingSet && hasExistingEpics(specsDir) {
		existingMode = promptExistingMode(reader, cmdOut)
	}

	switch strings.ToLower(existingMode) {
	case "overwrite":
		return epics.WriteEpics(specsDir, result.Epics, epics.WriteOptions{Existing: epics.ExistingOverwrite})
	case "prompt":
		return writeEpicsWithPrompt(reader, cmdOut, specsDir, result.Epics)
	default:
		return epics.WriteEpics(specsDir, result.Epics, epics.WriteOptions{Existing: epics.ExistingSkip})
	}
}

func loadSummary(planDir string) string {
	path := filepath.Join(planDir, "exploration.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func hasExistingEpics(specsDir string) bool {
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "EPIC-") && strings.HasSuffix(entry.Name(), ".yaml") {
			return true
		}
	}
	return false
}

func writeEpicsWithPrompt(reader *lineReader, out io.Writer, specsDir string, list []epics.Epic) error {
	for _, epic := range list {
		path := filepath.Join(specsDir, epic.ID+".yaml")
		if _, err := os.Stat(path); err == nil {
			if !promptConfirm(reader, out, "Overwrite "+epic.ID+"? [y/N]", false) {
				continue
			}
		}
		if err := epics.WriteEpics(specsDir, []epics.Epic{epic}, epics.WriteOptions{Existing: epics.ExistingOverwrite}); err != nil {
			return err
		}
	}
	return nil
}

func promptDepth(reader *lineReader, out io.Writer, defaultDepth int) int {
	fmt.Fprintln(out, "Exploration depth (1-3)? [2]")
	line, ok := reader.NextLine()
	if !ok {
		return defaultDepth
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultDepth
	}
	switch line {
	case "1":
		return 1
	case "2":
		return 2
	case "3":
		return 3
	default:
		return defaultDepth
	}
}

func promptExistingMode(reader *lineReader, out io.Writer) string {
	fmt.Fprintln(out, "Existing epics found. Choose [s]kip/[o]verwrite/[p]rompt:")
	line, ok := reader.NextLine()
	if !ok {
		return "skip"
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "o", "overwrite":
		return "overwrite"
	case "p", "prompt":
		return "prompt"
	default:
		return "skip"
	}
}

func promptConfirm(reader *lineReader, out io.Writer, message string, defaultYes bool) bool {
	fmt.Fprintln(out, message)
	line, ok := reader.NextLine()
	if !ok {
		return defaultYes
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return strings.HasPrefix(line, "y")
}

type agentGenerator struct {
	root      string
	agentName string
	out       io.Writer
	runner    agenttargets.AgentRunner
}

func (g *agentGenerator) Generate(ctx context.Context, input initflow.Input) (initflow.Result, error) {
	target, err := agent.ResolveTarget(g.root, g.agentName)
	if err != nil {
		return initflow.Result{}, err
	}
	if strings.TrimSpace(target.Command) == "" {
		return initflow.Result{}, fmt.Errorf("agent command not configured")
	}
	if _, err := exec.LookPath(target.Command); err != nil {
		return initflow.Result{}, err
	}
	promptPath, err := writeAgentPrompt(g.root, input)
	if err != nil {
		return initflow.Result{}, err
	}
	runner := g.runner
	if runner == nil {
		runner = agenttargets.NewExecAgentRunner()
	}
	output, err := runAgentWithRunner(ctx, runner, target, agenttargets.DefaultSafetyPolicy(), g.root, promptPath)
	if err != nil {
		return initflow.Result{}, err
	}
	epicsList, err := parseAndValidateEpics(output, filepath.Join(g.root, ".tandemonium", "plan"))
	if err != nil {
		return initflow.Result{}, err
	}
	return initflow.Result{Epics: epicsList}, nil
}

func writeAgentPrompt(root string, input initflow.Input) (string, error) {
	planDir := filepath.Join(root, ".tandemonium", "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(planDir, "init-epics.md")
	prompt := buildAgentPrompt(input)
	if err := os.WriteFile(path, []byte(prompt), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func runAgentWithRunner(ctx context.Context, runner agenttargets.AgentRunner, target agenttargets.ResolvedTarget, policy agenttargets.SafetyPolicy, workDir, promptPath string) ([]byte, error) {
	if runner == nil {
		return nil, fmt.Errorf("agent runner not configured")
	}
	handle, err := runner.Run(ctx, target, policy, workDir, promptPath)
	if err != nil {
		return nil, fmt.Errorf("agent run failed: %w", err)
	}
	result, waitErr := handle.Wait()
	if waitErr != nil {
		return nil, fmt.Errorf("agent run failed: %w", waitErr)
	}
	if result.TimedOut {
		return nil, fmt.Errorf("agent run timed out")
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("agent run failed (exit %d): %s", result.ExitCode, string(result.Output))
	}
	return result.Output, nil
}

func buildAgentPrompt(input initflow.Input) string {
	var b strings.Builder
	b.WriteString("# Coldwine Init: Epic + Story Generation\n\n")
	b.WriteString("You are generating epic/story specs for a repo. Read the context below and output YAML only.\n")
	b.WriteString("Allowed status: todo|in_progress|review|blocked|done\n")
	b.WriteString("Allowed priority: p0|p1|p2|p3\n")
	b.WriteString("Use estimates (plural).\n")
	b.WriteString("Output YAML only (no prose).\n\n")

	b.WriteString("Output schema:\n\n")
	b.WriteString("```yaml\n")
	b.WriteString("epics:\n")
	b.WriteString("  - id: EPIC-001\n")
	b.WriteString("    title: Example\n")
	b.WriteString("    summary: Short description\n")
	b.WriteString("    status: todo\n")
	b.WriteString("    priority: p1\n")
	b.WriteString("    acceptance_criteria:\n")
	b.WriteString("      - ...\n")
	b.WriteString("    risks:\n")
	b.WriteString("      - ...\n")
	b.WriteString("    estimates: \"S\"\n")
	b.WriteString("    stories:\n")
	b.WriteString("      - id: EPIC-001-S01\n")
	b.WriteString("        title: Story title\n")
	b.WriteString("        summary: Story summary\n")
	b.WriteString("        status: todo\n")
	b.WriteString("        priority: p1\n")
	b.WriteString("        acceptance_criteria:\n")
	b.WriteString("          - ...\n")
	b.WriteString("        risks:\n")
	b.WriteString("          - ...\n")
	b.WriteString("        estimates: \"S\"\n")
	b.WriteString("```\n\n")

	if input.SpecContext != "" {
		b.WriteString("## Gurgeh Specs (existing PRDs)\n\n")
		b.WriteString(input.SpecContext)
		b.WriteString("\n\n")
	}

	if input.ResearchContext != "" {
		b.WriteString("## Pollard Research Insights\n\n")
		b.WriteString(input.ResearchContext)
		b.WriteString("\n\n")
	}

	b.WriteString("## Exploration Summary\n\n")
	b.WriteString(input.Summary)
	b.WriteString("\n")
	return b.String()
}

func parseAgentEpics(raw []byte) ([]epics.Epic, error) {
	var wrapper struct {
		Epics []epics.Epic `yaml:"epics"`
	}
	if err := yamlsafe.Decode(raw, &wrapper); err == nil && len(wrapper.Epics) > 0 {
		return wrapper.Epics, nil
	}
	var list []epics.Epic
	if err := yamlsafe.Decode(raw, &list); err == nil && len(list) > 0 {
		return list, nil
	}
	if idx := bytes.Index(raw, []byte("epics:")); idx >= 0 {
		trimmed := raw[idx:]
		if err := yamlsafe.Decode(trimmed, &wrapper); err == nil && len(wrapper.Epics) > 0 {
			return wrapper.Epics, nil
		}
	}
	return nil, fmt.Errorf("agent output missing epics")
}

func parseAndValidateEpics(raw []byte, planDir string) ([]epics.Epic, error) {
	list, err := parseAgentEpics(raw)
	if err != nil {
		return nil, err
	}
	errList := epics.Validate(list)
	if len(errList) == 0 {
		return list, nil
	}

	// Attempt auto-fix for fixable errors (bad IDs, missing status/priority)
	if !epics.HasFatalErrors(errList) {
		remaining := epics.AutoFix(list)
		if len(remaining) == 0 {
			return list, nil
		}
	}

	// Write report with guidance
	outPath, errPath, writeErr := epics.WriteValidationReport(planDir, raw, errList)
	if writeErr != nil {
		return nil, writeErr
	}
	return nil, &InitValidationError{
		Errors:  errList,
		OutPath: outPath,
		ErrPath: errPath,
	}
}

// InitValidationError provides structured validation failure information.
type InitValidationError struct {
	Errors  []epics.ValidationError
	OutPath string
	ErrPath string
}

func (e *InitValidationError) Error() string {
	fatal := 0
	fixable := 0
	for _, err := range e.Errors {
		if err.Severity == epics.SeverityFatal {
			fatal++
		} else {
			fixable++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "validation failed: %d error(s) (%d fatal, %d fixable)\n", len(e.Errors), fatal, fixable)
	fmt.Fprintf(&b, "  raw output: %s\n", e.OutPath)
	fmt.Fprintf(&b, "  error log:  %s\n", e.ErrPath)
	b.WriteString("\nErrors:\n")
	b.WriteString(epics.FormatValidationErrors(e.Errors))
	return b.String()
}

// loadPollardContext loads Pollard insights as a summary string for the agent prompt.
// Returns empty string if no insights are available.
func loadPollardContext(root string) string {
	allInsights, err := insights.LoadAll(root)
	if err != nil || len(allInsights) == 0 {
		return ""
	}
	var b strings.Builder
	for _, ins := range allInsights {
		b.WriteString("- **")
		b.WriteString(ins.Title)
		b.WriteString("** (")
		b.WriteString(string(ins.Category))
		b.WriteString(")")
		if len(ins.Findings) > 0 {
			b.WriteString(": ")
			b.WriteString(ins.Findings[0].Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// loadGurgehContext loads Gurgeh spec summaries for the agent prompt.
// Returns empty string if no specs are available.
func loadGurgehContext(root string) string {
	specsDir := gurgehProject.SpecsDir(root)
	summaries, _ := specs.LoadSummaries(specsDir)
	if len(summaries) == 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range summaries {
		b.WriteString("- **")
		b.WriteString(s.ID)
		b.WriteString("**: ")
		b.WriteString(s.Title)
		if s.Status != "" {
			b.WriteString(" [")
			b.WriteString(s.Status)
			b.WriteString("]")
		}
		if s.Summary != "" {
			b.WriteString(" — ")
			b.WriteString(s.Summary)
		}
		b.WriteString("\n")
	}
	return b.String()
}
