// Package api provides a programmatic interface for running Pollard hunters.
//
// This package supports two integration modes:
//
// 1. Direct API (current): Praude/Tandemonium call Scanner methods directly
// 2. Intermute (future): Message-based coordination via Intermute server
//
// When Intermute is built, Pollard will register as an agent and receive
// research requests via its Intermute inbox. The Scanner API will remain
// available for direct integration.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/hunters"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/insights"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/sources"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/state"
	"gopkg.in/yaml.v3"
)

// Scanner provides programmatic access to Pollard hunters.
type Scanner struct {
	projectPath string
	config      *config.Config
	registry    *hunters.Registry
	db          *state.DB
}

// ScanOptions configures a scan operation.
type ScanOptions struct {
	// Hunters to run (empty means all enabled hunters)
	Hunters []string

	// Queries to use (overrides config if set)
	Queries []string

	// Targets for competitor tracking (overrides config if set)
	Targets []CompetitorTarget

	// MaxResults limits results per query
	MaxResults int
}

// CompetitorTarget defines a competitor to track.
type CompetitorTarget struct {
	Name      string
	Changelog string
	Docs      string
	GitHub    string
}

// ScanResult holds the combined results from all hunters.
type ScanResult struct {
	HunterResults map[string]*hunters.HuntResult
	TotalSources  int
	TotalInsights int
	OutputFiles   []string
	Errors        []error
}

// NewScanner creates a Scanner for the given project path.
func NewScanner(projectPath string) (*Scanner, error) {
	cfg, err := config.Load(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := sources.EnsureDirectories(projectPath); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	db, err := state.Open(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	return &Scanner{
		projectPath: projectPath,
		config:      cfg,
		registry:    hunters.DefaultRegistry(),
		db:          db,
	}, nil
}

// Close releases resources held by the scanner.
func (s *Scanner) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Scan runs the specified hunters (or all enabled hunters if none specified).
func (s *Scanner) Scan(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	result := &ScanResult{
		HunterResults: make(map[string]*hunters.HuntResult),
	}

	// Determine which hunters to run
	hunterNames := opts.Hunters
	if len(hunterNames) == 0 {
		hunterNames = s.config.EnabledHunters()
	}

	if len(hunterNames) == 0 {
		return result, nil
	}

	// Run each hunter
	for _, name := range hunterNames {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			return result, nil
		default:
		}

		hunter, ok := s.registry.Get(name)
		if !ok {
			result.Errors = append(result.Errors, fmt.Errorf("hunter %q not found", name))
			continue
		}

		hunterCfg, _ := s.config.GetHunterConfig(name)

		// Build hunter config
		hCfg := hunters.HunterConfig{
			Queries:     hunterCfg.Queries,
			MaxResults:  hunterCfg.MaxResults,
			MinStars:    hunterCfg.MinStars,
			MinPoints:   hunterCfg.MinPoints,
			Categories:  hunterCfg.Categories,
			OutputDir:   hunterCfg.Output,
			ProjectPath: s.projectPath,
		}

		// Override with opts if specified
		if len(opts.Queries) > 0 {
			hCfg.Queries = opts.Queries
		}
		if opts.MaxResults > 0 {
			hCfg.MaxResults = opts.MaxResults
		}

		// Add targets for competitor tracker
		if len(opts.Targets) > 0 {
			for _, t := range opts.Targets {
				hCfg.Targets = append(hCfg.Targets, hunters.CompetitorTarget{
					Name:      t.Name,
					Changelog: t.Changelog,
					Docs:      t.Docs,
					GitHub:    t.GitHub,
				})
			}
		} else {
			for _, t := range hunterCfg.Targets {
				hCfg.Targets = append(hCfg.Targets, hunters.CompetitorTarget{
					Name:      t.Name,
					Changelog: t.Changelog,
					Docs:      t.Docs,
					GitHub:    t.GitHub,
				})
			}
		}

		// Record run start
		runID, err := s.db.StartRun(name)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to record run start for %s: %w", name, err))
		}

		// Execute the hunt
		huntResult, err := hunter.Hunt(ctx, hCfg)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("hunter %s failed: %w", name, err))
			if runID > 0 {
				s.db.CompleteRun(runID, false, 0, 0, err.Error())
			}
			continue
		}

		// Record run completion
		success := huntResult.Success()
		errMsg := ""
		if !success && len(huntResult.Errors) > 0 {
			errMsg = huntResult.Errors[0].Error()
		}
		if runID > 0 {
			s.db.CompleteRun(runID, success, huntResult.SourcesCollected, huntResult.InsightsCreated, errMsg)
		}

		result.HunterResults[name] = huntResult
		result.TotalSources += huntResult.SourcesCollected
		result.TotalInsights += huntResult.InsightsCreated
		result.OutputFiles = append(result.OutputFiles, huntResult.OutputFiles...)
		result.Errors = append(result.Errors, huntResult.Errors...)
	}

	return result, nil
}

