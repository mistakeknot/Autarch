package planstatus

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type GitInfo interface {
	LastCommitDate(repoRoot, path string) (time.Time, bool)
}

type Options struct {
	RepoRoot        string
	IntermuteRoot   string
	Now             time.Time
	Git             GitInfo
	DerivedEvidence map[string][]string
}

type Generator struct {
	repoRoot        string
	intermuteRoot   string
	now             time.Time
	git             GitInfo
	derivedEvidence map[string][]string
}

const reportDateLayout = "2006-01-02"

var (
	excludeSuffixes = []string{"-design.md", "-audit.md"}
	pathMappings    = []pathMapping{
		{"internal/vauxhall/", "internal/bigend/"},
		{"internal/praude/", "internal/gurgeh/"},
		{"internal/tandemonium/", "internal/coldwine/"},
		{"cmd/vauxhall/", "cmd/bigend/"},
		{".praude/", ".gurgeh/"},
		{".tandemonium/", ".coldwine/"},
	}
	defaultDerived = map[string][]string{
		"2026-01-28-feat-coordination-api-foundation-plan.md": {
			"pkg/httpapi/envelope.go",
			"internal/pollard/server/server.go",
			"pkg/jobs/jobs.go",
			"internal/pollard/server/cache.go",
			"pkg/netguard/bind.go",
			"internal/gurgeh/server/server.go",
			"internal/signals/cli/serve.go",
			"pkg/signals/server.go",
		},
	}
)

// GenerateReport builds the plan status report markdown.
func GenerateReport(opts Options) (string, error) {
	g := newGenerator(opts)
	return g.Generate()
}

func newGenerator(opts Options) *Generator {
	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		repoRoot = "."
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	git := opts.Git
	if git == nil {
		git = gitCLI{}
	}
	derived := opts.DerivedEvidence
	if derived == nil {
		derived = defaultDerived
	}
	return &Generator{
		repoRoot:        repoRoot,
		intermuteRoot:   opts.IntermuteRoot,
		now:             now,
		git:             git,
		derivedEvidence: derived,
	}
}

type pathMapping struct {
	from string
	to   string
}

type planStatus struct {
	name     string
	status   string
	evidence string
}

type planInfo struct {
	name           string
	path           string
	date           time.Time
	autarchPaths   []string
	intermutePaths []string
	todoHits       []todoInfo
}

type todoInfo struct {
	file   string
	status string
	text   string
}

