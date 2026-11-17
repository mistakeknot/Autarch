package agenttargets

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// stderrRegion preserves provenance through ANSI removal and UTF-8 repair.
// Offsets are absolute in the retained raw window, including for region (a).
type stderrRegion struct {
	text   []byte
	source []int32
	ansi   [][]int
}

func normalizeStderr(raw []byte, base int) stderrRegion {
	region := stderrRegion{ansi: stderrANSI.FindAllIndex(raw, -1)}
	joined := make([]byte, 0, len(raw))
	tags := make([]int32, 0, len(raw))
	next := 0
	for i := 0; i < len(raw); {
		if next < len(region.ansi) && i == region.ansi[next][0] {
			i = region.ansi[next][1]
			next++
			continue
		}
		joined = append(joined, raw[i])
		tags = append(tags, int32(base+i))
		i++
	}
	region.text = make([]byte, 0, 3*len(raw))
	region.source = make([]int32, 0, 3*len(raw))
	for i := 0; i < len(joined); {
		r, n := utf8.DecodeRune(joined[i:])
		if !unicode.IsControl(r) || r == '\n' || r == '\t' {
			region.text = utf8.AppendRune(region.text, r)
			if r == utf8.RuneError && n == 1 {
				region.source = append(region.source, tags[i], tags[i], tags[i])
			} else {
				region.source = append(region.source, tags[i:i+n]...)
			}
		}
		i += n
	}
	return region
}

// maskStderrSpan maps a normalized match to an inclusive raw span. Stripped
// bytes between its sources belong to the span too. Difference-array updates
// keep overlapping candidates constant cost.
func maskStderrSpan(diff []int32, region stderrRegion, start, end int) {
	if start >= end {
		return
	}
	left, right := region.source[start], region.source[end-1]
	diff[left]++
	diff[right+1]--
}

func stderrSourceMask(raw []byte, start int) (stderrRegion, []int32) {
	a := normalizeStderr(raw[start:], start)
	diff := make([]int32, len(raw)+1)
	apply := func(region stderrRegion) {
		for _, span := range stderrToken.FindAllIndex(region.text, -1) {
			maskStderrSpan(diff, region, span[0], span[1])
		}
	}
	apply(a)
	if start != 0 {
		apply(normalizeStderr(raw, 0))
	}
	for i := 1; i < len(diff); i++ {
		diff[i] += diff[i-1]
	}
	return a, diff
}

func renderStderr(raw []byte, start int) string {
	region, mask := stderrSourceMask(raw, start)
	var out strings.Builder
	out.Grow(len(region.text))
	hiddenRun := false
	for i := 0; i < len(region.text); {
		_, n := utf8.DecodeRune(region.text[i:])
		hidden := false
		for _, p := range region.source[i : i+n] {
			hidden = hidden || mask[p] > 0
		}
		if hidden {
			if !hiddenRun {
				out.WriteString("[REDACTED]")
			}
		} else {
			out.Write(region.text[i : i+n])
		}
		hiddenRun = hidden
		i += n
	}
	text := out.String()
	if len(text) > stderrTailBytes {
		cut := len(text) - stderrTailBytes
		for cut < len(text) && !utf8.RuneStart(text[cut]) {
			cut++
		}
		text = text[cut:]
	}
	return strings.Clone(text)
}
