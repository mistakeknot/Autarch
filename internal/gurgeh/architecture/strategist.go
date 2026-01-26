// Package architecture provides the strategist subagent for high-level architectural recommendations.
package architecture

import (
	"fmt"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// ArchitecturePattern represents a recommended architecture approach
type ArchitecturePattern string

const (
	PatternMonolith      ArchitecturePattern = "monolith"
	PatternMicroservices ArchitecturePattern = "microservices"
	PatternServerless    ArchitecturePattern = "serverless"
	PatternEventDriven   ArchitecturePattern = "event-driven"
	PatternLayered       ArchitecturePattern = "layered"
)

// DataStrategy represents the data storage approach
type DataStrategy string

const (
	DataRelational  DataStrategy = "relational"   // PostgreSQL, MySQL
	DataDocument    DataStrategy = "document"     // MongoDB, CouchDB
	DataKeyValue    DataStrategy = "key-value"    // Redis, DynamoDB
	DataGraph       DataStrategy = "graph"        // Neo4j, Neptune
	DataTimeSeries  DataStrategy = "time-series"  // InfluxDB, TimescaleDB
	DataPolyglot    DataStrategy = "polyglot"     // Multiple stores
)

// CachingApproach represents the caching strategy
type CachingApproach string

const (
	CacheNone        CachingApproach = "none"
	CacheInMemory    CachingApproach = "in-memory"    // Local process cache
	CacheDistributed CachingApproach = "distributed"  // Redis, Memcached
	CacheCDN         CachingApproach = "cdn"          // Edge caching
	CacheMultiTier   CachingApproach = "multi-tier"   // L1 + L2 caching
)

// APIStyle represents the API design approach
type APIStyle string

const (
	APIREST    APIStyle = "rest"
	APIGraphQL APIStyle = "graphql"
	APIGRPC    APIStyle = "grpc"
	APIHybrid  APIStyle = "hybrid" // Multiple styles
)

// ScalingStrategy represents how the system should scale
type ScalingStrategy string

const (
	ScaleVertical   ScalingStrategy = "vertical"   // Bigger machines
	ScaleHorizontal ScalingStrategy = "horizontal" // More machines
	ScaleAuto       ScalingStrategy = "auto"       // Dynamic scaling
)

// ArchitectureRecommendation represents a single recommendation
type ArchitectureRecommendation struct {
	Category    string   `yaml:"category" json:"category"`       // e.g., "Architecture Pattern"
	Recommended string   `yaml:"recommended" json:"recommended"` // The recommendation
	Rationale   string   `yaml:"rationale" json:"rationale"`     // Why this choice
	Tradeoffs   []string `yaml:"tradeoffs" json:"tradeoffs"`     // Pros/cons to consider
	Confidence  string   `yaml:"confidence" json:"confidence"`   // high, medium, low
}

// TechnologySuggestion represents a specific tech recommendation
type TechnologySuggestion struct {
	Category    string   `yaml:"category" json:"category"`       // e.g., "Database"
	Primary     string   `yaml:"primary" json:"primary"`         // Main recommendation
	Alternatives []string `yaml:"alternatives" json:"alternatives"` // Other options
	Rationale   string   `yaml:"rationale" json:"rationale"`
}

// ArchitectureStrategy represents the full architectural analysis
type ArchitectureStrategy struct {
	SpecID           string                       `yaml:"spec_id" json:"spec_id"`
	Pattern          ArchitecturePattern          `yaml:"pattern" json:"pattern"`
	DataStrategy     DataStrategy                 `yaml:"data_strategy" json:"data_strategy"`
	CachingApproach  CachingApproach              `yaml:"caching_approach" json:"caching_approach"`
	APIStyle         APIStyle                     `yaml:"api_style" json:"api_style"`
	ScalingStrategy  ScalingStrategy              `yaml:"scaling_strategy" json:"scaling_strategy"`
	Recommendations  []ArchitectureRecommendation `yaml:"recommendations" json:"recommendations"`
	Technologies     []TechnologySuggestion       `yaml:"technologies" json:"technologies"`
	Considerations   []string                     `yaml:"considerations" json:"considerations"`
}

// Strategist analyzes specs and recommends architectural approaches
type Strategist struct{}

// NewStrategist creates a new Strategist instance
func NewStrategist() *Strategist {
	return &Strategist{}
}

// Strategize analyzes a spec and returns architectural recommendations
func (s *Strategist) Strategize(spec *specs.Spec) *ArchitectureStrategy {
	strategy := &ArchitectureStrategy{
		SpecID: spec.ID,
	}

	strategy.Pattern = s.recommendPattern(spec)
	strategy.DataStrategy = s.recommendDataStrategy(spec)
	strategy.CachingApproach = s.recommendCaching(spec)
	strategy.APIStyle = s.recommendAPIStyle(spec)
	strategy.ScalingStrategy = s.recommendScaling(spec)
	strategy.Recommendations = s.generateRecommendations(spec, strategy)
	strategy.Technologies = s.suggestTechnologies(spec, strategy)
	strategy.Considerations = s.identifyConsiderations(spec, strategy)

	return strategy
}

// StrategizePRD analyzes a full PRD for architecture
func (s *Strategist) StrategizePRD(prd *specs.PRD) *ArchitectureStrategy {
	// Combine all features into a synthetic spec for analysis
	combined := &specs.Spec{
		ID:      prd.ID,
		Title:   prd.Title,
		Summary: prd.Title, // PRD doesn't have Description, use Title
	}
	for _, feature := range prd.Features {
		combined.Requirements = append(combined.Requirements, feature.Requirements...)
	}

	return s.Strategize(combined)
}

func (s *Strategist) recommendPattern(spec *specs.Spec) ArchitecturePattern {
	text := strings.ToLower(spec.Title + " " + spec.Summary + " " + strings.Join(spec.Requirements, " "))

	// Serverless indicators
	serverlessPatterns := []string{"lambda", "serverless", "functions", "event trigger", "scheduled"}
	for _, pattern := range serverlessPatterns {
		if strings.Contains(text, pattern) {
			return PatternServerless
		}
	}

	// Event-driven indicators
	eventPatterns := []string{"real-time", "realtime", "websocket", "streaming", "events", "pubsub", "message queue"}
	for _, pattern := range eventPatterns {
		if strings.Contains(text, pattern) {
			return PatternEventDriven
		}
	}

	// Microservices indicators
	microPatterns := []string{"microservice", "independent teams", "polyglot", "multiple services", "service mesh"}
	for _, pattern := range microPatterns {
		if strings.Contains(text, pattern) {
			return PatternMicroservices
		}
	}

	// Complexity indicators that suggest microservices
	complexityIndicators := 0
	if strings.Contains(text, "scale") {
		complexityIndicators++
	}
	if strings.Contains(text, "different database") || strings.Contains(text, "multiple data") {
		complexityIndicators++
	}
	if strings.Contains(text, "independent") || strings.Contains(text, "separate team") {
		complexityIndicators++
	}
	if complexityIndicators >= 2 {
		return PatternMicroservices
	}

	// Default to monolith for simplicity
	return PatternMonolith
}

func (s *Strategist) recommendDataStrategy(spec *specs.Spec) DataStrategy {
	text := strings.ToLower(strings.Join(spec.Requirements, " ") + " " + spec.Summary)

	// Graph database indicators
	if strings.Contains(text, "relationship") || strings.Contains(text, "connected") ||
		strings.Contains(text, "graph") || strings.Contains(text, "social network") {
		return DataGraph
	}

	// Time-series indicators
	if strings.Contains(text, "time series") || strings.Contains(text, "metrics") ||
		strings.Contains(text, "monitoring") || strings.Contains(text, "iot") {
		return DataTimeSeries
	}

	// Document store indicators
	if strings.Contains(text, "flexible schema") || strings.Contains(text, "unstructured") ||
		strings.Contains(text, "document") || strings.Contains(text, "json storage") {
		return DataDocument
	}

	// Key-value indicators
	if strings.Contains(text, "cache") || strings.Contains(text, "session") ||
		strings.Contains(text, "key-value") || strings.Contains(text, "high throughput") {
		return DataKeyValue
	}

	// Multiple data stores
	storeTypes := 0
	if strings.Contains(text, "relational") || strings.Contains(text, "sql") {
		storeTypes++
	}
	if strings.Contains(text, "search") || strings.Contains(text, "elasticsearch") {
		storeTypes++
	}
	if strings.Contains(text, "cache") || strings.Contains(text, "redis") {
		storeTypes++
	}
	if storeTypes >= 2 {
		return DataPolyglot
	}

	// Default to relational
	return DataRelational
}

func (s *Strategist) recommendCaching(spec *specs.Spec) CachingApproach {
	text := strings.ToLower(strings.Join(spec.Requirements, " ") + " " + spec.Summary)

	// CDN caching for static content
	if strings.Contains(text, "static content") || strings.Contains(text, "images") ||
		strings.Contains(text, "cdn") || strings.Contains(text, "global users") {
		return CacheCDN
	}

	// Multi-tier for high-traffic
	if strings.Contains(text, "high traffic") || strings.Contains(text, "millions of users") {
		return CacheMultiTier
	}

	// Distributed caching
	if strings.Contains(text, "cache") || strings.Contains(text, "session") ||
		strings.Contains(text, "frequently accessed") || strings.Contains(text, "redis") {
		return CacheDistributed
	}

	// Simple in-memory for small scale
	if strings.Contains(text, "small scale") || strings.Contains(text, "internal tool") {
		return CacheInMemory
	}

	// Default to distributed for most production systems
	if strings.Contains(text, "api") || strings.Contains(text, "database") {
		return CacheDistributed
	}

	return CacheNone
}

func (s *Strategist) recommendAPIStyle(spec *specs.Spec) APIStyle {
	text := strings.ToLower(strings.Join(spec.Requirements, " ") + " " + spec.Summary)

	// GraphQL indicators
	if strings.Contains(text, "graphql") || strings.Contains(text, "flexible queries") ||
		strings.Contains(text, "mobile app") || strings.Contains(text, "varying data needs") {
		return APIGraphQL
	}

	// gRPC indicators
	if strings.Contains(text, "grpc") || strings.Contains(text, "internal services") ||
		strings.Contains(text, "high performance") || strings.Contains(text, "protocol buffers") {
		return APIGRPC
	}

	// Hybrid for complex systems
	if strings.Contains(text, "public api") && strings.Contains(text, "internal") {
		return APIHybrid
	}

	// Default to REST
	return APIREST
}

func (s *Strategist) recommendScaling(spec *specs.Spec) ScalingStrategy {
	text := strings.ToLower(strings.Join(spec.Requirements, " ") + " " + spec.Summary)

	// Auto-scaling indicators
	if strings.Contains(text, "variable load") || strings.Contains(text, "burst") ||
		strings.Contains(text, "auto-scale") || strings.Contains(text, "elastic") {
		return ScaleAuto
	}

	// Vertical scaling for simpler systems
	if strings.Contains(text, "database intensive") || strings.Contains(text, "stateful") {
		return ScaleVertical
	}

	// Default to horizontal for most systems
	return ScaleHorizontal
}

func (s *Strategist) generateRecommendations(spec *specs.Spec, strategy *ArchitectureStrategy) []ArchitectureRecommendation {
	var recs []ArchitectureRecommendation

	// Architecture pattern recommendation
	recs = append(recs, ArchitectureRecommendation{
		Category:    "Architecture Pattern",
		Recommended: string(strategy.Pattern),
		Rationale:   s.patternRationale(strategy.Pattern),
		Tradeoffs:   s.patternTradeoffs(strategy.Pattern),
		Confidence:  "medium",
	})

	// Data strategy recommendation
	recs = append(recs, ArchitectureRecommendation{
		Category:    "Data Strategy",
		Recommended: string(strategy.DataStrategy),
		Rationale:   s.dataRationale(strategy.DataStrategy),
		Tradeoffs:   s.dataTradeoffs(strategy.DataStrategy),
		Confidence:  "medium",
	})

	// API style recommendation
	recs = append(recs, ArchitectureRecommendation{
		Category:    "API Design",
		Recommended: string(strategy.APIStyle),
		Rationale:   s.apiRationale(strategy.APIStyle),
		Tradeoffs:   s.apiTradeoffs(strategy.APIStyle),
		Confidence:  "medium",
	})

	return recs
}

func (s *Strategist) suggestTechnologies(spec *specs.Spec, strategy *ArchitectureStrategy) []TechnologySuggestion {
	var techs []TechnologySuggestion

	// Database suggestions based on data strategy
	switch strategy.DataStrategy {
	case DataRelational:
		techs = append(techs, TechnologySuggestion{
			Category:     "Database",
			Primary:      "PostgreSQL",
			Alternatives: []string{"MySQL", "SQLite (dev/testing)"},
			Rationale:    "Battle-tested, feature-rich, excellent ecosystem",
		})
	case DataDocument:
		techs = append(techs, TechnologySuggestion{
			Category:     "Database",
			Primary:      "MongoDB",
			Alternatives: []string{"CouchDB", "Amazon DocumentDB"},
			Rationale:    "Flexible schema, good for evolving data models",
		})
	case DataGraph:
		techs = append(techs, TechnologySuggestion{
			Category:     "Database",
			Primary:      "Neo4j",
			Alternatives: []string{"Amazon Neptune", "ArangoDB"},
			Rationale:    "Purpose-built for relationship-heavy data",
		})
	case DataTimeSeries:
		techs = append(techs, TechnologySuggestion{
			Category:     "Database",
			Primary:      "TimescaleDB",
			Alternatives: []string{"InfluxDB", "Prometheus"},
			Rationale:    "Optimized for time-series workloads",
		})
	}

	// Caching suggestions
	if strategy.CachingApproach != CacheNone {
		switch strategy.CachingApproach {
		case CacheDistributed, CacheMultiTier:
			techs = append(techs, TechnologySuggestion{
				Category:     "Caching",
				Primary:      "Redis",
				Alternatives: []string{"Memcached", "KeyDB"},
				Rationale:    "Fast, versatile, rich data structures",
			})
		case CacheCDN:
			techs = append(techs, TechnologySuggestion{
				Category:     "CDN",
				Primary:      "Cloudflare",
				Alternatives: []string{"AWS CloudFront", "Fastly"},
				Rationale:    "Global edge caching, DDoS protection",
			})
		}
	}

	// Framework suggestions based on pattern
	switch strategy.Pattern {
	case PatternMonolith:
		techs = append(techs, TechnologySuggestion{
			Category:     "Framework",
			Primary:      "Ruby on Rails / Django",
			Alternatives: []string{"Laravel", "Spring Boot", "ASP.NET Core"},
			Rationale:    "Batteries-included, rapid development",
		})
	case PatternMicroservices:
		techs = append(techs, TechnologySuggestion{
			Category:     "Framework",
			Primary:      "Go + gRPC",
			Alternatives: []string{"Node.js + Express", "Spring Boot"},
			Rationale:    "Lightweight, fast, good for services",
		})
	case PatternServerless:
		techs = append(techs, TechnologySuggestion{
			Category:     "Platform",
			Primary:      "AWS Lambda",
			Alternatives: []string{"Vercel Functions", "Cloudflare Workers"},
			Rationale:    "Pay-per-use, auto-scaling, zero ops",
		})
	case PatternEventDriven:
		techs = append(techs, TechnologySuggestion{
			Category:     "Message Broker",
			Primary:      "Apache Kafka",
			Alternatives: []string{"RabbitMQ", "Amazon SQS/SNS"},
			Rationale:    "High throughput, durable event streaming",
		})
	}

	return techs
}

func (s *Strategist) identifyConsiderations(spec *specs.Spec, strategy *ArchitectureStrategy) []string {
	var considerations []string
	text := strings.ToLower(strings.Join(spec.Requirements, " ") + " " + spec.Summary)

	// Pattern-specific considerations
	switch strategy.Pattern {
	case PatternMicroservices:
		considerations = append(considerations,
			"Plan for service discovery and registry",
			"Consider distributed tracing (Jaeger, Zipkin)",
			"Design for eventual consistency",
		)
	case PatternServerless:
		considerations = append(considerations,
			"Cold start latency may impact user experience",
			"Function timeouts limit long-running operations",
			"Vendor lock-in considerations",
		)
	case PatternEventDriven:
		considerations = append(considerations,
			"Ensure idempotent event handlers",
			"Plan for event ordering and deduplication",
			"Consider event schema evolution strategy",
		)
	}

	// Security considerations
	if strings.Contains(text, "user data") || strings.Contains(text, "authentication") {
		considerations = append(considerations,
			"Implement proper authentication/authorization (OAuth 2.0, JWT)",
			"Encrypt sensitive data at rest and in transit",
		)
	}

	// Compliance considerations
	if strings.Contains(text, "payment") || strings.Contains(text, "financial") {
		considerations = append(considerations, "PCI-DSS compliance requirements")
	}
	if strings.Contains(text, "health") || strings.Contains(text, "medical") {
		considerations = append(considerations, "HIPAA compliance requirements")
	}
	if strings.Contains(text, "european") || strings.Contains(text, "gdpr") {
		considerations = append(considerations, "GDPR compliance requirements")
	}

	return considerations
}

func (s *Strategist) patternRationale(pattern ArchitecturePattern) string {
	switch pattern {
	case PatternMonolith:
		return "Start simple, split later. Monoliths are easier to develop, test, and deploy initially."
	case PatternMicroservices:
		return "Scale teams and components independently. Enables polyglot tech choices."
	case PatternServerless:
		return "Minimize operational overhead. Pay only for actual usage."
	case PatternEventDriven:
		return "Decouple components through events. Excellent for real-time features."
	case PatternLayered:
		return "Clear separation of concerns. Well-understood pattern with mature tooling."
	default:
		return "Selected based on requirements analysis"
	}
}

func (s *Strategist) patternTradeoffs(pattern ArchitecturePattern) []string {
	switch pattern {
	case PatternMonolith:
		return []string{
			"+ Simple deployment and debugging",
			"+ Easier data consistency",
			"- Harder to scale individual components",
			"- Tech stack locked across entire app",
		}
	case PatternMicroservices:
		return []string{
			"+ Independent scaling and deployment",
			"+ Team autonomy",
			"- Distributed systems complexity",
			"- Network latency between services",
		}
	case PatternServerless:
		return []string{
			"+ Zero infrastructure management",
			"+ Cost-effective for variable load",
			"- Cold start latency",
			"- Vendor lock-in risk",
		}
	case PatternEventDriven:
		return []string{
			"+ Loose coupling between components",
			"+ Excellent for real-time features",
			"- Eventual consistency challenges",
			"- Harder to debug and trace",
		}
	default:
		return []string{}
	}
}

func (s *Strategist) dataRationale(strategy DataStrategy) string {
	switch strategy {
	case DataRelational:
		return "ACID guarantees, mature tooling, well-understood query patterns."
	case DataDocument:
		return "Flexible schema evolution, natural fit for JSON-centric apps."
	case DataGraph:
		return "Optimized for relationship traversal and connected data."
	case DataTimeSeries:
		return "Efficient storage and queries for time-stamped data."
	case DataKeyValue:
		return "Ultra-fast reads/writes for simple access patterns."
	case DataPolyglot:
		return "Use the right tool for each data type and access pattern."
	default:
		return "Selected based on data requirements"
	}
}

func (s *Strategist) dataTradeoffs(strategy DataStrategy) []string {
	switch strategy {
	case DataRelational:
		return []string{
			"+ Strong consistency and transactions",
			"+ Rich query capabilities",
			"- Schema changes require migrations",
			"- Horizontal scaling is complex",
		}
	case DataDocument:
		return []string{
			"+ Schema flexibility",
			"+ Easy horizontal scaling",
			"- Limited join capabilities",
			"- Potential data duplication",
		}
	case DataPolyglot:
		return []string{
			"+ Best tool for each use case",
			"+ Optimized performance",
			"- Operational complexity",
			"- Data synchronization challenges",
		}
	default:
		return []string{}
	}
}

func (s *Strategist) apiRationale(style APIStyle) string {
	switch style {
	case APIREST:
		return "Industry standard, cacheable, well-understood by all developers."
	case APIGraphQL:
		return "Flexible queries reduce over-fetching, great for mobile clients."
	case APIGRPC:
		return "High performance with protocol buffers, excellent for service-to-service."
	case APIHybrid:
		return "REST for public APIs, gRPC for internal services - best of both worlds."
	default:
		return "Selected based on API requirements"
	}
}

func (s *Strategist) apiTradeoffs(style APIStyle) []string {
	switch style {
	case APIREST:
		return []string{
			"+ Universal client support",
			"+ HTTP caching",
			"- Multiple round trips for complex data",
			"- Over/under-fetching",
		}
	case APIGraphQL:
		return []string{
			"+ Single request for complex data",
			"+ Client-driven queries",
			"- Learning curve",
			"- Complex caching",
		}
	case APIGRPC:
		return []string{
			"+ Excellent performance",
			"+ Strong typing via protobuf",
			"- Browser support limited",
			"- Binary format harder to debug",
		}
	default:
		return []string{}
	}
}

// FormatArchitectureStrategy formats a strategy as readable text
func FormatArchitectureStrategy(strategy *ArchitectureStrategy) string {
	var sb strings.Builder

	sb.WriteString("# Architecture Strategy\n\n")
	sb.WriteString(fmt.Sprintf("**Spec:** %s\n\n", strategy.SpecID))

	sb.WriteString("## Overview\n\n")
	sb.WriteString(fmt.Sprintf("| Aspect | Recommendation |\n"))
	sb.WriteString(fmt.Sprintf("|--------|----------------|\n"))
	sb.WriteString(fmt.Sprintf("| Pattern | %s %s |\n", patternIcon(strategy.Pattern), strategy.Pattern))
	sb.WriteString(fmt.Sprintf("| Data Strategy | %s |\n", strategy.DataStrategy))
	sb.WriteString(fmt.Sprintf("| Caching | %s |\n", strategy.CachingApproach))
	sb.WriteString(fmt.Sprintf("| API Style | %s |\n", strategy.APIStyle))
	sb.WriteString(fmt.Sprintf("| Scaling | %s |\n", strategy.ScalingStrategy))
	sb.WriteString("\n")

	if len(strategy.Recommendations) > 0 {
		sb.WriteString("## Recommendations\n\n")
		for _, rec := range strategy.Recommendations {
			sb.WriteString(fmt.Sprintf("### %s: %s\n", rec.Category, rec.Recommended))
			sb.WriteString(fmt.Sprintf("*%s*\n\n", rec.Rationale))
			if len(rec.Tradeoffs) > 0 {
				sb.WriteString("**Tradeoffs:**\n")
				for _, t := range rec.Tradeoffs {
					sb.WriteString(fmt.Sprintf("- %s\n", t))
				}
				sb.WriteString("\n")
			}
		}
	}

	if len(strategy.Technologies) > 0 {
		sb.WriteString("## Technology Suggestions\n\n")
		for _, tech := range strategy.Technologies {
			sb.WriteString(fmt.Sprintf("### %s\n", tech.Category))
			sb.WriteString(fmt.Sprintf("**Primary:** %s\n", tech.Primary))
			if len(tech.Alternatives) > 0 {
				sb.WriteString(fmt.Sprintf("**Alternatives:** %s\n", strings.Join(tech.Alternatives, ", ")))
			}
			sb.WriteString(fmt.Sprintf("*%s*\n\n", tech.Rationale))
		}
	}

	if len(strategy.Considerations) > 0 {
		sb.WriteString("## Key Considerations\n\n")
		for _, c := range strategy.Considerations {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	return sb.String()
}

// BuildStrategistBrief generates an agent brief for architectural analysis
func BuildStrategistBrief(spec *specs.Spec) string {
	var sb strings.Builder

	sb.WriteString("# Strategist Agent Brief\n\n")
	sb.WriteString("## Mission\n")
	sb.WriteString("Recommend high-level architectural decisions for the given specification.\n\n")

	sb.WriteString("## Spec Context\n")
	sb.WriteString(fmt.Sprintf("- **ID:** %s\n", spec.ID))
	sb.WriteString(fmt.Sprintf("- **Title:** %s\n", spec.Title))
	sb.WriteString(fmt.Sprintf("- **Summary:** %s\n\n", spec.Summary))

	sb.WriteString("## Analysis Scope\n")
	sb.WriteString("1. **Architecture Pattern** - Monolith, Microservices, Serverless, Event-Driven\n")
	sb.WriteString("2. **Data Strategy** - Relational, Document, Graph, Time-Series, Polyglot\n")
	sb.WriteString("3. **API Design** - REST, GraphQL, gRPC, Hybrid\n")
	sb.WriteString("4. **Caching Approach** - None, In-Memory, Distributed, CDN, Multi-Tier\n")
	sb.WriteString("5. **Scaling Strategy** - Vertical, Horizontal, Auto-scaling\n")

	return sb.String()
}

func patternIcon(pattern ArchitecturePattern) string {
	switch pattern {
	case PatternMonolith:
		return "🏛️"
	case PatternMicroservices:
		return "🔗"
	case PatternServerless:
		return "☁️"
	case PatternEventDriven:
		return "⚡"
	case PatternLayered:
		return "📚"
	default:
		return "❓"
	}
}
