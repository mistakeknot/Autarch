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

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/agent"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/epics"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/explore"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/initflow"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"gopkg.in/yaml.v3"
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
		Summary: loadSummary(planDir),
		Depth:   depth,
		Repo:    root,
	})
	if err != nil {
		return err
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
	args := append([]string{}, target.Args...)
	args = append(args, promptPath)
	cmd := exec.CommandContext(ctx, target.Command, args...)
	if len(target.Env) > 0 {
		env := os.Environ()
		for key, value := range target.Env {
			env = append(env, key+"="+value)
		}
		cmd.Env = env
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return initflow.Result{}, fmt.Errorf("agent run failed: %w: %s", err, output.String())
	}
	epicsList, err := parseAgentEpics(output.Bytes())
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

func buildAgentPrompt(input initflow.Input) string {
	return fmt.Sprintf("# Tandemonium Init: Epic + Story Generation\n\n"+
		"You are generating epic/story specs for a repo. Read the exploration summary and output YAML only.\n\n"+
		"Output schema:\n\n"+
		"```yaml\n"+
		"epics:\n"+
		"  - id: EPIC-001\n"+
		"    title: Example\n"+
		"    summary: Short description\n"+
		"    status: todo\n"+
		"    priority: p1\n"+
		"    acceptance_criteria:\n"+
		"      - ...\n"+
		"    risks:\n"+
		"      - ...\n"+
		"    estimates: \"S\"\n"+
		"    stories:\n"+
		"      - id: EPIC-001-S01\n"+
		"        title: Story title\n"+
		"        summary: Story summary\n"+
		"        status: todo\n"+
		"        priority: p1\n"+
		"        acceptance_criteria:\n"+
		"          - ...\n"+
		"        risks:\n"+
		"          - ...\n"+
		"        estimates: \"S\"\n"+
		"```\n\n"+
		"Exploration Summary:\n\n%s\n", input.Summary)
}

func parseAgentEpics(raw []byte) ([]epics.Epic, error) {
	var wrapper struct {
		Epics []epics.Epic `yaml:"epics"`
	}
	if err := yaml.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Epics) > 0 {
		return wrapper.Epics, nil
	}
	var list []epics.Epic
	if err := yaml.Unmarshal(raw, &list); err == nil && len(list) > 0 {
		return list, nil
	}
	if idx := bytes.Index(raw, []byte("epics:")); idx >= 0 {
		trimmed := raw[idx:]
		if err := yaml.Unmarshal(trimmed, &wrapper); err == nil && len(wrapper.Epics) > 0 {
			return wrapper.Epics, nil
		}
	}
	return nil, fmt.Errorf("agent output missing epics")
}