// ScanGitHub runs only the GitHub Scout hunter with the specified queries.
func (s *Scanner) ScanGitHub(ctx context.Context, queries []string, maxResults int) (*hunters.HuntResult, error) {
	hunter, ok := s.registry.Get("github-scout")
	if !ok {
		return nil, fmt.Errorf("github-scout hunter not found")
	}

	hunterCfg, _ := s.config.GetHunterConfig("github-scout")

	cfg := hunters.HunterConfig{
		Queries:     queries,
		MaxResults:  maxResults,
		MinStars:    hunterCfg.MinStars,
		OutputDir:   hunterCfg.Output,
		ProjectPath: s.projectPath,
	}

	if cfg.MaxResults == 0 {
		cfg.MaxResults = 50
	}

	return hunter.Hunt(ctx, cfg)
}

// ScanTrends runs only the HackerNews TrendWatcher hunter.
func (s *Scanner) ScanTrends(ctx context.Context, queries []string, minPoints int) (*hunters.HuntResult, error) {
	hunter, ok := s.registry.Get("hackernews")
	if !ok {
		return nil, fmt.Errorf("hackernews hunter not found")
	}

	hunterCfg, _ := s.config.GetHunterConfig("hackernews")

	cfg := hunters.HunterConfig{
		Queries:     queries,
		MinPoints:   minPoints,
		OutputDir:   hunterCfg.Output,
		ProjectPath: s.projectPath,
	}

	if cfg.MinPoints == 0 {
		cfg.MinPoints = 50
	}

	return hunter.Hunt(ctx, cfg)
}

// ScanResearch runs only the arXiv ResearchScout hunter.
func (s *Scanner) ScanResearch(ctx context.Context, queries []string, categories []string) (*hunters.HuntResult, error) {
	hunter, ok := s.registry.Get("arxiv")
	if !ok {
		return nil, fmt.Errorf("arxiv hunter not found")
	}

	hunterCfg, _ := s.config.GetHunterConfig("arxiv")

	cfg := hunters.HunterConfig{
		Queries:     queries,
		Categories:  categories,
		MaxResults:  hunterCfg.MaxResults,
		OutputDir:   hunterCfg.Output,
		ProjectPath: s.projectPath,
	}

	if cfg.MaxResults == 0 {
		cfg.MaxResults = 50
	}

	return hunter.Hunt(ctx, cfg)
}

// ScanCompetitors runs only the Competitor Tracker hunter.
func (s *Scanner) ScanCompetitors(ctx context.Context, targets []CompetitorTarget) (*hunters.HuntResult, error) {
	hunter, ok := s.registry.Get("competitor-tracker")
	if !ok {
		return nil, fmt.Errorf("competitor-tracker hunter not found")
	}

	hunterCfg, _ := s.config.GetHunterConfig("competitor-tracker")

	cfg := hunters.HunterConfig{
		OutputDir:   hunterCfg.Output,
		ProjectPath: s.projectPath,
	}

	for _, t := range targets {
		cfg.Targets = append(cfg.Targets, hunters.CompetitorTarget{
			Name:      t.Name,
			Changelog: t.Changelog,
			Docs:      t.Docs,
			GitHub:    t.GitHub,
		})
	}

	return hunter.Hunt(ctx, cfg)
}

