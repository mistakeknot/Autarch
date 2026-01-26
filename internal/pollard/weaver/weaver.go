// Package weaver orchestrates parallel Pollard research and weaves findings into coherent context.
// The weaver subagent spawns multiple hunters in parallel, aggregates their results,
// and synthesizes insights suitable for surfacing during Gurgeh spec creation.
package weaver

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// HunterType represents the type of research hunter
type HunterType string

const (
	HunterGitHubScout     HunterType = "github-scout"     // OSS implementations
	HunterTrendWatcher    HunterType = "trend-watcher"    // Industry discourse
	HunterCompetitorTrack HunterType = "competitor-track" // Competitor analysis
	HunterAcademic        HunterType = "academic"         // Academic papers
	HunterTechnical       HunterType = "technical"        // Technical documentation
)

// InsightType categorizes the type of insight
type InsightType string

const (
	InsightImplementation InsightType = "implementation" // How others built it
	InsightPattern        InsightType = "pattern"        // Common patterns/anti-patterns
	InsightRisk           InsightType = "risk"           // Risks to avoid
	InsightOpportunity    InsightType = "opportunity"    // Opportunities to leverage
	InsightCompetitor     InsightType = "competitor"     // Competitor intelligence
	InsightTrend          InsightType = "trend"          // Market/tech trends
)

// Confidence indicates how confident we are in an insight
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// HunterResult represents results from a single hunter
type HunterResult struct {
	Hunter      HunterType `yaml:"hunter" json:"hunter"`
	Query       string     `yaml:"query" json:"query"`
	ResultCount int        `yaml:"result_count" json:"result_count"`
	Insights    []Insight  `yaml:"insights" json:"insights"`
	Duration    time.Duration `yaml:"duration" json:"duration"`
	Error       string     `yaml:"error,omitempty" json:"error,omitempty"`
}

// Insight represents a single research insight
type Insight struct {
	ID          string      `yaml:"id" json:"id"`
	Type        InsightType `yaml:"type" json:"type"`
	Title       string      `yaml:"title" json:"title"`
	Summary     string      `yaml:"summary" json:"summary"`
	Source      string      `yaml:"source" json:"source"`           // URL or reference
	SourceType  HunterType  `yaml:"source_type" json:"source_type"` // Which hunter found it
	Confidence  Confidence  `yaml:"confidence" json:"confidence"`
	Relevance   float64     `yaml:"relevance" json:"relevance"` // 0-1 relevance score
	Tags        []string    `yaml:"tags,omitempty" json:"tags,omitempty"`
	ExtractedAt time.Time   `yaml:"extracted_at" json:"extracted_at"`
}

// WovenContext represents the synthesized research context
type WovenContext struct {
	Query           string         `yaml:"query" json:"query"`
	Vision          string         `yaml:"vision" json:"vision"`
	Problem         string         `yaml:"problem" json:"problem"`
	HunterResults   []HunterResult `yaml:"hunter_results" json:"hunter_results"`
	SynthesizedInsights []Insight  `yaml:"synthesized_insights" json:"synthesized_insights"`
	Themes          []Theme        `yaml:"themes" json:"themes"`
	Recommendations []string       `yaml:"recommendations" json:"recommendations"`
	GeneratedAt     time.Time      `yaml:"generated_at" json:"generated_at"`
}

// Theme represents a recurring theme across insights
type Theme struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	InsightIDs  []string `yaml:"insight_ids" json:"insight_ids"`
	Frequency   int      `yaml:"frequency" json:"frequency"`
}

// ResearchRequest represents a request for research
type ResearchRequest struct {
	Vision       string   `yaml:"vision" json:"vision"`
	Problem      string   `yaml:"problem" json:"problem"`
	Requirements []string `yaml:"requirements,omitempty" json:"requirements,omitempty"`
	Keywords     []string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
	Hunters      []HunterType `yaml:"hunters,omitempty" json:"hunters,omitempty"` // Specific hunters to use
}

// Weaver orchestrates research across multiple hunters
type Weaver struct {
	insightCounter int
}

// NewWeaver creates a new research weaver
func NewWeaver() *Weaver {
	return &Weaver{}
}

