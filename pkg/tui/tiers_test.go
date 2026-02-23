package tui

import "testing"

func TestTierForWidth(t *testing.T) {
	tests := []struct {
		width int
		want  Tier
	}{
		{0, TierNarrow},
		{119, TierNarrow},
		{120, TierSplit},
		{121, TierSplit},
		{199, TierSplit},
		{200, TierWide},
		{239, TierWide},
		{240, TierUltra},
		{319, TierUltra},
		{320, TierMega},
		{400, TierMega},
	}

	for _, tt := range tests {
		if got := TierForWidth(tt.width); got != tt.want {
			t.Errorf("TierForWidth(%d) = %v, want %v", tt.width, got, tt.want)
		}
	}
}

func TestTierForWidthBoundaries(t *testing.T) {
	if got := TierForWidth(239); got != TierWide {
		t.Errorf("TierForWidth(239) = %v, want TierWide", got)
	}
	if got := TierForWidth(240); got != TierUltra {
		t.Errorf("TierForWidth(240) = %v, want TierUltra", got)
	}
	if got := TierForWidth(319); got != TierUltra {
		t.Errorf("TierForWidth(319) = %v, want TierUltra", got)
	}
	if got := TierForWidth(320); got != TierMega {
		t.Errorf("TierForWidth(320) = %v, want TierMega", got)
	}
}

func TestTierConstants(t *testing.T) {
	if SplitViewThreshold != 120 {
		t.Errorf("SplitViewThreshold = %d, want 120", SplitViewThreshold)
	}
	if WideViewThreshold != 200 {
		t.Errorf("WideViewThreshold = %d, want 200", WideViewThreshold)
	}
	if UltraWideViewThreshold != 240 {
		t.Errorf("UltraWideViewThreshold = %d, want 240", UltraWideViewThreshold)
	}
	if MegaWideViewThreshold != 320 {
		t.Errorf("MegaWideViewThreshold = %d, want 320", MegaWideViewThreshold)
	}

	if TierNarrow != 0 {
		t.Errorf("TierNarrow = %d, want 0", TierNarrow)
	}
	if TierSplit != 1 {
		t.Errorf("TierSplit = %d, want 1", TierSplit)
	}
	if TierWide != 2 {
		t.Errorf("TierWide = %d, want 2", TierWide)
	}
	if TierUltra != 4 {
		t.Errorf("TierUltra = %d, want 4", TierUltra)
	}
	if TierMega != 5 {
		t.Errorf("TierMega = %d, want 5", TierMega)
	}
}

func TestTierForWidthWithHysteresis_NoChange(t *testing.T) {
	t.Parallel()
	if got := TierForWidthWithHysteresis(150, TierSplit); got != TierSplit {
		t.Errorf("same tier: got %v, want TierSplit", got)
	}
}

func TestTierForWidthWithHysteresis_InvalidPrevTier(t *testing.T) {
	t.Parallel()
	if got := TierForWidthWithHysteresis(150, Tier(-1)); got != TierSplit {
		t.Errorf("invalid prev tier: got %v, want TierSplit", got)
	}
	if got := TierForWidthWithHysteresis(150, Tier(99)); got != TierSplit {
		t.Errorf("too-large prev tier: got %v, want TierSplit", got)
	}
}

func TestTierForWidthWithHysteresis_NarrowSticky(t *testing.T) {
	t.Parallel()
	// At 120 (exactly split threshold) — within hysteresis, stays narrow
	if got := TierForWidthWithHysteresis(120, TierNarrow); got != TierNarrow {
		t.Errorf("narrow sticky at 120: got %v, want TierNarrow", got)
	}
	// At 124 (threshold + margin - 1), still stays narrow
	if got := TierForWidthWithHysteresis(124, TierNarrow); got != TierNarrow {
		t.Errorf("narrow sticky at 124: got %v, want TierNarrow", got)
	}
	// At 125 (threshold + margin), transitions to split
	if got := TierForWidthWithHysteresis(125, TierNarrow); got != TierSplit {
		t.Errorf("narrow to split at 125: got %v, want TierSplit", got)
	}
}

func TestTierForWidthWithHysteresis_SplitSticky(t *testing.T) {
	t.Parallel()
	// Shrink toward narrow — stays split within margin
	if got := TierForWidthWithHysteresis(116, TierSplit); got != TierSplit {
		t.Errorf("split sticky at 116: got %v, want TierSplit", got)
	}
	// Below (split - margin), transitions to narrow
	if got := TierForWidthWithHysteresis(114, TierSplit); got != TierNarrow {
		t.Errorf("split to narrow at 114: got %v, want TierNarrow", got)
	}

	// Grow toward wide — stays split within margin
	if got := TierForWidthWithHysteresis(200, TierSplit); got != TierSplit {
		t.Errorf("split sticky at 200: got %v, want TierSplit", got)
	}
	if got := TierForWidthWithHysteresis(204, TierSplit); got != TierSplit {
		t.Errorf("split sticky at 204: got %v, want TierSplit", got)
	}
	// Past (wide + margin), transitions to wide
	if got := TierForWidthWithHysteresis(205, TierSplit); got != TierWide {
		t.Errorf("split to wide at 205: got %v, want TierWide", got)
	}
}

func TestTierForWidthWithHysteresis_WideSticky(t *testing.T) {
	t.Parallel()
	// Shrink toward split
	if got := TierForWidthWithHysteresis(196, TierWide); got != TierWide {
		t.Errorf("wide sticky at 196: got %v, want TierWide", got)
	}
	if got := TierForWidthWithHysteresis(194, TierWide); got != TierSplit {
		t.Errorf("wide to split at 194: got %v, want TierSplit", got)
	}

	// Grow toward ultra
	if got := TierForWidthWithHysteresis(244, TierWide); got != TierWide {
		t.Errorf("wide sticky at 244: got %v, want TierWide", got)
	}
	if got := TierForWidthWithHysteresis(245, TierWide); got != TierUltra {
		t.Errorf("wide to ultra at 245: got %v, want TierUltra", got)
	}
}

func TestTierForWidthWithHysteresis_UltraSticky(t *testing.T) {
	t.Parallel()
	// Shrink toward wide
	if got := TierForWidthWithHysteresis(236, TierUltra); got != TierUltra {
		t.Errorf("ultra sticky at 236: got %v, want TierUltra", got)
	}
	if got := TierForWidthWithHysteresis(234, TierUltra); got != TierWide {
		t.Errorf("ultra to wide at 234: got %v, want TierWide", got)
	}

	// Grow toward mega
	if got := TierForWidthWithHysteresis(324, TierUltra); got != TierUltra {
		t.Errorf("ultra sticky at 324: got %v, want TierUltra", got)
	}
	if got := TierForWidthWithHysteresis(325, TierUltra); got != TierMega {
		t.Errorf("ultra to mega at 325: got %v, want TierMega", got)
	}
}

func TestTierForWidthWithHysteresis_MegaSticky(t *testing.T) {
	t.Parallel()
	// Shrink toward ultra
	if got := TierForWidthWithHysteresis(316, TierMega); got != TierMega {
		t.Errorf("mega sticky at 316: got %v, want TierMega", got)
	}
	if got := TierForWidthWithHysteresis(314, TierMega); got != TierUltra {
		t.Errorf("mega to ultra at 314: got %v, want TierUltra", got)
	}
}
