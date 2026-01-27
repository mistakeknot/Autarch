package confidence

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func TestCalculate_ZeroPhases(t *testing.T) {
	c := NewCalculator()
	s := c.Calculate(0, 0, 0, 0.0)
	if s.Completeness != 0 || s.Research != 0 {
		t.Errorf("expected zero score, got %+v", s)
	}
}

func TestCalculate_ResearchQuality(t *testing.T) {
	c := NewCalculator()

	s0 := c.Calculate(5, 3, 0, 0.0)
	if s0.Research != 0.0 {
		t.Errorf("expected Research=0.0, got %f", s0.Research)
	}

	s5 := c.Calculate(5, 3, 0, 0.5)
	if !approxEqual(s5.Research, 0.5) {
		t.Errorf("expected Research=0.5, got %f", s5.Research)
	}

	s1 := c.Calculate(5, 3, 0, 1.0)
	if !approxEqual(s1.Research, 1.0) {
		t.Errorf("expected Research=1.0, got %f", s1.Research)
	}
}

func TestCalculate_ClampResearch(t *testing.T) {
	c := NewCalculator()

	sNeg := c.Calculate(5, 3, 0, -0.5)
	if sNeg.Research != 0 {
		t.Errorf("expected clamped to 0, got %f", sNeg.Research)
	}

	sOver := c.Calculate(5, 3, 0, 1.5)
	if sOver.Research != 1 {
		t.Errorf("expected clamped to 1, got %f", sOver.Research)
	}
}

func TestCalculate_Consistency(t *testing.T) {
	c := NewCalculator()

	s := c.Calculate(5, 5, 0, 0.5)
	if !approxEqual(s.Consistency, 1.0) {
		t.Errorf("expected 1.0 consistency with 0 conflicts, got %f", s.Consistency)
	}

	s2 := c.Calculate(5, 5, 2, 0.5)
	if !approxEqual(s2.Consistency, 1.0/3.0) {
		t.Errorf("expected ~0.333 consistency with 2 conflicts, got %f", s2.Consistency)
	}
}