// Weave orchestrates research and synthesizes results
func (w *Weaver) Weave(request *ResearchRequest) *WovenContext {
	context := &WovenContext{
		Query:       w.buildQuery(request),
		Vision:      request.Vision,
		Problem:     request.Problem,
		GeneratedAt: time.Now(),
	}

	// Determine which hunters to use
	hunters := request.Hunters
	if len(hunters) == 0 {
		hunters = w.selectHunters(request)
	}

	// Simulate parallel hunter execution (in real implementation, this would be concurrent)
	for _, hunter := range hunters {
		result := w.runHunter(hunter, request)
		context.HunterResults = append(context.HunterResults, result)
	}

	// Synthesize insights from all hunters
	context.SynthesizedInsights = w.synthesizeInsights(context.HunterResults)

	// Identify themes
	context.Themes = w.identifyThemes(context.SynthesizedInsights)

	// Generate recommendations
	context.Recommendations = w.generateRecommendations(context)

	return context
}

// buildQuery creates a search query from the request
func (w *Weaver) buildQuery(request *ResearchRequest) string {
	parts := []string{}
	if request.Vision != "" {
		parts = append(parts, request.Vision)
	}
	if request.Problem != "" {
		parts = append(parts, request.Problem)
	}
	parts = append(parts, request.Keywords...)
	return strings.Join(parts, " ")
}

// selectHunters determines which hunters to use based on the request
func (w *Weaver) selectHunters(request *ResearchRequest) []HunterType {
	hunters := []HunterType{HunterGitHubScout} // Always include OSS search

	text := strings.ToLower(request.Vision + " " + request.Problem + " " + strings.Join(request.Requirements, " "))

	// Add trend watcher for market/industry terms
	if strings.Contains(text, "market") || strings.Contains(text, "trend") ||
		strings.Contains(text, "industry") || strings.Contains(text, "users") {
		hunters = append(hunters, HunterTrendWatcher)
	}

	// Add competitor tracker for competitive terms
	if strings.Contains(text, "competitor") || strings.Contains(text, "alternative") ||
		strings.Contains(text, "better than") || strings.Contains(text, "vs") {
		hunters = append(hunters, HunterCompetitorTrack)
	}

	// Add academic for research-heavy terms
	if strings.Contains(text, "algorithm") || strings.Contains(text, "research") ||
		strings.Contains(text, "study") || strings.Contains(text, "ml") ||
		strings.Contains(text, "machine learning") || strings.Contains(text, "ai") {
		hunters = append(hunters, HunterAcademic)
	}

	// Add technical for implementation terms
	if strings.Contains(text, "api") || strings.Contains(text, "framework") ||
		strings.Contains(text, "library") || strings.Contains(text, "sdk") ||
		strings.Contains(text, "integration") {
		hunters = append(hunters, HunterTechnical)
	}

	return hunters
}

// runHunter simulates running a single hunter
func (w *Weaver) runHunter(hunter HunterType, request *ResearchRequest) HunterResult {
	result := HunterResult{
		Hunter:   hunter,
		Query:    w.buildQuery(request),
		Duration: 500 * time.Millisecond, // Simulated duration
	}

	// Generate simulated insights based on hunter type
	switch hunter {
	case HunterGitHubScout:
		result.Insights = w.generateGitHubInsights(request)
	case HunterTrendWatcher:
		result.Insights = w.generateTrendInsights(request)
	case HunterCompetitorTrack:
		result.Insights = w.generateCompetitorInsights(request)
	case HunterAcademic:
		result.Insights = w.generateAcademicInsights(request)
	case HunterTechnical:
		result.Insights = w.generateTechnicalInsights(request)
	}

	result.ResultCount = len(result.Insights)
	return result
}

