package tui

import "testing"

func TestDefaultTokens(t *testing.T) {
	tok := DefaultTokens()

	// Spacing should have ascending non-negative values.
	if tok.Spacing.None != 0 {
		t.Errorf("Spacing.None = %d, want 0", tok.Spacing.None)
	}
	if tok.Spacing.XS < tok.Spacing.None {
		t.Error("Spacing.XS should be >= None")
	}
	if tok.Spacing.XXL <= tok.Spacing.XL {
		t.Error("Spacing.XXL should be > XL")
	}

	// Size tokens should all be positive.
	if tok.Size.XS <= 0 {
		t.Errorf("Size.XS = %d, want > 0", tok.Size.XS)
	}
	if tok.Size.XXL <= tok.Size.XL {
		t.Error("Size.XXL should be > XL")
	}

	// Typography sizes should be ascending.
	if tok.Typography.SizeXS >= tok.Typography.SizeSM {
		t.Error("Typography sizes should be ascending")
	}
	if tok.Typography.SizeXXL <= tok.Typography.SizeXL {
		t.Error("Typography.SizeXXL should be > SizeXL")
	}

	// Layout margins/padding should be non-negative.
	if tok.Layout.MarginPage < 0 {
		t.Error("Layout.MarginPage should be non-negative")
	}
	if tok.Layout.PaddingCard < 0 {
		t.Error("Layout.PaddingCard should be non-negative")
	}

	// Animation ticks should be positive.
	if tok.Animation.TickFast <= 0 {
		t.Errorf("Animation.TickFast = %d, want > 0", tok.Animation.TickFast)
	}
	if tok.Animation.TickFast >= tok.Animation.TickNormal {
		t.Error("TickFast should be < TickNormal")
	}

	// Breakpoints should be ascending.
	if tok.Breakpoints.XS >= tok.Breakpoints.SM {
		t.Error("Breakpoints should be ascending")
	}
	if tok.Breakpoints.UltraWide <= tok.Breakpoints.Wide {
		t.Error("Breakpoints.UltraWide should be > Wide")
	}
}

func TestCompactSmallerThanDefault(t *testing.T) {
	compact := Compact()
	def := DefaultTokens()

	if compact.Spacing.MD > def.Spacing.MD {
		t.Errorf("Compact Spacing.MD (%d) should be <= Default (%d)",
			compact.Spacing.MD, def.Spacing.MD)
	}
	if compact.Size.MD > def.Size.MD {
		t.Errorf("Compact Size.MD (%d) should be <= Default (%d)",
			compact.Size.MD, def.Size.MD)
	}
	if compact.Layout.MarginPage > def.Layout.MarginPage {
		t.Errorf("Compact MarginPage (%d) should be <= Default (%d)",
			compact.Layout.MarginPage, def.Layout.MarginPage)
	}
	if compact.Layout.PaddingCard > def.Layout.PaddingCard {
		t.Errorf("Compact PaddingCard (%d) should be <= Default (%d)",
			compact.Layout.PaddingCard, def.Layout.PaddingCard)
	}
}

func TestSpaciousLargerThanDefault(t *testing.T) {
	spacious := Spacious()
	def := DefaultTokens()

	if spacious.Spacing.MD < def.Spacing.MD {
		t.Errorf("Spacious Spacing.MD (%d) should be >= Default (%d)",
			spacious.Spacing.MD, def.Spacing.MD)
	}
	if spacious.Size.MD < def.Size.MD {
		t.Errorf("Spacious Size.MD (%d) should be >= Default (%d)",
			spacious.Size.MD, def.Size.MD)
	}
	if spacious.Layout.MarginPage < def.Layout.MarginPage {
		t.Errorf("Spacious MarginPage (%d) should be >= Default (%d)",
			spacious.Layout.MarginPage, def.Layout.MarginPage)
	}
}

func TestUltraWideLargerThanSpacious(t *testing.T) {
	ultra := UltraWide()
	spacious := Spacious()

	if ultra.Spacing.MD < spacious.Spacing.MD {
		t.Errorf("UltraWide Spacing.MD (%d) should be >= Spacious (%d)",
			ultra.Spacing.MD, spacious.Spacing.MD)
	}
	if ultra.Size.MD < spacious.Size.MD {
		t.Errorf("UltraWide Size.MD (%d) should be >= Spacious (%d)",
			ultra.Size.MD, spacious.Size.MD)
	}
	if ultra.Layout.ModalWidth < spacious.Layout.ModalWidth {
		t.Errorf("UltraWide ModalWidth (%d) should be >= Spacious (%d)",
			ultra.Layout.ModalWidth, spacious.Layout.ModalWidth)
	}
}