// ResearchUserPersonas generates research queries from user persona information.
// This is designed to be called from Praude's interview flow.
func (s *Scanner) ResearchUserPersonas(ctx context.Context, personas []string, painpoints []string) (*ScanResult, error) {
	// Build queries from personas and painpoints
	var queries []string

	for _, persona := range personas {
		queries = append(queries, persona+" tools")
		queries = append(queries, persona+" workflow")
	}

	for _, painpoint := range painpoints {
		queries = append(queries, painpoint+" solution")
		queries = append(queries, painpoint+" alternative")
	}

	// Run both GitHub and HackerNews searches
	return s.Scan(ctx, ScanOptions{
		Hunters: []string{"github-scout", "hackernews"},
		Queries: queries,
	})
}

// ResearchForPRD runs comprehensive research based on a PRD's content.
// This is designed to be called from Praude after PRD creation.
func (s *Scanner) ResearchForPRD(ctx context.Context, vision, problem string, requirements []string) (*ScanResult, error) {
	var queries []string

	// Generate queries from PRD content
	if vision != "" {
		queries = append(queries, vision)
	}
	if problem != "" {
		queries = append(queries, problem+" solution")
	}
	for _, req := range requirements {
		if req != "" {
			queries = append(queries, req)
		}
	}

	// Limit to most relevant queries
	if len(queries) > 5 {
		queries = queries[:5]
	}

	return s.Scan(ctx, ScanOptions{
		Hunters:    []string{"github-scout", "hackernews", "arxiv"},
		Queries:    queries,
		MaxResults: 20,
	})
}

// ResearchForEpic runs research focused on implementation patterns.
// This is designed to be called from Tandemonium for epic planning.
func (s *Scanner) ResearchForEpic(ctx context.Context, epicTitle, epicDescription string) (*ScanResult, error) {
	var queries []string

	if epicTitle != "" {
		queries = append(queries, epicTitle+" implementation")
		queries = append(queries, epicTitle+" library")
	}
	if epicDescription != "" {
		queries = append(queries, epicDescription)
	}

	return s.Scan(ctx, ScanOptions{
		Hunters:    []string{"github-scout"},
		Queries:    queries,
		MaxResults: 30,
	})
}

// GetGitHubToken returns the configured GitHub token for API access.
func GetGitHubToken() string {
	return os.Getenv("GITHUB_TOKEN")
}

// GetInsightsForFeature returns insights linked to a specific feature.
// This is designed to be called from Tandemonium when assigning stories.
func (s *Scanner) GetInsightsForFeature(ctx context.Context, featureRef string) ([]*insights.Insight, error) {
	allInsights, err := insights.LoadAll(s.projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load insights: %w", err)
	}

	// Filter insights that have this feature in LinkedFeatures
	var matched []*insights.Insight
	for _, insight := range allInsights {
		for _, linkedFeature := range insight.LinkedFeatures {
			if strings.EqualFold(linkedFeature, featureRef) {
				matched = append(matched, insight)
				break
			}
		}
	}

	return matched, nil
}

// GenerateResearchBrief creates a summary of research for a feature/epic.
// This is attached to Tandemonium mail messages when assigning work.
func (s *Scanner) GenerateResearchBrief(ctx context.Context, featureRef string) (string, error) {
	linkedInsights, err := s.GetInsightsForFeature(ctx, featureRef)
	if err != nil {
		return "", err
	}

	if len(linkedInsights) == 0 {
		return "", nil
	}

	var brief strings.Builder
	brief.WriteString("## Research Context\n\n")

	// Group by relevance
	var highRelevance, mediumRelevance []string
	for _, insight := range linkedInsights {
		for _, finding := range insight.Findings {
			entry := fmt.Sprintf("**%s**: %s", finding.Title, finding.Description)
			if finding.Relevance == insights.RelevanceHigh {
				highRelevance = append(highRelevance, entry)
			} else if finding.Relevance == insights.RelevanceMedium {
				mediumRelevance = append(mediumRelevance, entry)
			}
		}
	}

	if len(highRelevance) > 0 {
		brief.WriteString("### Key Insights (HIGH relevance)\n")
		for i, entry := range highRelevance {
			if i >= 3 {
				break // Limit to top 3
			}
			brief.WriteString(fmt.Sprintf("- %s\n", entry))
		}
		brief.WriteString("\n")
	}

	if len(mediumRelevance) > 0 {
		brief.WriteString("### Additional Context (MEDIUM relevance)\n")
		for i, entry := range mediumRelevance {
			if i >= 2 {
				break // Limit to top 2
			}
			brief.WriteString(fmt.Sprintf("- %s\n", entry))
		}
		brief.WriteString("\n")
	}

	// Add recommendations if any
	var recommendations []string
	for _, insight := range linkedInsights {
		for _, rec := range insight.Recommendations {
			recommendations = append(recommendations, fmt.Sprintf("- **%s** (%s): %s",
				rec.FeatureHint, rec.Priority, rec.Rationale))
		}
	}

	if len(recommendations) > 0 {
		brief.WriteString("### Recommendations\n")
		for _, rec := range recommendations {
			brief.WriteString(rec + "\n")
		}
	}

	return brief.String(), nil
}