// generateGitHubInsights generates GitHub-sourced insights
func (w *Weaver) generateGitHubInsights(request *ResearchRequest) []Insight {
	var insights []Insight

	// Check for common technology patterns in the request
	text := strings.ToLower(request.Vision + " " + request.Problem)

	if strings.Contains(text, "auth") {
		insights = append(insights, w.createInsight(
			InsightImplementation,
			"OAuth2/OIDC Implementation Patterns",
			"Popular implementations use established libraries (passport.js, auth0-spa-js) rather than rolling custom auth",
			"github.com/trending/auth",
			HunterGitHubScout,
			ConfidenceHigh,
			0.9,
			[]string{"auth", "oauth", "security"},
		))
	}

	if strings.Contains(text, "api") || strings.Contains(text, "rest") {
		insights = append(insights, w.createInsight(
			InsightPattern,
			"REST API Design Patterns",
			"Most successful APIs follow OpenAPI spec, use versioning (/v1/), and implement rate limiting",
			"github.com/search?q=rest+api+best+practices",
			HunterGitHubScout,
			ConfidenceHigh,
			0.85,
			[]string{"api", "rest", "design"},
		))
	}

	if strings.Contains(text, "real-time") || strings.Contains(text, "realtime") || strings.Contains(text, "websocket") {
		insights = append(insights, w.createInsight(
			InsightImplementation,
			"Real-time Communication Approaches",
			"WebSocket with fallback to SSE is the dominant pattern; Socket.io and Pusher are common choices",
			"github.com/trending/websocket",
			HunterGitHubScout,
			ConfidenceMedium,
			0.8,
			[]string{"realtime", "websocket", "sse"},
		))
	}

	// Default insight about OSS landscape
	insights = append(insights, w.createInsight(
		InsightOpportunity,
		"Open Source Ecosystem",
		"Multiple OSS solutions exist in this space - consider building on existing foundations rather than from scratch",
		"github.com/search",
		HunterGitHubScout,
		ConfidenceMedium,
		0.7,
		[]string{"oss", "foundation"},
	))

	return insights
}

// generateTrendInsights generates market trend insights
func (w *Weaver) generateTrendInsights(request *ResearchRequest) []Insight {
	var insights []Insight

	insights = append(insights, w.createInsight(
		InsightTrend,
		"Developer Experience Focus",
		"Industry trend toward prioritizing DX - clear docs, quick starts, and intuitive APIs are differentiators",
		"industry-analysis/dx-trends",
		HunterTrendWatcher,
		ConfidenceMedium,
		0.75,
		[]string{"dx", "developer-experience", "trend"},
	))

	text := strings.ToLower(request.Vision + " " + request.Problem)
	if strings.Contains(text, "ai") || strings.Contains(text, "ml") {
		insights = append(insights, w.createInsight(
			InsightTrend,
			"AI Integration Expectations",
			"Users increasingly expect AI-powered features; consider where ML can add value without over-engineering",
			"trend-analysis/ai-adoption",
			HunterTrendWatcher,
			ConfidenceHigh,
			0.85,
			[]string{"ai", "ml", "trend"},
		))
	}

	return insights
}

// generateCompetitorInsights generates competitor analysis insights
func (w *Weaver) generateCompetitorInsights(request *ResearchRequest) []Insight {
	var insights []Insight

	insights = append(insights, w.createInsight(
		InsightCompetitor,
		"Competitive Landscape Overview",
		"Several established players exist - differentiation through UX, pricing, or niche focus is recommended",
		"competitor-analysis/overview",
		HunterCompetitorTrack,
		ConfidenceMedium,
		0.7,
		[]string{"competitor", "differentiation"},
	))

	insights = append(insights, w.createInsight(
		InsightRisk,
		"Feature Parity Risk",
		"Avoid feature-by-feature competition with incumbents; focus on underserved use cases",
		"competitor-analysis/strategy",
		HunterCompetitorTrack,
		ConfidenceHigh,
		0.8,
		[]string{"risk", "strategy", "competition"},
	))

	return insights
}

// generateAcademicInsights generates academic/research insights
func (w *Weaver) generateAcademicInsights(request *ResearchRequest) []Insight {
	var insights []Insight

	text := strings.ToLower(request.Vision + " " + request.Problem)

	if strings.Contains(text, "search") || strings.Contains(text, "recommendation") {
		insights = append(insights, w.createInsight(
			InsightPattern,
			"Search & Recommendation Research",
			"Hybrid approaches (semantic + keyword) outperform single methods; consider embedding-based search",
			"arxiv.org/search",
			HunterAcademic,
			ConfidenceHigh,
			0.85,
			[]string{"search", "semantic", "embeddings"},
		))
	}

	if strings.Contains(text, "scale") || strings.Contains(text, "performance") {
		insights = append(insights, w.createInsight(
			InsightPattern,
			"Scalability Research",
			"Horizontal scaling with eventual consistency often preferred over strict consistency at scale",
			"research/distributed-systems",
			HunterAcademic,
			ConfidenceMedium,
			0.75,
			[]string{"scale", "distributed", "consistency"},
		))
	}

	return insights
}