func TestTokensForWidth(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		wantMode TokenPreset
	}{
		{"very narrow", 20, PresetCompact},
		{"narrow boundary", 39, PresetCompact},
		{"small", 40, PresetDefault},
		{"medium", 60, PresetDefault},
		{"at MD boundary", 80, PresetSpacious},
		{"wide", 150, PresetSpacious},
		{"at Wide boundary", 200, PresetUltraWide},
		{"ultra-wide", 250, PresetUltraWide},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTokenPreset(tt.width)
			if got != tt.wantMode {
				t.Errorf("GetTokenPreset(%d) = %d, want %d", tt.width, got, tt.wantMode)
			}

			// TokensForWidth should return matching preset.
			tok := TokensForWidth(tt.width)
			var expected DesignTokens
			switch tt.wantMode {
			case PresetCompact:
				expected = Compact()
			case PresetDefault:
				expected = DefaultTokens()
			case PresetSpacious:
				expected = Spacious()
			case PresetUltraWide:
				expected = UltraWide()
			}
			if tok.Spacing.MD != expected.Spacing.MD {
				t.Errorf("TokensForWidth(%d).Spacing.MD = %d, want %d",
					tt.width, tok.Spacing.MD, expected.Spacing.MD)
			}
		})
	}
}

func TestAdaptiveCardDimensions(t *testing.T) {
	tests := []struct {
		name            string
		totalWidth      int
		minCardWidth    int
		maxCardWidth    int
		gap             int
		wantMinCards    int
		wantMaxCards    int
		wantMinWidth    int
		wantMaxWidth    int
	}{
		{
			name:         "standard grid",
			totalWidth:   120,
			minCardWidth: 25,
			maxCardWidth: 40,
			gap:          2,
			wantMinCards: 1,
			wantMaxCards: 10,
			wantMinWidth: 1,
			wantMaxWidth: 40,
		},
		{
			name:         "width less than min card",
			totalWidth:   15,
			minCardWidth: 25,
			maxCardWidth: 40,
			gap:          2,
			wantMinCards: 1,
			wantMaxCards: 1,
			wantMinWidth: 15,
			wantMaxWidth: 15,
		},
		{
			name:         "zero width",
			totalWidth:   0,
			minCardWidth: 25,
			maxCardWidth: 40,
			gap:          2,
			wantMinCards: 1,
			wantMaxCards: 1,
			wantMinWidth: 1,
			wantMaxWidth: 1,
		},
		{
			name:         "negative inputs",
			totalWidth:   -10,
			minCardWidth: 25,
			maxCardWidth: 40,
			gap:          2,
			wantMinCards: 1,
			wantMaxCards: 1,
			wantMinWidth: 1,
			wantMaxWidth: 1,
		},
		{
			name:         "zero min card width",
			totalWidth:   120,
			minCardWidth: 0,
			maxCardWidth: 40,
			gap:          2,
			wantMinCards: 1,
			wantMaxCards: 1,
			wantMinWidth: 1,
			wantMaxWidth: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cardWidth, cardsPerRow := AdaptiveCardDimensions(
				tt.totalWidth, tt.minCardWidth, tt.maxCardWidth, tt.gap)

			if cardsPerRow < tt.wantMinCards || cardsPerRow > tt.wantMaxCards {
				t.Errorf("cardsPerRow = %d, want [%d, %d]",
					cardsPerRow, tt.wantMinCards, tt.wantMaxCards)
			}
			if cardWidth < tt.wantMinWidth || cardWidth > tt.wantMaxWidth {
				t.Errorf("cardWidth = %d, want [%d, %d]",
					cardWidth, tt.wantMinWidth, tt.wantMaxWidth)
			}
		})
	}
}

func TestTokensForTier(t *testing.T) {
	tests := []struct {
		tier     Tier
		wantMode TokenPreset
	}{
		{TierNarrow, PresetCompact},
		{TierSplit, PresetDefault},
		{TierWide, PresetSpacious},
		{TierUltra, PresetUltraWide},
		{TierMega, PresetUltraWide},
	}

	for _, tt := range tests {
		tok := TokensForTier(tt.tier)
		var expected DesignTokens
		switch tt.wantMode {
		case PresetCompact:
			expected = Compact()
		case PresetDefault:
			expected = DefaultTokens()
		case PresetSpacious:
			expected = Spacious()
		case PresetUltraWide:
			expected = UltraWide()
		}
		if tok.Spacing.MD != expected.Spacing.MD {
			t.Errorf("TokensForTier(%d).Spacing.MD = %d, want %d",
				tt.tier, tok.Spacing.MD, expected.Spacing.MD)
		}
	}
}
