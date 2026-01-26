// Package performance provides the prophet subagent for predicting performance characteristics and budgets.
package performance

import (
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// PerformanceClass categorizes performance requirements
type PerformanceClass string

const (
	ClassRealtime    PerformanceClass = "realtime"    // <100ms
	ClassInteractive PerformanceClass = "interactive" // <500ms
	ClassBatch       PerformanceClass = "batch"       // >1s OK
)

// Budget represents a performance budget for a metric
type Budget struct {
	Metric      string           `yaml:"metric" json:"metric"`
	Target      string           `yaml:"target" json:"target"`
	Rationale   string           `yaml:"rationale" json:"rationale"`
	Class       PerformanceClass `yaml:"class" json:"class"`
	Measurement string           `yaml:"measurement" json:"measurement"` // How to measure
}

// PredictionConfidence represents confidence in a prediction
type PredictionConfidence string

const (
	ConfidenceHigh   PredictionConfidence = "high"
	ConfidenceMedium PredictionConfidence = "medium"
	ConfidenceLow    PredictionConfidence = "low"
)

// Prediction represents a performance prediction
type Prediction struct {
	Area       string               `yaml:"area" json:"area"`
	Concern    string               `yaml:"concern" json:"concern"`
	Impact     string               `yaml:"impact" json:"impact"`
	Confidence PredictionConfidence `yaml:"confidence" json:"confidence"`
	Suggestion string               `yaml:"suggestion" json:"suggestion"`
}

// ScalingConsideration represents a scaling concern
type ScalingConsideration struct {
	Component   string `yaml:"component" json:"component"`
	Bottleneck  string `yaml:"bottleneck" json:"bottleneck"`
	ScaleMethod string `yaml:"scale_method" json:"scale_method"` // horizontal, vertical, cache, shard
}

// PerformanceProfile represents the full performance analysis
type PerformanceProfile struct {
	SpecID        string                 `yaml:"spec_id" json:"spec_id"`
	Class         PerformanceClass       `yaml:"class" json:"class"`
	Budgets       []Budget               `yaml:"budgets" json:"budgets"`
	Predictions   []Prediction           `yaml:"predictions" json:"predictions"`
	Scaling       []ScalingConsideration `yaml:"scaling" json:"scaling"`
	Monitoring    []string               `yaml:"monitoring" json:"monitoring"`
}

// Prophet analyzes specs and predicts performance characteristics
type Prophet struct{}

// NewProphet creates a new Prophet instance
func NewProphet() *Prophet {
	return &Prophet{}
}

// Predict analyzes a spec and returns its performance profile
func (p *Prophet) Predict(spec *specs.Spec) *PerformanceProfile {
	profile := &PerformanceProfile{
		SpecID: spec.ID,
	}

	profile.Class = p.classifyPerformance(spec)
	profile.Budgets = p.generateBudgets(spec, profile.Class)
	profile.Predictions = p.makePredictions(spec)
	profile.Scaling = p.assessScaling(spec)
	profile.Monitoring = p.recommendMonitoring(spec, profile.Class)

	return profile
}

// PredictPRD predicts performance characteristics across all features
func (p *Prophet) PredictPRD(prd *specs.PRD) *PerformanceProfile {
	profile := &PerformanceProfile{
		SpecID: prd.ID,
	}

	// Find the most demanding performance class
	mostDemanding := ClassBatch
	for _, feature := range prd.Features {
		spec := &specs.Spec{
			ID:           feature.ID,
			Title:        feature.Title,
			Summary:      feature.Summary,
			Requirements: feature.Requirements,
		}
		class := p.classifyPerformance(spec)
		if classOrder(class) < classOrder(mostDemanding) {
			mostDemanding = class
		}

		// Aggregate predictions
		profile.Predictions = append(profile.Predictions, p.makePredictions(spec)...)
		profile.Scaling = append(profile.Scaling, p.assessScaling(spec)...)
	}

	profile.Class = mostDemanding
	profile.Budgets = p.generateBudgets(nil, mostDemanding)
	profile.Monitoring = p.recommendMonitoring(nil, mostDemanding)

	return profile
}

func classOrder(class PerformanceClass) int {
	switch class {
	case ClassRealtime:
		return 0
	case ClassInteractive:
		return 1
	case ClassBatch:
		return 2
	default:
		return 3
	}
}

func (p *Prophet) classifyPerformance(spec *specs.Spec) PerformanceClass {
	text := strings.ToLower(spec.Title + " " + spec.Summary + " " + strings.Join(spec.Requirements, " "))

	// Realtime indicators
	realtimePatterns := []string{
		"real-time", "realtime", "websocket", "live", "streaming",
		"instant", "push notification", "chat", "collaboration",
	}
	for _, pattern := range realtimePatterns {
		if strings.Contains(text, pattern) {
			return ClassRealtime
		}
	}

	// Batch indicators
	batchPatterns := []string{
		"batch", "background", "scheduled", "cron", "nightly",
		"report generation", "data export", "migration", "etl",
	}
	for _, pattern := range batchPatterns {
		if strings.Contains(text, pattern) {
			return ClassBatch
		}
	}

	// Default to interactive
	return ClassInteractive
}

func (p *Prophet) generateBudgets(spec *specs.Spec, class PerformanceClass) []Budget {
	var budgets []Budget

	switch class {
	case ClassRealtime:
		budgets = append(budgets,
			Budget{
				Metric:      "API Response Time (P95)",
				Target:      "100ms",
				Rationale:   "Realtime features require sub-100ms responses",
				Class:       ClassRealtime,
				Measurement: "Application Performance Monitoring (APM)",
			},
			Budget{
				Metric:      "WebSocket Latency",
				Target:      "50ms",
				Rationale:   "Push messages must feel instantaneous",
				Class:       ClassRealtime,
				Measurement: "WebSocket ping monitoring",
			},
		)
	case ClassInteractive:
		budgets = append(budgets,
			Budget{
				Metric:      "API Response Time (P95)",
				Target:      "300ms",
				Rationale:   "Interactive UIs need snappy responses",
				Class:       ClassInteractive,
				Measurement: "Application Performance Monitoring (APM)",
			},
			Budget{
				Metric:      "Page Load Time",
				Target:      "2s",
				Rationale:   "Core Web Vitals target",
				Class:       ClassInteractive,
				Measurement: "Real User Monitoring (RUM)",
			},
		)
	case ClassBatch:
		budgets = append(budgets,
			Budget{
				Metric:      "Job Completion Time",
				Target:      "SLA-dependent",
				Rationale:   "Batch jobs should complete within defined windows",
				Class:       ClassBatch,
				Measurement: "Job scheduler metrics",
			},
		)
	}

	// Always add these
	budgets = append(budgets,
		Budget{
			Metric:      "Error Rate",
			Target:      "<1%",
			Rationale:   "Standard reliability target",
			Class:       class,
			Measurement: "Error tracking (Sentry, DataDog)",
		},
		Budget{
			Metric:      "Availability",
			Target:      "99.9%",
			Rationale:   "Standard availability SLA",
			Class:       class,
			Measurement: "Uptime monitoring",
		},
	)

	// Database-specific budgets
	if spec != nil {
		text := strings.ToLower(strings.Join(spec.Requirements, " "))
		if strings.Contains(text, "database") || strings.Contains(text, "store") {
			budgets = append(budgets, Budget{
				Metric:      "Database Query Time (P95)",
				Target:      "50ms",
				Rationale:   "Slow queries cascade to poor UX",
				Class:       class,
				Measurement: "Query performance monitoring",
			})
		}
	}

	return budgets
}

func (p *Prophet) makePredictions(spec *specs.Spec) []Prediction {
	var predictions []Prediction
	text := strings.ToLower(spec.Title + " " + spec.Summary + " " + strings.Join(spec.Requirements, " "))

	// N+1 query risk
	if strings.Contains(text, "list") || strings.Contains(text, "all") || strings.Contains(text, "each") {
		predictions = append(predictions, Prediction{
			Area:       "Database",
			Concern:    "N+1 query pattern risk",
			Impact:     "Response time grows linearly with data size",
			Confidence: ConfidenceMedium,
			Suggestion: "Use eager loading or batch queries",
		})
	}

	// File upload concerns
	if strings.Contains(text, "upload") || strings.Contains(text, "file") || strings.Contains(text, "image") {
		predictions = append(predictions, Prediction{
			Area:       "Storage",
			Concern:    "Large file uploads may timeout",
			Impact:     "Poor UX for large files, potential data loss",
			Confidence: ConfidenceHigh,
			Suggestion: "Use chunked uploads with resume capability",
		})
	}

	// Search functionality
	if strings.Contains(text, "search") || strings.Contains(text, "filter") {
		predictions = append(predictions, Prediction{
			Area:       "Search",
			Concern:    "Full-text search at scale",
			Impact:     "Slow search degrades discoverability",
			Confidence: ConfidenceMedium,
			Suggestion: "Consider dedicated search engine (Elasticsearch, Meilisearch)",
		})
	}

	// Real-time features
	if strings.Contains(text, "real-time") || strings.Contains(text, "live") {
		predictions = append(predictions, Prediction{
			Area:       "Infrastructure",
			Concern:    "Connection scaling for real-time features",
			Impact:     "WebSocket connections consume server resources",
			Confidence: ConfidenceHigh,
			Suggestion: "Plan for horizontal scaling and connection limits",
		})
	}

	// Notification systems
	if strings.Contains(text, "notification") || strings.Contains(text, "alert") {
		predictions = append(predictions, Prediction{
			Area:       "Messaging",
			Concern:    "Notification delivery latency",
			Impact:     "Delayed notifications reduce engagement",
			Confidence: ConfidenceMedium,
			Suggestion: "Use message queue with priority handling",
		})
	}

	// Payment processing
	if strings.Contains(text, "payment") || strings.Contains(text, "checkout") {
		predictions = append(predictions, Prediction{
			Area:       "Payments",
			Concern:    "Third-party payment API latency",
			Impact:     "Slow checkout increases cart abandonment",
			Confidence: ConfidenceHigh,
			Suggestion: "Cache payment methods, async confirmation",
		})
	}

	return predictions
}

func (p *Prophet) assessScaling(spec *specs.Spec) []ScalingConsideration {
	var considerations []ScalingConsideration
	text := strings.ToLower(strings.Join(spec.Requirements, " "))

	// Database scaling
	if strings.Contains(text, "database") || strings.Contains(text, "store") {
		considerations = append(considerations, ScalingConsideration{
			Component:   "Database",
			Bottleneck:  "Write throughput and connection limits",
			ScaleMethod: "Read replicas, connection pooling, sharding",
		})
	}

	// API scaling
	if strings.Contains(text, "api") || strings.Contains(text, "endpoint") {
		considerations = append(considerations, ScalingConsideration{
			Component:   "API Layer",
			Bottleneck:  "Request concurrency",
			ScaleMethod: "Horizontal scaling with load balancer",
		})
	}

	// Cache scaling
	if strings.Contains(text, "cache") || strings.Contains(text, "session") {
		considerations = append(considerations, ScalingConsideration{
			Component:   "Cache",
			Bottleneck:  "Memory limits and cache coherence",
			ScaleMethod: "Distributed cache cluster (Redis Cluster)",
		})
	}

	// Background jobs
	if strings.Contains(text, "background") || strings.Contains(text, "async") || strings.Contains(text, "queue") {
		considerations = append(considerations, ScalingConsideration{
			Component:   "Background Workers",
			Bottleneck:  "Queue depth and worker throughput",
			ScaleMethod: "Auto-scaling workers based on queue depth",
		})
	}

	return considerations
}

func (p *Prophet) recommendMonitoring(spec *specs.Spec, class PerformanceClass) []string {
	monitoring := []string{
		"Response time percentiles (P50, P95, P99)",
		"Error rate by endpoint",
		"Request throughput",
	}

	switch class {
	case ClassRealtime:
		monitoring = append(monitoring,
			"WebSocket connection count",
			"Message latency distribution",
			"Connection drop rate",
		)
	case ClassInteractive:
		monitoring = append(monitoring,
			"Core Web Vitals (LCP, FID, CLS)",
			"Time to First Byte (TTFB)",
			"Client-side JavaScript errors",
		)
	case ClassBatch:
		monitoring = append(monitoring,
			"Job completion time",
			"Queue depth",
			"Failure/retry rate",
		)
	}

	if spec != nil {
		text := strings.ToLower(strings.Join(spec.Requirements, " "))
		if strings.Contains(text, "database") {
			monitoring = append(monitoring,
				"Database query time",
				"Connection pool utilization",
				"Slow query log",
			)
		}
	}

	return monitoring
}

// FormatPerformanceProfile formats a performance profile as readable text
func FormatPerformanceProfile(profile *PerformanceProfile) string {
	var sb strings.Builder

	sb.WriteString("# Performance Profile\n\n")
	sb.WriteString(fmt.Sprintf("**Spec:** %s\n", profile.SpecID))
	sb.WriteString(fmt.Sprintf("**Performance Class:** %s\n\n", classIcon(profile.Class)))

	if len(profile.Budgets) > 0 {
		sb.WriteString("## Performance Budgets\n\n")
		sb.WriteString("| Metric | Target | Measurement |\n")
		sb.WriteString("|--------|--------|-------------|\n")
		for _, budget := range profile.Budgets {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", budget.Metric, budget.Target, budget.Measurement))
		}
		sb.WriteString("\n")
	}

	if len(profile.Predictions) > 0 {
		sb.WriteString("## Performance Predictions\n\n")
		for _, pred := range profile.Predictions {
			sb.WriteString(fmt.Sprintf("### %s: %s\n", pred.Area, pred.Concern))
			sb.WriteString(fmt.Sprintf("- **Impact:** %s\n", pred.Impact))
			sb.WriteString(fmt.Sprintf("- **Confidence:** %s\n", pred.Confidence))
			sb.WriteString(fmt.Sprintf("- **Suggestion:** %s\n\n", pred.Suggestion))
		}
	}

	if len(profile.Scaling) > 0 {
		sb.WriteString("## Scaling Considerations\n\n")
		for _, scale := range profile.Scaling {
			sb.WriteString(fmt.Sprintf("### %s\n", scale.Component))
			sb.WriteString(fmt.Sprintf("- **Bottleneck:** %s\n", scale.Bottleneck))
			sb.WriteString(fmt.Sprintf("- **Scale Method:** %s\n\n", scale.ScaleMethod))
		}
	}

	if len(profile.Monitoring) > 0 {
		sb.WriteString("## Recommended Monitoring\n\n")
		for _, m := range profile.Monitoring {
			sb.WriteString(fmt.Sprintf("- %s\n", m))
		}
	}

	return sb.String()
}

// BuildProphetBrief generates an agent brief for performance analysis
func BuildProphetBrief(spec *specs.Spec) string {
	var sb strings.Builder

	sb.WriteString("# Prophet Agent Brief\n\n")
	sb.WriteString("## Mission\n")
	sb.WriteString("Predict performance characteristics and set appropriate budgets.\n\n")

	sb.WriteString("## Spec Context\n")
	sb.WriteString(fmt.Sprintf("- **ID:** %s\n", spec.ID))
	sb.WriteString(fmt.Sprintf("- **Title:** %s\n", spec.Title))
	sb.WriteString(fmt.Sprintf("- **Summary:** %s\n\n", spec.Summary))

	sb.WriteString("## Analysis Scope\n")
	sb.WriteString("1. **Performance Classification** - Realtime, Interactive, or Batch\n")
	sb.WriteString("2. **Budget Setting** - Response times, throughput, error rates\n")
	sb.WriteString("3. **Bottleneck Prediction** - Likely performance issues\n")
	sb.WriteString("4. **Scaling Assessment** - How components will need to scale\n")
	sb.WriteString("5. **Monitoring Recommendations** - What to measure\n")

	return sb.String()
}

func classIcon(class PerformanceClass) string {
	switch class {
	case ClassRealtime:
		return "⚡ Realtime"
	case ClassInteractive:
		return "🖱️ Interactive"
	case ClassBatch:
		return "📦 Batch"
	default:
		return "❓ Unknown"
	}
}

// Duration helpers for budget targets
func ParseBudgetDuration(target string) (time.Duration, error) {
	// Handle common formats
	target = strings.TrimSpace(strings.ToLower(target))
	target = strings.ReplaceAll(target, " ", "")
	return time.ParseDuration(target)
}
