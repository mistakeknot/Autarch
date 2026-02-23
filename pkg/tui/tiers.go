package tui

// Width tier thresholds aligned with the design in research/ntm.
// Consumers should call TierForWidthWithHysteresis during resize to
// prevent rapid layout toggling near boundaries.
const (
	SplitViewThreshold     = 120
	WideViewThreshold      = 200
	UltraWideViewThreshold = 240
	MegaWideViewThreshold  = 320
)

// Tier describes the current width bucket.
type Tier int

const (
	TierNarrow Tier = 0
	TierSplit  Tier = 1
	TierWide   Tier = 2
	// 3 intentionally unused to preserve ordering compatibility with ntm.
	TierUltra Tier = 4
	TierMega  Tier = 5
)

// TierForWidth maps a terminal width to a tier without hysteresis.
func TierForWidth(width int) Tier {
	switch {
	case width >= MegaWideViewThreshold:
		return TierMega
	case width >= UltraWideViewThreshold:
		return TierUltra
	case width >= WideViewThreshold:
		return TierWide
	case width >= SplitViewThreshold:
		return TierSplit
	default:
		return TierNarrow
	}
}

// HysteresisMargin is the number of columns of padding around tier
// boundaries to prevent flickering during resize.
const HysteresisMargin = 5

// TierForWidthWithHysteresis maps width to a tier, preferring the
// previous tier when the width is within HysteresisMargin of a
// boundary. Pass TierNarrow as prevTier on the first call.
func TierForWidthWithHysteresis(width int, prevTier Tier) Tier {
	newTier := TierForWidth(width)
	if newTier == prevTier || prevTier < TierNarrow || prevTier > TierMega {
		return newTier
	}

	switch prevTier {
	case TierNarrow:
		if width < SplitViewThreshold+HysteresisMargin {
			return TierNarrow
		}
	case TierSplit:
		if width >= SplitViewThreshold-HysteresisMargin && width < WideViewThreshold+HysteresisMargin {
			return TierSplit
		}
	case TierWide:
		if width >= WideViewThreshold-HysteresisMargin && width < UltraWideViewThreshold+HysteresisMargin {
			return TierWide
		}
	case TierUltra:
		if width >= UltraWideViewThreshold-HysteresisMargin && width < MegaWideViewThreshold+HysteresisMargin {
			return TierUltra
		}
	case TierMega:
		if width >= MegaWideViewThreshold-HysteresisMargin {
			return TierMega
		}
	}

	return newTier
}

// TokensForTier returns design tokens appropriate for the given tier.
// This bridges the tier system (width bucketing with hysteresis) to the
// token system (spatial dimension presets).
func TokensForTier(t Tier) DesignTokens {
	switch t {
	case TierNarrow:
		return Compact()
	case TierSplit:
		return DefaultTokens()
	case TierWide:
		return Spacious()
	case TierUltra, TierMega:
		return UltraWide()
	default:
		return DefaultTokens()
	}
}
