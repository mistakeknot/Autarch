package confidence

import (
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
	"github.com/mistakeknot/autarch/pkg/thinking"
)

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
// shapesUsed maps phase names to the thinking shape used; nil preserves legacy behavior.
func (c *Calculator) Calculate(totalPhases, acceptedPhases, conflictCount int, researchQuality float64, shapesUsed map[string]thinking.Shape) Score {
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

	// Shape-aware specificity: structured thinking produces more specific output
	specificity := 0.5
	if shapesUsed != nil {
		for _, shape := range shapesUsed {
			if shape == thinking.ShapeDeductive || shape == thinking.ShapeDSL {
				specificity = 0.7
				break
			}
		}
	}

	// Shape-aware assumptions: contrapositive thinking surfaces assumptions
	assumptions := 0.5
	if shapesUsed != nil {
		for _, shape := range shapesUsed {
			if shape == thinking.ShapeContrapositive {
				assumptions = 0.7
				break
			}
		}
	}

	return Score{
		Completeness: completeness,
		Consistency:  consistency,
		Specificity:  specificity,
		Research:     research,
		Assumptions:  assumptions,
	}
}

// ApplyQualityScores blends scan quality scores into an existing Score.
// Mapping: Grounding→Research, Clarity→Specificity, Completeness→Completeness,
// Consistency→Consistency. Values are averaged with existing scores.
func (c *Calculator) ApplyQualityScores(s Score, quality *scan.QualityScores) Score {
	if quality == nil {
		return s
	}
	s.Research = avg(s.Research, quality.Grounding)
	s.Specificity = avg(s.Specificity, quality.Clarity)
	s.Completeness = avg(s.Completeness, quality.Completeness)
	s.Consistency = avg(s.Consistency, quality.Consistency)
	return s
}

func avg(a, b float64) float64 {
	return (a + b) / 2.0
}