func (g *Generator) Generate() (string, error) {
	plans, err := g.loadPlans()
	if err != nil {
		return "", err
	}

	statusRows, counts := g.classifyPlans(plans)

	buf := &bytes.Buffer{}
	w := bufio.NewWriter(buf)

	dateStr := g.now.Format(reportDateLayout)
	fmt.Fprintf(w, "# Plan Status Report — %s\n\n", dateStr)
	fmt.Fprintln(w, "> Generated from todos, git history, and codebase evidence. Status labels are heuristics unless backed by a todo.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Method")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "- Map legacy paths to current modules: vauxhall→bigend, praude→gurgeh, tandemonium→coldwine; .praude→.gurgeh; .tandemonium→.coldwine; cmd/vauxhall→cmd/bigend.")
	fmt.Fprintf(w, "- Scan %s and %s (for Intermute plan paths).\n", g.repoRoot, g.intermuteRootOrDefault())
	fmt.Fprintln(w, "- Status precedence: todo > derived evidence > commit evidence > preexisting > none.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Summary")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "- Todo-tracked: %d\n", counts.todo)
	fmt.Fprintf(w, "- Derived evidence: %d\n", counts.derived)
	fmt.Fprintf(w, "- Commit evidence: %d\n", counts.commit)
	fmt.Fprintf(w, "- Preexisting paths (no git evidence): %d\n", counts.preexisting)
	fmt.Fprintf(w, "- No evidence: %d\n", counts.none)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Status Legend")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "- todo:<status>: authoritative via todos/")
	fmt.Fprintln(w, "- derived: implemented by concrete subsystems even if plan has no path list")
	fmt.Fprintln(w, "- commit: referenced paths updated on/after plan date")
	fmt.Fprintln(w, "- preexisting: referenced paths exist but no commit evidence")
	fmt.Fprintln(w, "- none: no referenced paths found")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Plan Status")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "| Plan | Status | Evidence |")
	fmt.Fprintln(w, "| --- | --- | --- |")
	for _, row := range statusRows {
		fmt.Fprintf(w, "| %s | %s | %s |\n", escapePipes(row.name), escapePipes(row.status), escapePipes(row.evidence))
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Derived Evidence Details")
	fmt.Fprintln(w, "")
	for name, paths := range g.derivedEvidence {
		fmt.Fprintf(w, "- %s\n", name)
		for _, p := range paths {
			fmt.Fprintf(w, "  - %s\n", p)
		}
	}

	if err := w.Flush(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type statusCounts struct {
	todo       int
	derived    int
	commit     int
	preexisting int
	none       int
}

func (g *Generator) classifyPlans(plans []planInfo) ([]planStatus, statusCounts) {
	rows := make([]planStatus, 0, len(plans))
	counts := statusCounts{}

	for _, plan := range plans {
		status := planStatus{name: plan.name}
		if len(plan.todoHits) > 0 {
			status.status = fmt.Sprintf("todo:%s", plan.todoHits[0].status)
			status.evidence = plan.todoHits[0].file
			counts.todo++
			rows = append(rows, status)
			continue
		}
		if derived, ok := g.derivedEvidence[plan.name]; ok {
			status.status = "derived"
			status.evidence = strings.Join(derived, ", ")
			counts.derived++
			rows = append(rows, status)
			continue
		}

		commitCount, latestCommit := g.commitEvidence(plan)
		if commitCount > 0 {
			status.status = "commit"
			status.evidence = fmt.Sprintf("paths:%d latest:%s", commitCount, latestCommit)
			counts.commit++
			rows = append(rows, status)
			continue
		}

		existingCount := g.existingPathCount(plan)
		if existingCount > 0 {
			status.status = "preexisting"
			status.evidence = fmt.Sprintf("paths:%d", existingCount)
			counts.preexisting++
			rows = append(rows, status)
			continue
		}

		status.status = "none"
		status.evidence = "no referenced paths found"
		counts.none++
		rows = append(rows, status)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	return rows, counts
}

func (g *Generator) commitEvidence(plan planInfo) (int, string) {
	planDate := plan.date
	commitCount := 0
	latest := time.Time{}

	for _, p := range plan.autarchPaths {
		full := filepath.Join(g.repoRoot, p)
		if _, err := os.Stat(full); err != nil {
			continue
		}
		date, ok := g.git.LastCommitDate(g.repoRoot, p)
		if !ok {
			continue
		}
		if !planDate.IsZero() && !date.Before(planDate) {
			commitCount++
		}
		if date.After(latest) {
			latest = date
		}
	}

	for _, p := range plan.intermutePaths {
		root := g.intermuteRootOrDefault()
		if root == "" {
			break
		}
		full := filepath.Join(root, p)
		if _, err := os.Stat(full); err != nil {
			continue
		}
		date, ok := g.git.LastCommitDate(root, p)
		if !ok {
			continue
		}
		if !planDate.IsZero() && !date.Before(planDate) {
			commitCount++
		}
		if date.After(latest) {
			latest = date
		}
	}

	if commitCount == 0 {
		return 0, ""
	}

	return commitCount, latest.Format(reportDateLayout)
}

func (g *Generator) existingPathCount(plan planInfo) int {
	count := 0
	for _, p := range plan.autarchPaths {
		full := filepath.Join(g.repoRoot, p)
		if _, err := os.Stat(full); err == nil {
			count++
		}
	}
	root := g.intermuteRootOrDefault()
	if root == "" {
		return count
	}
	for _, p := range plan.intermutePaths {
		full := filepath.Join(root, p)
		if _, err := os.Stat(full); err == nil {
			count++
		}
	}
	return count
}

func (g *Generator) loadPlans() ([]planInfo, error) {
	plansDir := filepath.Join(g.repoRoot, "docs", "plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return nil, err
	}

	todos, err := g.loadTodos()
	if err != nil {
		return nil, err
	}

	var plans []planInfo
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)
		if lower == "index.md" || lower == "status.md" || strings.HasPrefix(lower, "status-") {
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if hasAnySuffix(name, excludeSuffixes) {
			continue
		}

		path := filepath.Join(plansDir, name)
		text, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		plan := planInfo{
			name: name,
			path: path,
			date: parsePlanDate(name),
		}
		autarchPaths, intermutePaths := g.extractPaths(string(text))
		plan.autarchPaths = autarchPaths
		plan.intermutePaths = intermutePaths
		plan.todoHits = matchTodos(name, path, todos)
		plans = append(plans, plan)
	}
	return plans, nil
}

func (g *Generator) loadTodos() ([]todoInfo, error) {
	todosDir := filepath.Join(g.repoRoot, "todos")
	entries, err := os.ReadDir(todosDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var todos []todoInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(todosDir, entry.Name())
		text, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		todos = append(todos, todoInfo{
			file:   entry.Name(),
			status: parseTodoStatus(string(text)),
			text:   string(text),
		})
	}
	return todos, nil
}

var backtickPattern = regexp.MustCompile("`([^`]+)`")

func (g *Generator) extractPaths(text string) ([]string, []string) {
	var paths []string
	var intermute []string

	backticks := backtickPattern.FindAllStringSubmatch(text, -1)
	for _, match := range backticks {
		candidate := normalizeCandidate(match[1])
		if candidate == "" {
			continue
		}
		if addPath(candidate, &paths, &intermute) {
			continue
		}
	}

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if !strings.Contains(line, "Create:") {
			continue
		}
		for _, match := range backtickPattern.FindAllStringSubmatch(line, -1) {
			candidate := normalizeCandidate(match[1])
			if candidate == "" {
				continue
			}
			addPath(candidate, &paths, &intermute)
		}
	}

	paths = uniq(paths)
	intermute = uniq(intermute)
	return paths, intermute
}

func normalizeCandidate(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	candidate = strings.TrimSuffix(candidate, ":")
	candidate = strings.TrimPrefix(candidate, "./")
	if candidate == "" {
		return ""
	}
	if strings.Contains(candidate, "*") {
		return ""
	}
	if strings.HasPrefix(candidate, "http") {
		return ""
	}
	if strings.HasPrefix(candidate, "go ") || strings.HasPrefix(candidate, "git ") {
		return ""
	}
	if !strings.Contains(candidate, "/") {
		return ""
	}
	return candidate
}

func addPath(candidate string, paths *[]string, intermute *[]string) bool {
	if strings.HasPrefix(candidate, "/root/projects/Intermute/") {
		*intermute = append(*intermute, strings.TrimPrefix(candidate, "/root/projects/Intermute/"))
		return true
	}
	if strings.HasPrefix(candidate, "../Intermute/") {
		*intermute = append(*intermute, strings.TrimPrefix(candidate, "../Intermute/"))
		return true
	}
	if strings.HasPrefix(candidate, "Intermute/") {
		*intermute = append(*intermute, strings.TrimPrefix(candidate, "Intermute/"))
		return true
	}

	mapped := candidate
	for _, mapping := range pathMappings {
		if strings.HasPrefix(candidate, mapping.from) {
			mapped = mapping.to + strings.TrimPrefix(candidate, mapping.from)
			break
		}
	}
	*paths = append(*paths, mapped)
	return true
}

func parseTodoStatus(text string) string {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "status:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "status:"))
		}
	}
	return "unknown"
}

