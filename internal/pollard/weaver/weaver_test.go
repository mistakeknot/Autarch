package weaver

import (
	"strings"
	"testing"
)

func TestWeaver_Weave_BasicRequest(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Build a task management app",
		Problem: "Users struggle to organize their work",
	}

	context := w.Weave(request)

	if context.Vision != request.Vision {
		t.Errorf("Vision = %s, want %s", context.Vision, request.Vision)
	}
	if context.Problem != request.Problem {
		t.Errorf("Problem = %s, want %s", context.Problem, request.Problem)
	}
	if len(context.HunterResults) == 0 {
		t.Error("expected hunter results")
	}
	if len(context.SynthesizedInsights) == 0 {
		t.Error("expected synthesized insights")
	}
}

func TestWeaver_SelectHunters_Default(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Simple app",
		Problem: "Basic problem",
	}

	hunters := w.selectHunters(request)

	// Should always include GitHub scout
	found := false
	for _, h := range hunters {
		if h == HunterGitHubScout {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected GitHub scout in default hunters")
	}
}

func TestWeaver_SelectHunters_Market(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Capture the market for project management",
		Problem: "Market is fragmented",
	}

	hunters := w.selectHunters(request)

	found := false
	for _, h := range hunters {
		if h == HunterTrendWatcher {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected trend watcher for market-related request")
	}
}

func TestWeaver_SelectHunters_Competitor(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Build a better alternative to Jira",
		Problem: "Jira is too complex",
	}

	hunters := w.selectHunters(request)

	found := false
	for _, h := range hunters {
		if h == HunterCompetitorTrack {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected competitor tracker for competitive request")
	}
}

func TestWeaver_SelectHunters_Academic(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Use machine learning for recommendations",
		Problem: "Need intelligent algorithm",
	}

	hunters := w.selectHunters(request)

	found := false
	for _, h := range hunters {
		if h == HunterAcademic {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected academic hunter for ML-related request")
	}
}

func TestWeaver_SelectHunters_Technical(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Build API integration layer",
		Problem: "Need to connect multiple frameworks",
	}

	hunters := w.selectHunters(request)

	found := false
	for _, h := range hunters {
		if h == HunterTechnical {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected technical hunter for API-related request")
	}
}

func TestWeaver_SelectHunters_Explicit(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Any vision",
		Problem: "Any problem",
		Hunters: []HunterType{HunterAcademic},
	}

	context := w.Weave(request)

	// Should only use the explicitly specified hunter
	if len(context.HunterResults) != 1 {
		t.Errorf("expected 1 hunter result, got %d", len(context.HunterResults))
	}
	if context.HunterResults[0].Hunter != HunterAcademic {
		t.Errorf("expected academic hunter, got %s", context.HunterResults[0].Hunter)
	}
}

func TestWeaver_GenerateGitHubInsights_Auth(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Build secure authentication system",
		Problem: "Need OAuth integration",
	}

	insights := w.generateGitHubInsights(request)

	foundAuth := false
	for _, insight := range insights {
		if strings.Contains(strings.ToLower(insight.Title), "oauth") ||
			strings.Contains(strings.ToLower(insight.Title), "auth") {
			foundAuth = true
			break
		}
	}
	if !foundAuth {
		t.Error("expected auth-related insight for auth request")
	}
}

func TestWeaver_GenerateGitHubInsights_API(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:  "Build REST API",
		Problem: "Need standard API patterns",
	}

	insights := w.generateGitHubInsights(request)

	foundAPI := false
	for _, insight := range insights {
		if strings.Contains(strings.ToLower(insight.Title), "api") {
			foundAPI = true
			break
		}
	}
	if !foundAPI {
		t.Error("expected API-related insight for API request")
	}
}

func TestWeaver_SynthesizeInsights_Deduplication(t *testing.T) {
	w := NewWeaver()

	// Create results with duplicate titles
	results := []HunterResult{
		{
			Hunter: HunterGitHubScout,
			Insights: []Insight{
				{ID: "INS-001", Title: "Same Title", Relevance: 0.9},
			},
		},
		{
			Hunter: HunterTechnical,
			Insights: []Insight{
				{ID: "INS-002", Title: "Same Title", Relevance: 0.8}, // Duplicate
				{ID: "INS-003", Title: "Different Title", Relevance: 0.7},
			},
		},
	}

	synthesized := w.synthesizeInsights(results)

	// Should have 2 unique insights
	if len(synthesized) != 2 {
		t.Errorf("expected 2 unique insights, got %d", len(synthesized))
	}
}

func TestWeaver_SynthesizeInsights_SortedByRelevance(t *testing.T) {
	w := NewWeaver()

	results := []HunterResult{
		{
			Hunter: HunterGitHubScout,
			Insights: []Insight{
				{ID: "INS-001", Title: "Low", Relevance: 0.3},
				{ID: "INS-002", Title: "High", Relevance: 0.9},
				{ID: "INS-003", Title: "Medium", Relevance: 0.6},
			},
		},
	}

	synthesized := w.synthesizeInsights(results)

	// Should be sorted by relevance descending
	if synthesized[0].Title != "High" {
		t.Errorf("expected 'High' first, got %s", synthesized[0].Title)
	}
	if synthesized[2].Title != "Low" {
		t.Errorf("expected 'Low' last, got %s", synthesized[2].Title)
	}
}