// generateTechnicalInsights generates technical documentation insights
func (w *Weaver) generateTechnicalInsights(request *ResearchRequest) []Insight {
	var insights []Insight

	insights = append(insights, w.createInsight(
		InsightImplementation,
		"API Best Practices",
		"Use established standards: OpenAPI for docs, JSON:API or REST conventions, proper HTTP status codes",
		"developer-docs/api-standards",
		HunterTechnical,
		ConfidenceHigh,
		0.9,
		[]string{"api", "standards", "documentation"},
	))

	text := strings.ToLower(request.Vision + " " + request.Problem)
	if strings.Contains(text, "database") || strings.Contains(text, "storage") {
		insights = append(insights, w.createInsight(
			InsightImplementation,
			"Data Storage Considerations",
			"Choose storage based on access patterns: PostgreSQL for relational, Redis for caching, S3 for objects",
			"tech-docs/storage-patterns",
			HunterTechnical,
			ConfidenceHigh,
			0.85,
			[]string{"database", "storage", "patterns"},
		))
	}

	return insights
}

// createInsight creates a new insight with auto-incrementing ID
func (w *Weaver) createInsight(insightType InsightType, title, summary, source string, sourceType HunterType, confidence Confidence, relevance float64, tags []string) Insight {
	w.insightCounter++
	return Insight{
		ID:          fmt.Sprintf("INS-%03d", w.insightCounter),
		Type:        insightType,
		Title:       title,
		Summary:     summary,
		Source:      source,
		SourceType:  sourceType,
		Confidence:  confidence,
		Relevance:   relevance,
		Tags:        tags,
		ExtractedAt: time.Now(),
	}
}

// synthesizeInsights combines and deduplicates insights from all hunters
func (w *Weaver) synthesizeInsights(results []HunterResult) []Insight {
	var all []Insight
	seen := make(map[string]bool)

	for _, result := range results {
		for _, insight := range result.Insights {
			// Simple deduplication by title
			if !seen[insight.Title] {
				seen[insight.Title] = true
				all = append(all, insight)
			}
		}
	}

	// Sort by relevance
	sort.Slice(all, func(i, j int) bool {
		return all[i].Relevance > all[j].Relevance
	})

	return all
}

// identifyThemes finds recurring themes across insights
func (w *Weaver) identifyThemes(insights []Insight) []Theme {
	// Count tag occurrences
	tagCounts := make(map[string][]string) // tag -> insight IDs

	for _, insight := range insights {
		for _, tag := range insight.Tags {
			tagCounts[tag] = append(tagCounts[tag], insight.ID)
		}
	}

	// Create themes from frequently occurring tags
	var themes []Theme
	for tag, insightIDs := range tagCounts {
		if len(insightIDs) >= 2 { // Threshold for theme
			themes = append(themes, Theme{
				Name:        tag,
				Description: fmt.Sprintf("Multiple insights related to %s", tag),
				InsightIDs:  insightIDs,
				Frequency:   len(insightIDs),
			})
		}
	}

	// Sort by frequency
	sort.Slice(themes, func(i, j int) bool {
		return themes[i].Frequency > themes[j].Frequency
	})

	return themes
}

// generateRecommendations creates actionable recommendations from context
func (w *Weaver) generateRecommendations(context *WovenContext) []string {
	var recs []string

	// Based on insight types
	hasImplementation := false
	hasRisk := false
	hasCompetitor := false

	for _, insight := range context.SynthesizedInsights {
		switch insight.Type {
		case InsightImplementation:
			hasImplementation = true
		case InsightRisk:
			hasRisk = true
		case InsightCompetitor:
			hasCompetitor = true
		}
	}

	if hasImplementation {
		recs = append(recs, "Review existing implementations before designing custom solutions")
	}
	if hasRisk {
		recs = append(recs, "Address identified risks in the spec's acceptance criteria")
	}
	if hasCompetitor {
		recs = append(recs, "Define clear differentiation points from competitors")
	}

	// Based on themes
	if len(context.Themes) > 0 {
		recs = append(recs, fmt.Sprintf("Focus on key themes: %s", context.Themes[0].Name))
	}

	// Default recommendations
	if len(recs) == 0 {
		recs = []string{
			"Validate assumptions with target users before finalizing spec",
			"Consider phased rollout to gather feedback early",
		}
	}

	return recs
}

