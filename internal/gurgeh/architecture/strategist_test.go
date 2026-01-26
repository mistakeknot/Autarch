package architecture

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestStrategist_Strategize_RecommendsMonolith(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Blog Platform",
		Summary:      "Simple blog with posts and comments",
		Requirements: []string{"Create posts", "Add comments", "User profiles"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.Pattern != PatternMonolith {
		t.Errorf("pattern = %v, want monolith for simple app", strategy.Pattern)
	}
}

func TestStrategist_Strategize_RecommendsMicroservices(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "E-commerce Platform",
		Summary:      "Large scale platform with multiple services and independent teams",
		Requirements: []string{"Microservice architecture", "Independent deployments", "Multiple data stores"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.Pattern != PatternMicroservices {
		t.Errorf("pattern = %v, want microservices", strategy.Pattern)
	}
}

func TestStrategist_Strategize_RecommendsServerless(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Image Processing",
		Summary:      "Process images with lambda functions",
		Requirements: []string{"Run serverless functions", "Process on upload"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.Pattern != PatternServerless {
		t.Errorf("pattern = %v, want serverless", strategy.Pattern)
	}
}

func TestStrategist_Strategize_RecommendsEventDriven(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Real-time Dashboard",
		Summary:      "Live streaming data dashboard with WebSocket",
		Requirements: []string{"Real-time updates", "WebSocket connections"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.Pattern != PatternEventDriven {
		t.Errorf("pattern = %v, want event-driven", strategy.Pattern)
	}
}

func TestStrategist_Strategize_RecommendsRelationalData(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Inventory System",
		Summary:      "Track inventory with SQL database",
		Requirements: []string{"Store products", "Track orders"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.DataStrategy != DataRelational {
		t.Errorf("data strategy = %v, want relational", strategy.DataStrategy)
	}
}

func TestStrategist_Strategize_RecommendsDocumentStore(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Content Management",
		Summary:      "CMS with flexible schema and document storage",
		Requirements: []string{"Flexible schema for content types", "JSON storage for unstructured data"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.DataStrategy != DataDocument {
		t.Errorf("data strategy = %v, want document", strategy.DataStrategy)
	}
}

func TestStrategist_Strategize_RecommendsGraphDB(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Social Network",
		Summary:      "Social network with complex relationships",
		Requirements: []string{"Model user relationships", "Find connected users"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.DataStrategy != DataGraph {
		t.Errorf("data strategy = %v, want graph", strategy.DataStrategy)
	}
}

func TestStrategist_Strategize_RecommendsTimeSeries(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Metrics Dashboard",
		Summary:      "Store and query time series metrics data",
		Requirements: []string{"Collect metrics", "Query time series data"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.DataStrategy != DataTimeSeries {
		t.Errorf("data strategy = %v, want time-series", strategy.DataStrategy)
	}
}

func TestStrategist_Strategize_RecommendsDistributedCache(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "API Gateway",
		Summary:      "API with caching layer",
		Requirements: []string{"Cache frequently accessed data", "Reduce database load"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.CachingApproach != CacheDistributed {
		t.Errorf("caching = %v, want distributed", strategy.CachingApproach)
	}
}

func TestStrategist_Strategize_RecommendsCDN(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Media Platform",
		Summary:      "Serve static content to global users",
		Requirements: []string{"Serve images and videos", "CDN for global distribution"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.CachingApproach != CacheCDN {
		t.Errorf("caching = %v, want CDN", strategy.CachingApproach)
	}
}

func TestStrategist_Strategize_RecommendsREST(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Public API",
		Summary:      "RESTful API for third-party integrations",
		Requirements: []string{"REST endpoints", "API documentation"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.APIStyle != APIREST {
		t.Errorf("api style = %v, want REST", strategy.APIStyle)
	}
}

func TestStrategist_Strategize_RecommendsGraphQL(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Mobile Backend",
		Summary:      "Backend for mobile app with flexible queries",
		Requirements: []string{"Support mobile app", "Flexible queries for varying data needs"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.APIStyle != APIGraphQL {
		t.Errorf("api style = %v, want GraphQL", strategy.APIStyle)
	}
}

func TestStrategist_Strategize_RecommendsGRPC(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Internal Services",
		Summary:      "High performance internal services communication",
		Requirements: []string{"Internal services using gRPC", "Protocol buffers for efficiency"},
	}

	strategy := strategist.Strategize(spec)

	if strategy.APIStyle != APIGRPC {
		t.Errorf("api style = %v, want gRPC", strategy.APIStyle)
	}
}

func TestStrategist_Strategize_GeneratesRecommendations(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Web App",
		Summary:      "Standard web application",
		Requirements: []string{"User interface", "Database storage"},
	}

	strategy := strategist.Strategize(spec)

	if len(strategy.Recommendations) == 0 {
		t.Error("expected recommendations")
	}

	// Should have architecture pattern recommendation
	foundPattern := false
	for _, rec := range strategy.Recommendations {
		if rec.Category == "Architecture Pattern" {
			foundPattern = true
			if rec.Rationale == "" {
				t.Error("recommendation should have rationale")
			}
			break
		}
	}
	if !foundPattern {
		t.Error("expected architecture pattern recommendation")
	}
}

func TestStrategist_Strategize_SuggestsTechnologies(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "API Service",
		Summary:      "REST API with PostgreSQL",
		Requirements: []string{"REST endpoints", "Store data in database"},
	}

	strategy := strategist.Strategize(spec)

	if len(strategy.Technologies) == 0 {
		t.Error("expected technology suggestions")
	}

	// Should suggest database technology
	foundDB := false
	for _, tech := range strategy.Technologies {
		if tech.Category == "Database" {
			foundDB = true
			if tech.Primary == "" {
				t.Error("tech suggestion should have primary recommendation")
			}
			break
		}
	}
	if !foundDB {
		t.Error("expected database technology suggestion")
	}
}

func TestStrategist_Strategize_IdentifiesSecurityConsiderations(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Auth",
		Summary:      "User authentication system",
		Requirements: []string{"Store user data", "Handle authentication"},
	}

	strategy := strategist.Strategize(spec)

	foundSecurity := false
	for _, c := range strategy.Considerations {
		if strings.Contains(strings.ToLower(c), "auth") || strings.Contains(strings.ToLower(c), "encrypt") {
			foundSecurity = true
			break
		}
	}
	if !foundSecurity {
		t.Error("expected security considerations for auth system")
	}
}

func TestStrategist_Strategize_IdentifiesComplianceRequirements(t *testing.T) {
	strategist := NewStrategist()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Payment Processing",
		Summary:      "Process credit card payments",
		Requirements: []string{"Accept payment information", "Store financial data"},
	}

	strategy := strategist.Strategize(spec)

	foundPCI := false
	for _, c := range strategy.Considerations {
		if strings.Contains(c, "PCI") {
			foundPCI = true
			break
		}
	}
	if !foundPCI {
		t.Error("expected PCI compliance consideration for payment processing")
	}
}

func TestStrategist_StrategizePRD_AggregatesFeatures(t *testing.T) {
	strategist := NewStrategist()

	prd := &specs.PRD{
		ID:    "MVP",
		Title: "MVP Release",
		Features: []specs.Feature{
			{
				ID:           "FEAT-001",
				Title:        "Live Chat",
				Requirements: []string{"Real-time messaging", "WebSocket connections"},
			},
			{
				ID:           "FEAT-002",
				Title:        "User Profiles",
				Requirements: []string{"Store user data"},
			},
		},
	}

	strategy := strategist.StrategizePRD(prd)

	// Should detect event-driven from live chat feature
	if strategy.Pattern != PatternEventDriven {
		t.Errorf("pattern = %v, want event-driven (from live chat)", strategy.Pattern)
	}
}

func TestFormatArchitectureStrategy(t *testing.T) {
	strategy := &ArchitectureStrategy{
		SpecID:          "SPEC-001",
		Pattern:         PatternMicroservices,
		DataStrategy:    DataRelational,
		CachingApproach: CacheDistributed,
		APIStyle:        APIREST,
		ScalingStrategy: ScaleHorizontal,
		Recommendations: []ArchitectureRecommendation{
			{Category: "Architecture Pattern", Recommended: "microservices", Rationale: "Scale independently"},
		},
		Technologies: []TechnologySuggestion{
			{Category: "Database", Primary: "PostgreSQL", Alternatives: []string{"MySQL"}},
		},
		Considerations: []string{"Consider distributed tracing"},
	}

	output := FormatArchitectureStrategy(strategy)

	if !strings.Contains(output, "Architecture Strategy") {
		t.Error("should contain report header")
	}
	if !strings.Contains(output, "microservices") {
		t.Error("should contain pattern")
	}
	if !strings.Contains(output, "PostgreSQL") {
		t.Error("should contain technology suggestion")
	}
	if !strings.Contains(output, "distributed tracing") {
		t.Error("should contain considerations")
	}
}

func TestBuildStrategistBrief(t *testing.T) {
	spec := &specs.Spec{
		ID:      "SPEC-001",
		Title:   "Test Feature",
		Summary: "A test feature",
	}

	brief := BuildStrategistBrief(spec)

	if !strings.Contains(brief, "Strategist Agent Brief") {
		t.Error("should contain brief header")
	}
	if !strings.Contains(brief, "Architecture Pattern") {
		t.Error("should reference architecture patterns")
	}
	if !strings.Contains(brief, "Data Strategy") {
		t.Error("should reference data strategy")
	}
}

func TestPatternIcons(t *testing.T) {
	patterns := []ArchitecturePattern{PatternMonolith, PatternMicroservices, PatternServerless, PatternEventDriven, PatternLayered}
	for _, pattern := range patterns {
		icon := patternIcon(pattern)
		if icon == "❓" {
			t.Errorf("pattern %s should have specific icon", pattern)
		}
	}
}