func TestWeaver_IdentifyThemes(t *testing.T) {
	w := NewWeaver()

	insights := []Insight{
		{ID: "INS-001", Tags: []string{"api", "design"}},
		{ID: "INS-002", Tags: []string{"api", "patterns"}},
		{ID: "INS-003", Tags: []string{"security"}},
	}

	themes := w.identifyThemes(insights)

	// Should identify "api" as a theme (appears 2+ times)
	foundAPI := false
	for _, theme := range themes {
		if theme.Name == "api" {
			foundAPI = true
			if theme.Frequency != 2 {
				t.Errorf("expected frequency 2 for 'api', got %d", theme.Frequency)
			}
			break
		}
	}
	if !foundAPI {
		t.Error("expected 'api' theme to be identified")
	}
}

func TestWeaver_GenerateRecommendations(t *testing.T) {
	w := NewWeaver()

	context := &WovenContext{
		SynthesizedInsights: []Insight{
			{Type: InsightImplementation},
			{Type: InsightRisk},
		},
		Themes: []Theme{
			{Name: "security", Frequency: 3},
		},
	}

	recs := w.generateRecommendations(context)

	if len(recs) == 0 {
		t.Error("expected recommendations")
	}

	// Should have implementation-based recommendation
	foundImpl := false
	for _, rec := range recs {
		if strings.Contains(rec, "implementation") || strings.Contains(rec, "existing") {
			foundImpl = true
			break
		}
	}
	if !foundImpl {
		t.Error("expected implementation-related recommendation")
	}
}

func TestWeaver_BuildQuery(t *testing.T) {
	w := NewWeaver()

	request := &ResearchRequest{
		Vision:   "Build app",
		Problem:  "Solve problem",
		Keywords: []string{"golang", "api"},
	}

	query := w.buildQuery(request)

	if !strings.Contains(query, "Build app") {
		t.Error("query should contain vision")
	}
	if !strings.Contains(query, "Solve problem") {
		t.Error("query should contain problem")
	}
	if !strings.Contains(query, "golang") {
		t.Error("query should contain keywords")
	}
}

func TestFormatWovenContext(t *testing.T) {
	context := &WovenContext{
		Query:   "test query",
		Vision:  "test vision",
		Problem: "test problem",
		HunterResults: []HunterResult{
			{Hunter: HunterGitHubScout, ResultCount: 3},
		},
		SynthesizedInsights: []Insight{
			{
				ID:         "INS-001",
				Type:       InsightImplementation,
				Title:      "Test Insight",
				Summary:    "Test summary",
				SourceType: HunterGitHubScout,
				Confidence: ConfidenceHigh,
				Relevance:  0.9,
			},
		},
		Themes: []Theme{
			{Name: "test-theme", Frequency: 2},
		},
		Recommendations: []string{"Test recommendation"},
	}

	output := FormatWovenContext(context)

	if !strings.Contains(output, "Research Context") {
		t.Error("should contain header")
	}
	if !strings.Contains(output, "test query") {
		t.Error("should contain query")
	}
	if !strings.Contains(output, "github-scout") {
		t.Error("should contain hunter name")
	}
	if !strings.Contains(output, "Test Insight") {
		t.Error("should contain insight title")
	}
	if !strings.Contains(output, "Recurring Themes") {
		t.Error("should contain themes section")
	}
	if !strings.Contains(output, "Recommendations") {
		t.Error("should contain recommendations section")
	}
}

func TestFormatInsightsForInterview(t *testing.T) {
	insights := []Insight{
		{
			Type:    InsightImplementation,
			Title:   "Test Insight",
			Summary: "Test summary",
		},
	}

	output := FormatInsightsForInterview(insights)

	if !strings.Contains(output, "Research Findings") {
		t.Error("should contain header")
	}
	if !strings.Contains(output, "Test Insight") {
		t.Error("should contain insight title")
	}
	if !strings.Contains(output, "Test summary") {
		t.Error("should contain insight summary")
	}
}

func TestBuildWeaverBrief(t *testing.T) {
	request := &ResearchRequest{
		Vision:       "Test vision",
		Problem:      "Test problem",
		Requirements: []string{"Req 1"},
		Keywords:     []string{"keyword1"},
	}

	brief := BuildWeaverBrief(request)

	if !strings.Contains(brief, "Weaver Agent Brief") {
		t.Error("should contain header")
	}
	if !strings.Contains(brief, "Test vision") {
		t.Error("should contain vision")
	}
	if !strings.Contains(brief, "Test problem") {
		t.Error("should contain problem")
	}
	if !strings.Contains(brief, "github-scout") {
		t.Error("should contain hunter types")
	}
}

func TestInsightTypes(t *testing.T) {
	// Verify all insight types have icons
	types := []InsightType{
		InsightImplementation,
		InsightPattern,
		InsightRisk,
		InsightOpportunity,
		InsightCompetitor,
		InsightTrend,
	}

	for _, it := range types {
		icon := typeIcon(it)
		if icon == "📌" { // Default icon
			t.Errorf("insight type %s should have a specific icon", it)
		}
	}
}

func TestConfidenceIcons(t *testing.T) {
	// Verify all confidence levels have icons
	levels := []Confidence{
		ConfidenceHigh,
		ConfidenceMedium,
		ConfidenceLow,
	}

	for _, level := range levels {
		icon := confidenceIcon(level)
		if icon == "⚪" { // Default icon
			t.Errorf("confidence level %s should have a specific icon", level)
		}
	}
}