// =============================================================================
// Intermute Message Handling
// =============================================================================
// These types and methods provide Intermute-compatible message handling.
// Currently uses file-based messaging; will transition to Intermute HTTP API.

// MessageType identifies the kind of research request.
type MessageType string

const (
	TypeResearchRequest  MessageType = "research_request"
	TypeResearchComplete MessageType = "research_complete"
	TypeScanRequest      MessageType = "scan_request"
	TypeScanComplete     MessageType = "scan_complete"
)

// ResearchMessage is an Intermute-compatible message for research requests.
type ResearchMessage struct {
	ID        string            `yaml:"id" json:"id"`
	Type      MessageType       `yaml:"type" json:"type"`
	From      string            `yaml:"from" json:"from"` // praude, tandemonium
	To        string            `yaml:"to" json:"to"`     // pollard
	CreatedAt time.Time         `yaml:"created_at" json:"created_at"`
	Status    string            `yaml:"status" json:"status"` // pending, processing, complete, failed
	Payload   ResearchPayload   `yaml:"payload" json:"payload"`
	Response  *ResearchResponse `yaml:"response,omitempty" json:"response,omitempty"`
}

// ResearchPayload contains the research request details.
type ResearchPayload struct {
	// Request type
	RequestType string `yaml:"request_type" json:"request_type"` // prd, epic, persona, general

	// Source context
	SourceID   string `yaml:"source_id,omitempty" json:"source_id,omitempty"`     // PRD-001, EPIC-001
	SourceType string `yaml:"source_type,omitempty" json:"source_type,omitempty"` // prd, epic, feature

	// Research parameters
	Queries    []string `yaml:"queries,omitempty" json:"queries,omitempty"`
	Personas   []string `yaml:"personas,omitempty" json:"personas,omitempty"`
	Painpoints []string `yaml:"painpoints,omitempty" json:"painpoints,omitempty"`
	Vision     string   `yaml:"vision,omitempty" json:"vision,omitempty"`
	Problem    string   `yaml:"problem,omitempty" json:"problem,omitempty"`

	// Optional overrides
	Hunters    []string           `yaml:"hunters,omitempty" json:"hunters,omitempty"`
	MaxResults int                `yaml:"max_results,omitempty" json:"max_results,omitempty"`
	Targets    []CompetitorTarget `yaml:"targets,omitempty" json:"targets,omitempty"`
}

// ResearchResponse contains the research result.
type ResearchResponse struct {
	TotalSources  int      `yaml:"total_sources" json:"total_sources"`
	TotalInsights int      `yaml:"total_insights" json:"total_insights"`
	OutputFiles   []string `yaml:"output_files" json:"output_files"`
	Errors        []string `yaml:"errors,omitempty" json:"errors,omitempty"`
	CompletedAt   string   `yaml:"completed_at" json:"completed_at"`
}

// InboxPath returns the path to Pollard's message inbox.
func InboxPath(projectPath string) string {
	return filepath.Join(projectPath, ".pollard", "inbox")
}

