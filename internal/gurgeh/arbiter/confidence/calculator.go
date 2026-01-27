package confidence

// Score holds confidence metrics.
type Score struct {
	Completeness float64
	Consistency  float64
	Specificity  float64
	Research     float64
	Assumptions  float64
}

// Calculator computes confidence scores.
type Calculator struct{}

// NewCalculator creates a new Calculator.
func NewCalculator() *Calculator {
	return &Calculator{}
}

// Calculate computes a confidence score from section stats.
func (c *Calculator) Calculate(totalPhases, acceptedPhases, conflictCount int, hasResearch bool) Score {
	if totalPhases <= 0 {
		return Score{}
	}

	completeness := float64(acceptedPhases) / float64(totalPhases)

	consistency := 1.0
	if conflictCount > 0 {
		consistency = 1.0 / float64(1+conflictCount)
	}

	research := 0.5
	if hasResearch {
		research = 1.0
	}

	return Score{
		Completeness: completeness,
		Consistency:  consistency,
		Specificity:  0.5,
		Research:     research,
		Assumptions:  0.5,
	}
}
