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
// researchQuality should be 0.0 (no research) to 1.0 (high-quality findings).
func (c *Calculator) Calculate(totalPhases, acceptedPhases, conflictCount int, researchQuality float64) Score {
	if totalPhases <= 0 {
		return Score{}
	}

	completeness := float64(acceptedPhases) / float64(totalPhases)

	consistency := 1.0
	if conflictCount > 0 {
		consistency = 1.0 / float64(1+conflictCount)
	}

	// Clamp research quality to [0, 1]
	research := researchQuality
	if research < 0 {
		research = 0
	}
	if research > 1 {
		research = 1
	}

	return Score{
		Completeness: completeness,
		Consistency:  consistency,
		Specificity:  0.5,
		Research:     research,
		Assumptions:  0.5,
	}
}
