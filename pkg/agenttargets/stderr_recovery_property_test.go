package agenttargets

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

type recoverySpan struct{ start, end int }

// The legacy oracle uses frozen normalization/grammar. New-rule intervals
// come only from reviewed table expectations or explicit fixture intervals.
func checkRecoverySources(t testing.TB, input string, chunk int, required []recoverySpan) string {
	t.Helper()
	tail := recoveryWrite(input, chunk)
	if input == "" {
		recoveryEvaluations++
		if err := processError(context.Canceled, false, tail); err != context.Canceled {
			t.Fatalf("empty stderr changed process error: %v", err)
		}
		return ""
	}
	raw := []byte(input)
	first := strings.IndexByte(input, '\n')
	eligible := len(raw) >= 4096 && len(raw) <= 8192 && (first < 0 || first == len(raw)-1)
	if !eligible {
		got := recoveryOutput(t, tail)
		if want := sanitizedStderr(tail.data); got != want {
			t.Fatalf("ineligible recovery: %q != %q", got, want)
		}
		return got
	}
	text, src, mask := referenceRegion(t, raw, 0)
	_, _, windowMask := referenceRegion(t, raw[len(raw)-4096:], len(raw)-4096)
	for i, hidden := range windowMask {
		mask[i] = mask[i] || hidden
	}
	for _, span := range required {
		if span.start < 0 || span.end > len(raw) || span.start >= span.end {
			t.Fatal("bad independent interval")
		}
		for p := span.start; p < span.end; p++ {
			mask[p] = true
		}
	}
	surviving := make([]bool, len(raw))
	for _, p := range src {
		surviving[p] = true
	}
	observed := false
	stderrRenderObserver = func(region stderrRegion, actual []int32, emitted []int32) {
		if emitted == nil {
			observed = true
			if string(region.text) != text || len(region.source) != len(src) {
				t.Fatal("full history normalization differs")
			}
			for i, p := range src {
				if region.source[i] != int32(p) {
					t.Fatal("source offset differs")
				}
			}
			for p, hidden := range mask {
				if hidden && actual[p] <= 0 {
					t.Fatalf("required hidden source %d exposed", p)
				}
			}
		}
		for _, p := range emitted {
			if !surviving[p] || mask[p] {
				t.Fatalf("emitted hidden or erased source %d", p)
			}
		}
	}
	defer func() { stderrRenderObserver = nil }()
	got := recoveryOutput(t, tail)
	if !observed {
		t.Fatal("eligible stream bypassed renderer")
	}
	if !utf8.ValidString(got) || len(got) > 4096 {
		t.Fatal("invalid recovery output")
	}
	for _, r := range got {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			t.Fatal("control survived")
		}
	}
	return got
}

func recoveryTableSpans(t testing.TB, c redactCase, base int, finalLF bool) []recoverySpan {
	t.Helper()
	// Derive the LF form from the reviewed output, preserving LF for capped
	// equals/colon/flag values. Only an open quoted JSON value consumes it.
	jsonToEnd := strings.HasPrefix(c.input, `{"token":"`) && strings.Count(c.input, "\"") == 3
	hidden := expectedHidden(t, c)
	if finalLF {
		c.input += "\n"
		hidden = append(hidden, jsonToEnd)
	}
	_, _, legacy := referenceRegion(t, []byte(c.input), 0)
	_, mainMask := stderrSourceMask([]byte(c.input), 0)
	for p := range hidden {
		if (legacy[p] || mainMask[p] > 0) && !hidden[p] {
			t.Fatalf("LF-form expectation omits main hidden source %d in %q", p, c.input)
		}
	}
	if finalLF && (hidden[len(hidden)-1] != jsonToEnd || (mainMask[len(hidden)-1] > 0) != jsonToEnd) {
		t.Fatalf("final LF visibility differs for %q", c.input)
	}
	var spans []recoverySpan
	for p := 0; p < len(hidden); {
		if !hidden[p] {
			p++
			continue
		}
		start := p
		for p < len(hidden) && hidden[p] {
			p++
		}
		spans = append(spans, recoverySpan{base + start, base + p})
	}

	return spans
}

