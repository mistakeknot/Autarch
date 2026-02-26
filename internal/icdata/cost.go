package icdata

// CostBaseline holds the cost-per-landable-change baseline from ic cost baseline.
type CostBaseline struct {
	Period       CostPeriod                `json:"period"`
	ShippedBeads int                       `json:"shipped_beads"`
	Stats        CostTokenStats            `json:"stats"`
	ByPhase      map[string]CostTokenStats `json:"by_phase,omitempty"`
	ByAgent      map[string]CostTokenStats `json:"by_agent,omitempty"`
}

// CostPeriod describes the time window of the baseline query.
type CostPeriod struct {
	Days  int    `json:"days"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// CostTokenStats holds percentile and aggregate token statistics.
type CostTokenStats struct {
	P50         int64 `json:"p50"`
	P90         int64 `json:"p90"`
	P95         int64 `json:"p95"`
	Mean        int64 `json:"mean"`
	Total       int64 `json:"total"`
	InputTotal  int64 `json:"input_total"`
	OutputTotal int64 `json:"output_total"`
	Count       int   `json:"count"`
}
