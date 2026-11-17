package agenttargets

import (
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// referenceRegion is deliberately independent of production normalization.
// Source tags follow bytes through ANSI concatenation and UTF-8 decoding.
func referenceRegion(t testing.TB, raw []byte, base int) (string, []int, []bool) {
	t.Helper()
	erased := make([]bool, len(raw))
	for _, span := range frozenANSI.FindAllIndex(raw, -1) {
		for i := span[0]; i < span[1]; i++ {
			erased[i] = true
		}
	}
	var joined []byte
	var tags []int
	for i, b := range raw {
		if !erased[i] {
			joined = append(joined, b)
			tags = append(tags, base+i)
		}
	}
	var text strings.Builder
	var sources []int
	for i := 0; i < len(joined); {
		r, n := utf8.DecodeRune(joined[i:])
		if !unicode.IsControl(r) || r == '\n' || r == '\t' {
			text.WriteRune(r)
			if r == utf8.RuneError && n == 1 {
				sources = append(sources, tags[i], tags[i], tags[i])
			} else {
				sources = append(sources, tags[i:i+n]...)
			}
		}
		i += n
	}
	normalized := text.String()
	frozen := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, frozenANSI.ReplaceAllString(string(raw), ""))
	if normalized != frozen {
		t.Fatalf("reference normalization drift: %q != %q", normalized, frozen)
	}
	mask := make([]bool, base+len(raw))
	for _, span := range frozenToken.FindAllStringIndex(normalized, -1) {
		for i := sources[span[0]]; i <= sources[span[1]-1]; i++ {
			mask[i] = true
		}
	}
	return normalized, sources, mask
}

func referenceStart(raw []byte) (int, bool) {
	if len(raw) < stderrTailBytes {
		return 0, false
	}
	start := len(raw)
	for i, b := range raw {
		if b == '\n' {
			start = i + 1
			break
		}
	}
	return start, strings.TrimSpace(string(raw[start:])) == ""
}

// Verify each output byte can be attributed to a source main exposes, in order.
// The source-mask assertion added with the renderer also checks positions even
// when a hidden and a visible byte happen to have the same spelling.
func checkMainSuperset(t testing.TB, raw []byte) {
	t.Helper()
	got := sanitizedStderr(raw)
	start, notice := referenceStart(raw)
	if notice {
		if got != fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(raw)) {
			t.Fatalf("notice: %q", got)
		}
		return
	}
	text, src, mask := referenceRegion(t, raw[start:], start)
	var visible strings.Builder
	for i := 0; i < len(text); {
		_, n := utf8.DecodeRuneInString(text[i:])
		hidden := false
		for _, p := range src[i : i+n] {
			hidden = hidden || mask[p]
		}
		if !hidden {
			visible.WriteString(text[i : i+n])
		}
		i += n
	}
	allowed := visible.String()
	output := strings.ReplaceAll(got, "[REDACTED]", "")
	// Marker spelling is generated text; all other bytes must retain ordering.
	for i := 0; i < len(output); i++ {
		p := strings.IndexByte(allowed, output[i])
		if p < 0 {
			t.Fatalf("superset attribution failed: raw %q output %q", raw, got)
		}
		allowed = allowed[p+1:]
	}
	if !utf8.ValidString(got) || len(got) > 4096 {
		t.Fatalf("invalid output %q", got)
	}
	for _, r := range got {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			t.Fatalf("control in %q", got)
		}
	}
}
