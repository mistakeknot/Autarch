package confidence

import (
	"math"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
	"github.com/mistakeknot/autarch/pkg/thinking"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func TestCalculate_ZeroPhases(t *testing.T) {
	c := NewCalculator()
	s := c.Calculate(0, 0, 0, 0.0, nil)
	if s.Completeness != 0 || s.Research != 0 {
		t.Errorf("expected zero score, got %+v", s)
	}
}

func TestCalculate_ResearchQuality(t *testing.T) {
	c := NewCalculator()

	s0 := c.Calculate(5, 3, 0, 0.0, nil)
	if s0.Research != 0.0 {
		t.Errorf("expected Research=0.0, got %f", s0.Research)
	}

	s5 := c.Calculate(5, 3, 0, 0.5, nil)
	if !approxEqual(s5.Research, 0.5) {
		t.Errorf("expected Research=0.5, got %f", s5.Research)
	}

	s1 := c.Calculate(5, 3, 0, 1.0, nil)
	if !approxEqual(s1.Research, 1.0) {
		t.Errorf("expected Research=1.0, got %f", s1.Research)
	}
}

func TestCalculate_ClampResearch(t *testing.T) {
	c := NewCalculator()

	sNeg := c.Calculate(5, 3, 0, -0.5, nil)
	if sNeg.Research != 0 {
		t.Errorf("expected clamped to 0, got %f", sNeg.Research)
	}

	sOver := c.Calculate(5, 3, 0, 1.5, nil)
	if sOver.Research != 1 {
		t.Errorf("expected clamped to 1, got %f", sOver.Research)
	}
}

func TestCalculate_Consistency(t *testing.T) {
	c := NewCalculator()

	s := c.Calculate(5, 5, 0, 0.5, nil)
	if !approxEqual(s.Consistency, 1.0) {
		t.Errorf("expected 1.0 consistency with 0 conflicts, got %f", s.Consistency)
	}

	s2 := c.Calculate(5, 5, 2, 0.5, nil)
	if !approxEqual(s2.Consistency, 1.0/3.0) {
		t.Errorf("expected ~0.333 consistency with 2 conflicts, got %f", s2.Consistency)
	}
}

func TestCalculate_ShapeAwareSpecificity(t *testing.T) {
	c := NewCalculator()

	// Without shapes: default 0.5
	sNil := c.Calculate(5, 3, 0, 0.5, nil)
	if !approxEqual(sNil.Specificity, 0.5) {
		t.Errorf("expected Specificity=0.5 without shapes, got %f", sNil.Specificity)
	}

	// With deductive shape: 0.7
	shapes := map[string]thinking.Shape{"Vision": thinking.ShapeDeductive}
	sDed := c.Calculate(5, 3, 0, 0.5, shapes)
	if !approxEqual(sDed.Specificity, 0.7) {
		t.Errorf("expected Specificity=0.7 with deductive, got %f", sDed.Specificity)
	}

	// With DSL shape: 0.7
	shapesDSL := map[string]thinking.Shape{"Features": thinking.ShapeDSL}
	sDSL := c.Calculate(5, 3, 0, 0.5, shapesDSL)
	if !approxEqual(sDSL.Specificity, 0.7) {
		t.Errorf("expected Specificity=0.7 with DSL, got %f", sDSL.Specificity)
	}
}

func TestCalculate_ShapeAwareAssumptions(t *testing.T) {
	c := NewCalculator()

	// Without shapes: default 0.5
	sNil := c.Calculate(5, 3, 0, 0.5, nil)
	if !approxEqual(sNil.Assumptions, 0.5) {
		t.Errorf("expected Assumptions=0.5 without shapes, got %f", sNil.Assumptions)
	}

	// With contrapositive shape: 0.7
	shapes := map[string]thinking.Shape{"Problem": thinking.ShapeContrapositive}
	sContra := c.Calculate(5, 3, 0, 0.5, shapes)
	if !approxEqual(sContra.Assumptions, 0.7) {
		t.Errorf("expected Assumptions=0.7 with contrapositive, got %f", sContra.Assumptions)
	}
}

func TestApplyQualityScores(t *testing.T) {
	c := NewCalculator()

	base := c.Calculate(5, 3, 0, 0.5, nil)
	// base: Research=0.5, Specificity=0.5, Completeness=0.6, Consistency=1.0

	quality := &scan.QualityScores{
		Grounding:    0.8, // → Research
		Clarity:      0.9, // → Specificity
		Completeness: 0.7, // → Completeness
		Consistency:  0.6, // → Consistency
	}
	result := c.ApplyQualityScores(base, quality)

	if !approxEqual(result.Research, (0.5+0.8)/2.0) {
		t.Errorf("expected Research=%f, got %f", (0.5+0.8)/2.0, result.Research)
	}
	if !approxEqual(result.Specificity, (0.5+0.9)/2.0) {
		t.Errorf("expected Specificity=%f, got %f", (0.5+0.9)/2.0, result.Specificity)
	}
}

func TestApplyQualityScores_Nil(t *testing.T) {
	c := NewCalculator()
	base := Score{Research: 0.5, Specificity: 0.5}
	result := c.ApplyQualityScores(base, nil)
	if !approxEqual(result.Research, 0.5) {
		t.Error("nil quality should not change scores")
	}
}