func TestRecoveryTableEmbeddings(t *testing.T) {
	// This is the first recovery test in each package iteration.
	recoveryEvaluationStart = recoveryEvaluations
	prefix := strings.Repeat("x", 5000) + " \r"
	for i, c := range newShapeCases {
		if got := sanitizedStderr([]byte(c.input)); got != c.want {
			t.Fatalf("direct row %d changed", i)
		}
		if !strings.Contains(c.want, "[REDACTED]") {
			continue
		}
		for _, end := range []string{"", "\n"} {
			input := prefix + c.input + end
			t.Run(fmt.Sprintf("%d/%q", i, end), func(t *testing.T) {
				// Interior raw LF explicitly exercises fallback; no positive is dropped.
				spans := recoveryTableSpans(t, c, len(prefix), end != "")
				got := checkRecoverySources(t, input, 7, spans)
				if end != "" && !strings.Contains(c.input, "\n") && !strings.HasSuffix(got, "\n") {
					t.Fatal("LF-capped table row lost its visible final LF")
				}
			})
		}
	}
	// Here both legacy and multiline JSON rules include the final LF.
	input := `{"token":"` + strings.Repeat("1", 5000) + "\n"
	jsonCase := redactCase{strings.TrimSuffix(input, "\n"), "{[REDACTED]", 0}
	jsonSpans := recoveryTableSpans(t, jsonCase, 0, true)
	if got := checkRecoverySources(t, input, 7, jsonSpans); got != "{[REDACTED]" {
		t.Fatal(got)
	}
	// Independently demonstrate the flag boundary belongs only to W.
	input = strings.Repeat("x", 96) + "--token 1111 " + strings.Repeat("x", 4083)
	base := len(input) - 4096
	if !strings.HasPrefix(input[base:], "--token ") || strings.Contains(input[:base], " ") {
		t.Fatal("W-only flag premise")
	}
	checkRecoverySources(t, input, 7, []recoverySpan{{base + 8, base + 12}})
}

func TestRecoveryGenerated(t *testing.T) {
	rng := rand.New(rand.NewSource(82))
	families := []struct{ pre, post string }{
		{"password=", ""}, {"token=", ""}, {"--password ", ""},
		{"https://u:", "@example.test/"}, {`{"password":"`, `"}`},
		{"Bearer ", ""}, {"Bea\x1b[mrer ", ""}, {`{"token":"`, `"}`},
	}
	for i := 0; i < 96; i++ {
		f := families[i%len(families)]
		value := strings.Repeat([]string{"1", "D", "�", "é", "界", "\xff"}[i%6], 1400+rng.Intn(400))
		// The value interval is explicit, independent of either matcher.
		prefix := strings.Repeat("x", rng.Intn(1800)) + " \r" + f.pre
		input := prefix + value + f.post
		if len(input) < 4096 {
			input = strings.Repeat("x", 4096-len(input)) + input
			prefix = strings.Repeat("x", 4096-len(prefix+value+f.post)) + prefix
		}
		if i%3 == 0 {
			input += "\n"
		}
		checkRecoverySources(t, input, []int{1, 7, 1024, 4096}[i%4], []recoverySpan{{len(prefix), len(prefix) + len(value)}})
	}
	for _, input := range []string{
		strings.Repeat("x", 5000) + " \rBearer \xc3\x1b[0m\xa9",
		strings.Repeat("x", 5000) + "\x1b]" + strings.Repeat("1", 1500) + "\x07ok",
		"\x1b]" + strings.Repeat("1", 5000) + "\x07ok",
		strings.Repeat("x", 5000) + "\rBearer \xff11",
		strings.Repeat("x", 5000) + "\x1b]payload\n\x07ok",
		strings.Repeat("x", 32768),
	} {
		checkRecoverySources(t, input, 7, nil)
	}
}