func matchTodos(planName, planPath string, todos []todoInfo) []todoInfo {
	var hits []todoInfo
	for _, todo := range todos {
		if strings.Contains(todo.text, planName) || strings.Contains(todo.text, planPath) {
			hits = append(hits, todo)
		}
	}
	return hits
}

func parsePlanDate(name string) time.Time {
	if len(name) < len(reportDateLayout) {
		return time.Time{}
	}
	datePart := name[:len(reportDateLayout)]
	date, err := time.Parse(reportDateLayout, datePart)
	if err != nil {
		return time.Time{}
	}
	return date
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func hasAnySuffix(name string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func uniq(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	var out []string
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (g *Generator) intermuteRootOrDefault() string {
	if g.intermuteRoot != "" {
		return g.intermuteRoot
	}
	defaultPath := "/root/projects/Intermute"
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}
	return ""
}

type gitCLI struct{}

func (g gitCLI) LastCommitDate(repoRoot, path string) (time.Time, bool) {
	cmd := exec.Command("git", "-C", repoRoot, "log", "-1", "--format=%cs", "--", path)
	output, err := cmd.Output()
	if err != nil {
		return time.Time{}, false
	}
	out := strings.TrimSpace(string(output))
	if out == "" {
		return time.Time{}, false
	}
	date, err := time.Parse(reportDateLayout, out)
	if err != nil {
		return time.Time{}, false
	}
	return date, true
}