// ProcessInbox processes pending messages in Pollard's inbox.
// This is the Intermute-compatible message handling interface.
func (s *Scanner) ProcessInbox(ctx context.Context) error {
	inboxDir := InboxPath(s.projectPath)
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return fmt.Errorf("failed to create inbox: %w", err)
	}

	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		return fmt.Errorf("failed to read inbox: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		msgPath := filepath.Join(inboxDir, entry.Name())
		if err := s.processMessage(ctx, msgPath); err != nil {
			// Log error but continue processing other messages
			continue
		}
	}

	return nil
}

// processMessage handles a single inbox message.
func (s *Scanner) processMessage(ctx context.Context, msgPath string) error {
	data, err := os.ReadFile(msgPath)
	if err != nil {
		return err
	}

	var msg ResearchMessage
	if err := yaml.Unmarshal(data, &msg); err != nil {
		return err
	}

	// Skip already processed messages
	if msg.Status != "pending" {
		return nil
	}

	// Mark as processing
	msg.Status = "processing"
	if err := s.saveMessage(msgPath, &msg); err != nil {
		return err
	}

	// Process based on request type
	var result *ScanResult
	var scanErr error

	switch msg.Payload.RequestType {
	case "prd":
		result, scanErr = s.ResearchForPRD(ctx, msg.Payload.Vision, msg.Payload.Problem, msg.Payload.Queries)
	case "epic":
		result, scanErr = s.ResearchForEpic(ctx, msg.Payload.Vision, msg.Payload.Problem)
	case "persona":
		result, scanErr = s.ResearchUserPersonas(ctx, msg.Payload.Personas, msg.Payload.Painpoints)
	default:
		// General scan with provided hunters/queries
		result, scanErr = s.Scan(ctx, ScanOptions{
			Hunters:    msg.Payload.Hunters,
			Queries:    msg.Payload.Queries,
			Targets:    msg.Payload.Targets,
			MaxResults: msg.Payload.MaxResults,
		})
	}

	// Build response
	response := &ResearchResponse{
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if scanErr != nil {
		msg.Status = "failed"
		response.Errors = []string{scanErr.Error()}
	} else {
		msg.Status = "complete"
		response.TotalSources = result.TotalSources
		response.TotalInsights = result.TotalInsights
		response.OutputFiles = result.OutputFiles
		for _, e := range result.Errors {
			response.Errors = append(response.Errors, e.Error())
		}
	}

	msg.Response = response
	return s.saveMessage(msgPath, &msg)
}

// saveMessage writes a message back to disk.
func (s *Scanner) saveMessage(path string, msg *ResearchMessage) error {
	data, err := yaml.Marshal(msg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SendResearchRequest creates a research request message for Pollard.
// This is called by Praude/Tandemonium to request research.
func SendResearchRequest(projectPath string, payload ResearchPayload, from string) (*ResearchMessage, error) {
	inboxDir := InboxPath(projectPath)
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create inbox: %w", err)
	}

	msg := &ResearchMessage{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Type:      TypeResearchRequest,
		From:      from,
		To:        "pollard",
		CreatedAt: time.Now().UTC(),
		Status:    "pending",
		Payload:   payload,
	}

	filename := fmt.Sprintf("%s-%s.yaml", msg.CreatedAt.Format("20060102-150405"), msg.ID)
	path := filepath.Join(inboxDir, filename)

	data, err := yaml.Marshal(msg)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, err
	}

	return msg, nil
}

// WaitForResponse waits for a research request to complete.
// This polls the message file until status changes from pending/processing.
func WaitForResponse(projectPath, msgID string, timeout time.Duration) (*ResearchMessage, error) {
	inboxDir := InboxPath(projectPath)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(inboxDir)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if !containsID(entry.Name(), msgID) {
				continue
			}

			path := filepath.Join(inboxDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var msg ResearchMessage
			if err := yaml.Unmarshal(data, &msg); err != nil {
				continue
			}

			if msg.Status == "complete" || msg.Status == "failed" {
				return &msg, nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for response")
}

func containsID(filename, id string) bool {
	return len(filename) > len(id) && filename[len(filename)-len(id)-5:len(filename)-5] == id
}

// MarshalJSON implements JSON marshaling for ResearchMessage.
func (m *ResearchMessage) MarshalJSON() ([]byte, error) {
	type Alias ResearchMessage
	return json.Marshal((*Alias)(m))
}