func TestRecoveryCriticalSplits(t *testing.T) {
	fixtures := []string{
		strings.Repeat("x", 5000) + "\rError: quota exceeded\r\n",
		"Bea\x1b[mrer " + strings.Repeat("1", 1100) + "\r" + strings.Repeat("1", 5000),
		"\x1b]" + strings.Repeat("1", 5000) + "\x1b\\ok",
		`{"password":"` + strings.Repeat("1", 5000) + `"}`,
		strings.Repeat("é", 2500) + "x",
		strings.Repeat("x", 8192) + "\n",
	}
	for _, input := range fixtures {
		want := recoveryOutput(t, recoveryWrite(input, len(input)))
		points := map[int]bool{4095: true, 4096: true, 4097: true, 8191: true, 8192: true, 8193: true}
		for i, b := range []byte(input) {
			if b == '\r' || b == '\n' || b == '\x1b' || b == '"' || b >= 128 || i < 20 {
				// UTF-8's first occurrence suffices for each code-unit boundary.
				if b >= 128 && i > 8 {
					continue
				}
				for _, p := range []int{i, i + 1} {
					points[p] = true
				}
			}
		}
		for p := range points {
			if p < 0 || p > len(input) {
				continue
			}
			tail := new(stderrTail)
			tail.Write(nil)
			tail.Write([]byte(input[:p]))
			tail.Write(nil)
			tail.Write([]byte(input[p:]))
			tail.Write(nil)
			if got := recoveryOutput(t, tail); got != want {
				t.Fatalf("split %d differs", p)
			}
			if len(input) <= 8192 {
				h, ok := tail.completeHistory()
				if !ok || string(h) != input {
					t.Fatal("split certification")
				}
			} else if !tail.historyLost {
				t.Fatal("split overflow")
			}
		}
	}
}

func TestRecoveryExtended(t *testing.T) {
	if os.Getenv("STDERR_RECOVERY_EXTENDED") != "1" {
		t.Skip("set STDERR_RECOVERY_EXTENDED=1 for exhaustive offsets")
	}
	for _, shape := range []string{"Bearer 1111", "--token 1111", `{"password":"1111"}`, "\xc3\x1b[m\xa9", "\x1b]1111\x07ok"} {
		for offset := 0; offset <= len(shape); offset++ {
			input := strings.Repeat("x", 4096-offset) + shape + strings.Repeat("x", 100)
			for _, end := range []string{"", "\n", "\r\n"} {
				checkRecoverySources(t, input+end, 7, nil)
			}
		}
	}
	for _, n := range []int{4095, 4096, 4097, 8191, 8192, 8193, 32768} {
		input := strings.Repeat("x", n)
		for split := 0; split <= n; split++ {
			tail := new(stderrTail)
			tail.Write([]byte(input[:split]))
			tail.Write([]byte(input[split:]))
			h, ok := tail.completeHistory()
			if ok != (n >= 4096 && n <= 8192) || ok && !bytes.Equal(h, []byte(input)) {
				t.Fatalf("threshold %d/%d", n, split)
			}
		}
	}
}

func FuzzRecoverySuperset(f *testing.F) {
	f.Add([]byte{}, uint16(1))
	for _, n := range []int{4095, 4096, 4097, 8191, 8192, 8193, 32768} {
		f.Add([]byte(strings.Repeat("x", n)), uint16(7))
	}
	for _, input := range []string{"Bearer 1111", "\xff\xc3\x1b[m\xa9", "\x1b]payload\n\x07ok", `{"token":"1111`} {
		f.Add([]byte(input), uint16(1024))
	}
	f.Fuzz(func(t *testing.T, raw []byte, size uint16) {
		if len(raw) > 65536 {
			t.Skip()
		}
		chunk := 1 + int(size)%8193
		checkRecoverySources(t, string(raw), chunk, nil)
		// Restricted generator retains an explicit new-rule value interval.
		value := strings.Repeat("1", 4096+len(raw)%4090)
		input := "password=" + value
		if size%2 == 0 {
			input += "\n"
		}
		checkRecoverySources(t, input, chunk, []recoverySpan{{9, 9 + len(value)}})
	})
}