// FormatWovenContext formats the context as markdown for display
func FormatWovenContext(context *WovenContext) string {
	var sb strings.Builder

	sb.WriteString("# Research Context\n\n")
	sb.WriteString(fmt.Sprintf("**Query:** %s\n\n", context.Query))

	// Hunter summary
	sb.WriteString("## Research Sources\n\n")
	for _, result := range context.HunterResults {
		status := "✓"
		if result.Error != "" {
			status = "✗"
		}
		sb.WriteString(fmt.Sprintf("- %s **%s**: %d insights\n", status, result.Hunter, result.ResultCount))
	}
	sb.WriteString("\n")

	// Top insights
	sb.WriteString("## Key Insights\n\n")
	maxInsights := 5
	if len(context.SynthesizedInsights) < maxInsights {
		maxInsights = len(context.SynthesizedInsights)
	}
	for i := 0; i < maxInsights; i++ {
		insight := context.SynthesizedInsights[i]
		confidence := confidenceIcon(insight.Confidence)
		sb.WriteString(fmt.Sprintf("### %s %s\n\n", confidence, insight.Title))
		sb.WriteString(fmt.Sprintf("**Type:** %s | **Source:** %s | **Relevance:** %.0f%%\n\n", insight.Type, insight.SourceType, insight.Relevance*100))
		sb.WriteString(fmt.Sprintf("%s\n\n", insight.Summary))
	}

	// Themes
	if len(context.Themes) > 0 {
		sb.WriteString("## Recurring Themes\n\n")
		for _, theme := range context.Themes {
			sb.WriteString(fmt.Sprintf("- **%s** (appears in %d insights)\n", theme.Name, theme.Frequency))
		}
		sb.WriteString("\n")
	}

	// Recommendations
	sb.WriteString("## Recommendations\n\n")
	for _, rec := range context.Recommendations {
		sb.WriteString(fmt.Sprintf("- %s\n", rec))
	}

	return sb.String()
}

// FormatInsightsForInterview formats insights for display in Gurgeh interview
func FormatInsightsForInterview(insights []Insight) string {
	var sb strings.Builder

	sb.WriteString("📚 **Research Findings**\n\n")

	for _, insight := range insights {
		icon := typeIcon(insight.Type)
		sb.WriteString(fmt.Sprintf("%s **%s**\n", icon, insight.Title))
		sb.WriteString(fmt.Sprintf("   %s\n\n", insight.Summary))
	}

	return sb.String()
}

// BuildWeaverBrief creates an agent brief for research orchestration
func BuildWeaverBrief(request *ResearchRequest) string {
	var sb strings.Builder

	sb.WriteString("# Weaver Agent Brief: Research Orchestration\n\n")
	sb.WriteString("## Task\n")
	sb.WriteString("Orchestrate parallel research hunters and synthesize findings.\n\n")

	sb.WriteString("## Research Request\n")
	sb.WriteString(fmt.Sprintf("**Vision:** %s\n", request.Vision))
	sb.WriteString(fmt.Sprintf("**Problem:** %s\n\n", request.Problem))

	if len(request.Requirements) > 0 {
		sb.WriteString("### Requirements Context\n")
		for _, req := range request.Requirements {
			sb.WriteString(fmt.Sprintf("- %s\n", req))
		}
		sb.WriteString("\n")
	}

	if len(request.Keywords) > 0 {
		sb.WriteString(fmt.Sprintf("**Keywords:** %s\n\n", strings.Join(request.Keywords, ", ")))
	}

	sb.WriteString("## Hunter Types\n\n")
	sb.WriteString("- **github-scout**: Search OSS implementations\n")
	sb.WriteString("- **trend-watcher**: Track industry trends\n")
	sb.WriteString("- **competitor-track**: Analyze competitors\n")
	sb.WriteString("- **academic**: Research papers and studies\n")
	sb.WriteString("- **technical**: Technical documentation\n")

	return sb.String()
}

// --- Helper functions ---

func confidenceIcon(c Confidence) string {
	switch c {
	case ConfidenceHigh:
		return "🟢"
	case ConfidenceMedium:
		return "🟡"
	case ConfidenceLow:
		return "🔴"
	default:
		return "⚪"
	}
}

func typeIcon(t InsightType) string {
	switch t {
	case InsightImplementation:
		return "🔧"
	case InsightPattern:
		return "📐"
	case InsightRisk:
		return "⚠️"
	case InsightOpportunity:
		return "💡"
	case InsightCompetitor:
		return "🎯"
	case InsightTrend:
		return "📈"
	default:
		return "📌"
	}
}
