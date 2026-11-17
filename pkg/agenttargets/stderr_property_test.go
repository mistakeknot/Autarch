package agenttargets

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"regexp"
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
	joined := make([]byte, 0, len(raw))
	tags := make([]int, 0, len(raw))
	for i, b := range raw {
		if !erased[i] {
			joined = append(joined, b)
			tags = append(tags, base+i)
		}
	}
	var text strings.Builder
	text.Grow(min(3*len(raw), 256))
	sources := make([]int, 0, min(3*len(raw), 256))
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

// Independent normalization for the cleaned-notice exception (mk-o9xlb).
func referenceCleanedRemainder(raw []byte) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, frozenANSI.ReplaceAllString(string(raw), ""))
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
	return start, strings.TrimSpace(referenceCleanedRemainder(raw[start:])) == "" || referenceEmptyFrame(raw[start:])
}

// Independently select the final CR frame on each line before control cleanup.
// A notice is content-free, so no redaction grammar is needed for this check.
func referenceEmptyFrame(raw []byte) bool {
	lines := strings.Split(frozenANSI.ReplaceAllString(string(raw), ""), "\n")
	for i, line := range lines {
		lines[i] = line[strings.LastIndexByte(line, '\r')+1:]
	}
	return referenceCleanedRemainder([]byte(strings.Join(lines, "\n"))) == ""
}

// Verify each output byte can be attributed to a source main exposes, in order.
// The source-mask assertion added with the renderer also checks positions even
// when a hidden and a visible byte happen to have the same spelling.
func checkMainSuperset(t testing.TB, raw []byte) {
	t.Helper()
	got := sanitizedStderr(raw)
	checkMainSupersetOutput(t, raw, got)
}
func checkMainSupersetOutput(t testing.TB, raw []byte, got string) {
	t.Helper()
	checkMainSupersetObserved(t, observeStderr(t, raw, got))
}
func checkMainSupersetObserved(t testing.TB, observation *stderrObservation) {
	t.Helper()
	raw, got, start, notice := observation.raw, observation.got, observation.start, observation.notice
	if notice {
		if got != fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(raw)) {
			t.Fatalf("notice: %q", got)
		}
		return
	}
	text, src, mask := observation.text, observation.source, observation.mask

	actualRegion, actualMask := stderrSourceMask(raw, start)
	if string(actualRegion.text) != text || len(actualRegion.source) != len(src) {
		t.Fatalf("production normalization for %q", raw)
	}
	for i, p := range src {
		if actualRegion.source[i] != int32(p) {
			t.Fatalf("production provenance for %q", raw)
		}
	}
	for p, hidden := range mask {
		if hidden && actualMask[p] <= 0 {
			t.Fatalf("legacy source %d exposed for %q", p, raw)
		}
	}
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
	// The final byte cut can bisect a generated marker. Its alphabet is
	// excluded here; exact source-mask coverage above remains positional.
	stripMarkerAlphabet := func(s string) string {
		return strings.Map(func(r rune) rune {
			if strings.ContainsRune("[REDACTED]", r) {
				return -1
			}
			return r
		}, s)
	}
	output := stripMarkerAlphabet(got)
	allowed = stripMarkerAlphabet(allowed)
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

// Sentinels occur only in one instance's value, never filler or generated text.
const stderrSentinels = "123578JMQ"

// An observation belongs to one input. Checks share its frozen-main
// normalization and provenance; no process-wide cache retains test inputs.
type stderrObservation struct {
	raw       []byte
	got, text string
	start     int
	notice    bool
	source    []int
	mask      []bool
	whole     *stderrObservation
}

func observeStderr(t testing.TB, raw []byte, got string) *stderrObservation {
	t.Helper()
	start, notice := referenceStart(raw)
	o := &stderrObservation{raw: raw, got: got, start: start, notice: notice}
	o.text, o.source, o.mask = referenceRegion(t, raw[start:], start)
	return o
}
func (o *stderrObservation) region(t testing.TB, base int) (string, []int, []bool) {
	t.Helper()
	if base == o.start {
		return o.text, o.source, o.mask
	}
	if o.whole == nil {
		text, src, mask := referenceRegion(t, o.raw, 0)
		o.whole = &stderrObservation{text: text, source: src, mask: mask}
	}
	return o.whole.text, o.whole.source, o.whole.mask
}
func (o *stderrObservation) frozenOutput() string {
	if len(o.raw) >= stderrTailBytes && strings.TrimSpace(string(o.raw[o.start:])) == "" {
		return fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(o.raw))
	}
	text := frozenToken.ReplaceAllString(o.text, "[REDACTED]")
	if len(text) > 4096 {
		start := len(text) - 4096
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		text = text[start:]
	}
	return text
}
func checkConstructed(t testing.TB, input string) string {
	t.Helper()
	return checkConstructedObserved(t, input).got
}
func checkConstructedObserved(t testing.TB, input string) *stderrObservation {
	t.Helper()
	var tail stderrTail
	tail.Write([]byte(input))
	raw := tail.data
	// Verify the streaming path's bytes independently. The pure sanitizer then
	// needs one call for these identical direct and streamed retained bytes.
	expected := []byte(input)
	if len(expected) > 4096 {
		expected = expected[len(expected)-4096:]
	}
	if !bytes.Equal(raw, expected) {
		t.Fatal("streaming window differs from direct retained bytes")
	}
	o := observeStderr(t, raw, sanitizedStderr(raw))
	want := fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(raw))
	if (o.got == want) != o.notice {
		t.Fatalf("notice iff for %q: %q", raw, o.got)
	}
	if !o.notice {
		var visible, shown [256]int
		for i, b := range []byte(o.text) {
			if !o.mask[o.source[i]] {
				visible[b]++
			}
		}
		for _, b := range []byte(o.got) {
			shown[b]++
		}
		for b := range visible {
			if strings.IndexByte("[REDACTED]", byte(b)) < 0 && shown[b] > visible[b] {
				t.Fatalf("legacy visibility byte %d raw %q output %q", b, raw, o.got)
			}
		}
	}
	if !utf8.ValidString(o.got) || len(o.got) > 4096 {
		t.Fatal("bounded UTF-8")
	}
	for _, r := range o.got {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			t.Fatal("output control")
		}
	}
	return o
}

const stderrDefaultGeneratedCases = 256

func stderrExhaustive() bool { return os.Getenv("STDERR_SWEEP_FULL") == "1" }

// Default sampling preserves both ends and the interior of every lexical run,
// every byte around quote/backslash/control runs, and a stride through longer
// interiors. Exhaustive mode retains every original byte offset.
func stderrSweepOffsets(input string) []int {
	selected := make([]bool, len(input)+1)
	add := func(i int) {
		if i >= 0 && i < len(selected) {
			selected[i] = true
		}
	}
	add(0)
	add(len(input))
	if stderrExhaustive() {
		for i := range selected {
			selected[i] = true
		}
	} else {
		class := func(b byte) byte {
			switch {
			case b == '"' || b == '\'':
				return 'q'
			case b == '\\':
				return 'b'
			case b < 32 || b == 127 || b >= 128:
				return 'c'
			case b == ' ' || b == '\t':
				return 'w'
			case b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-':
				return 'n'
			default:
				return b
			}
		}
		for i := 0; i < len(input); {
			j := i + 1
			c := class(input[i])
			for j < len(input) && class(input[j]) == c {
				j++
			}
			if c == 'q' || c == 'b' || c == 'c' {
				for p := i - 1; p <= j+1; p++ {
					add(p)
				}
			} else {
				add(i)
				add(i + 1)
				add(j - 1)
				add(j)
				for p := i + 16; p < j; p += 16 {
					add(p)
				}
			}
			// A retained suffix can still be a credential name after its vendor
			// prefix is evicted. Keep both sides of every such opener explicitly.
			if c == 'n' {
				name := strings.ToLower(input[i:j])
				for _, suffix := range []string{"password", "passwd", "secret", "credential", "credentials", "authorization", "cookie", "privatekey", "private_key", "private-key", "token", "apikey", "api_key", "api-key", "accesskey", "access_key", "secretkey", "secret_key"} {
					if strings.HasSuffix(name, suffix) {
						opener := j - len(suffix)
						add(opener - 1)
						add(opener)
						add(opener + 1)
					}
				}
				if len(name) > 4 && (strings.HasSuffix(name, "_key") || strings.HasSuffix(name, "-key")) {
					add(j - 6)
					add(j - 5)
					add(j - 4)
				}
			}
			i = j
		}
	}
	offsets := make([]int, 0, len(selected))
	for i, yes := range selected {
		if yes {
			offsets = append(offsets, i)
		}
	}
	return offsets
}

// NUL padding keeps a full raw window without repeatedly feeding thousands of
// irrelevant printable filler bytes to every regex. Exact printable-padding
// boundary cases remain in SW5/SW6, and exhaustive mode uses the original pads.
func stderrSweepPadding() string {
	if stderrExhaustive() {
		return "x"
	}
	return "\x00"
}

func stderrFullWindow(input string) string {
	return strings.Repeat(stderrSweepPadding(), 5000) + "\n" + input
}

// Every sampled offset runs directly. Full-window cases additionally retain
// every key/value byte-adjacency class and every sensitive suffix opener,
// instead of repeating that context for equivalent interiors of the same run.
type stderrOffsetClass struct {
	value       bool
	left, right byte
}

func stderrOffsetKind(b byte) byte {
	switch {
	case b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-':
		return 'n'
	case b == ' ' || b == '\t' || b == '\n':
		return 'w'
	default:
		return b
	}
}
func stderrOffsetClassAt(input string, offset int) stderrOffsetClass {
	separator := strings.IndexAny(input, ":= ")
	kind := stderrOffsetClass{value: separator >= 0 && offset > separator}
	if offset == 0 {
		kind.left = 255
	} else {
		kind.left = stderrOffsetKind(input[offset-1])
	}
	if offset == len(input) {
		kind.right = 255
	} else {
		kind.right = stderrOffsetKind(input[offset])
	}
	return kind
}
func stderrFullSampleOffsets(input string) map[int]bool {
	selected := map[int]bool{}
	seen := map[stderrOffsetClass]bool{}
	for _, offset := range stderrSweepOffsets(input) {
		class := stderrOffsetClassAt(input, offset)
		if stderrExhaustive() || !seen[class] {
			selected[offset] = true
			seen[class] = true
		}
	}
	// Keep shortest class-matching suffixes even inside a vendor-prefixed run.
	for _, run := range referenceNames.FindAllStringIndex(input, -1) {
		name := strings.ToLower(input[run[0]:run[1]])
		for _, suffix := range []string{"password", "passwd", "secret", "credential", "credentials", "authorization", "cookie", "privatekey", "private_key", "private-key", "token", "apikey", "api_key", "api-key", "accesskey", "access_key", "secretkey", "secret_key"} {
			if strings.HasSuffix(name, suffix) {
				opener := run[1] - len(suffix)
				for _, i := range []int{opener - 1, opener, opener + 1} {
					if i >= 0 && i <= len(input) {
						selected[i] = true
					}
				}
			}
		}
		if len(name) > 4 && (strings.HasSuffix(name, "_key") || strings.HasSuffix(name, "-key")) {
			for _, i := range []int{run[1] - 6, run[1] - 5, run[1] - 4} {
				if i >= 0 && i <= len(input) {
					selected[i] = true
				}
			}
		}
	}
	return selected
}

func stderrNewlineOffsets() []int {
	if stderrExhaustive() {
		out := make([]int, 129)
		for i := range out {
			out[i] = i - 64
		}
		return out
	}
	return []int{-64, -48, -32, -16, -2, -1, 0, 1, 2, 16, 32, 48, 64}
}

// expectedHidden aligns the exact reviewed output with its raw fixture. It
// derives hidden positions from the contract, never the production matcher.
func expectedHidden(t testing.TB, c redactCase) []bool {
	t.Helper()
	hidden := make([]bool, len(c.input))
	parts := strings.Split(c.want, "[REDACTED]")
	pos := 0
	for i, part := range parts {
		if i == 0 {
			if !strings.HasPrefix(c.input, part) {
				t.Fatalf("fixture prefix %q", c.input)
			}
			pos = len(part)
			continue
		}
		end := len(c.input)
		if i < len(parts)-1 || part != "" {
			next := strings.Index(c.input[pos:], part)
			if next < 0 {
				t.Fatalf("fixture suffix %q", c.input)
			}
			end = pos + next
		}
		for j := pos; j < end; j++ {
			hidden[j] = true
		}
		pos = end + len(part)
	}
	return hidden
}

type stderrSweepFixture struct {
	input, want string
	symbol      byte
	legacy      bool
}

func stderrSweepFixtures(t testing.TB) []stderrSweepFixture {
	t.Helper()
	var fixtures []stderrSweepFixture
	for _, c := range newShapeCases {
		if !strings.Contains(c.want, "[REDACTED]") {
			continue
		}
		hidden := expectedHidden(t, c)
		placeholder := func(i int) bool { return hidden[i] && (c.input[i] >= '0' && c.input[i] <= '9') }
		count := 0
		for i := range hidden {
			if placeholder(i) {
				count++
			}
		}
		if count == 0 {
			// Exact-output-only: no placeholder bytes in the required-hidden part.
			// These include expired, not-found prose and an unterminated name value.
			fixtures = append(fixtures, stderrSweepFixture{input: c.input, want: c.want})
			continue
		}
		outside := ""
		for i := range c.input {
			if !placeholder(i) {
				outside += c.input[i : i+1]
			}
		}
		symbol := byte(0)
		for i := range stderrSentinels {
			s := stderrSentinels[i]
			if !strings.ContainsRune(outside, rune(s)) && !strings.ContainsRune(c.want, rune(s)) {
				symbol = s
				break
			}
		}
		if symbol == 0 {
			t.Fatalf("no sentinel for %q", c.input)
		}
		input := []byte(c.input)
		for i := range input {
			if placeholder(i) {
				input[i] = symbol
			}
		}
		fixtures = append(fixtures, stderrSweepFixture{string(input), c.want, symbol, false})
	}
	for _, shape := range []string{"sk-1111", "ghp_1111", "gho_1111", "ghs_1111", "ghu_1111", "ghr_1111", "github_pat_1111", "Bearer 1111", "Authorization: 1111", "x-api-key: 1111", "access_token=1111", `{"access_token":"1111"}`, `{"api_key":"1111"}`, `{"token":"1111"}`, `{"refresh_token":"1111"}`, `{"id_token":"1111"}`, `{"client_secret":"1111"}`, `{"apiKey":"1111"}`} {
		fixtures = append(fixtures, stderrSweepFixture{shape, frozenStderr([]byte(shape)), '1', true})
	}
	return fixtures
}

// requiredDiagnosticHidden checks legacy masks separately from new quoted-key
// forms. In particular token":"1111 survives eviction of the opening quote.
func requiredDiagnosticHidden(t testing.TB, observation *stderrObservation) bool {
	raw, start, notice := observation.raw, observation.start, observation.notice
	if notice {
		return true
	}
	diagnostic := strings.LastIndex(string(raw), "useful")
	legacyHidden := false
	for _, base := range []int{start, 0} {
		text, src, mask := observation.region(t, base)
		if diagnostic < len(mask) && mask[diagnostic] {
			legacyHidden = true
		}
		// Independent, deliberately simple quoted-key parser: only this form can
		// carry a new mask through the first newline into the diagnostic.
		for _, name := range referenceNames.FindAllStringIndex(text, -1) {
			if !referenceSensitiveName.MatchString(text[name[0]:name[1]]) {
				continue
			}
			pos := name[1]
			token := func() (byte, int, bool) {
				begin := pos
				for pos < len(text) && text[pos] == '\\' {
					pos++
				}
				if pos < len(text) && (text[pos] == '"' || text[pos] == '\'') {
					q := text[pos]
					k := pos - begin
					pos++
					return q, k, true
				}
				return 0, 0, false
			}
			if _, _, ok := token(); !ok {
				continue
			}
			for pos < len(text) && stderrSpace(text[pos]) {
				pos++
			}
			if pos == len(text) || text[pos] != ':' {
				continue
			}
			pos++
			for pos < len(text) && stderrSpace(text[pos]) {
				pos++
			}
			q, k, ok := token()
			if !ok {
				continue
			}
			valueStart := pos
			end := len(text)
			slashes := 0
			for j := pos; j < len(text); j++ {
				if text[j] == q && (k == 0 && slashes%2 == 0 || k > 0 && slashes == k) {
					end = j + 1
					break
				}
				if text[j] == '\\' {
					slashes++
				} else {
					slashes = 0
				}
			}
			if valueStart < end && src[valueStart] <= diagnostic && src[end-1] >= diagnostic {
				return true
			}
		}
	}
	return legacyHidden
}

func TestStderrSweeps(t *testing.T) {
	fixtures := stderrSweepFixtures(t)
	t.Run("SW1", func(t *testing.T) {
		windowFixtures := append([]stderrSweepFixture{}, fixtures...)
		windowFixtures = append(windowFixtures, stderrSweepFixture{input: `{"token":"1111`}, stderrSweepFixture{input: `{"password": "1111`})
		for _, f := range windowFixtures {
			for _, offset := range stderrSweepOffsets(f.input) {
				suffix := f.input[offset:] + "\nuseful"
				pad := "\x00"
				if stderrExhaustive() {
					pad = " "
				}
				raw := []byte(suffix + strings.Repeat(pad, 4096-len(suffix)))
				observation := checkConstructedObserved(t, strings.Repeat(stderrSweepPadding(), 5000)+string(raw))
				// Keep the legacy-only applicability cross-check distinct from the new
				// quoted-key forms, whose name can survive its opening quote's eviction.
				if f.input == `{"token":"1111` {
					_, _, mask := observation.region(t, 0)
					legacyFlag := offset <= strings.IndexByte(f.input, '"')
					if mask[strings.Index(string(raw), "useful")] != legacyFlag {
						t.Fatal("legacy flag/reference drift")
					}
				}
				hide := requiredDiagnosticHidden(t, observation)
				got := observation.got
				if strings.Contains(got, "useful") == hide {
					t.Fatalf("diagnostic coverage offset %d fixture %q hidden %v got %q", offset, f.input, hide, got)
				}
			}
		}
	})
	for _, group := range []struct {
		name    string
		inserts []string
	}{{"SW2", []string{"\r"}}, {"SW3", []string{"\x1b[0m", "\x1b]0;t\x07"}}, {"SW4", []string{"\x00", "\x07", "\x7f", "\xc2\x85"}}} {
		t.Run(group.name, func(t *testing.T) {
			for _, f := range fixtures {
				fullOffsets := stderrFullSampleOffsets(f.input)
				for _, insert := range group.inserts {
					for _, offset := range stderrSweepOffsets(f.input) {
						input := f.input[:offset] + insert + f.input[offset:]
						checkConstructed(t, input)
						if fullOffsets[offset] {
							checkConstructed(t, stderrFullWindow(input))
						}
					}
				}
			}
		})
	}
	t.Run("SW5", func(t *testing.T) {
		for _, f := range fixtures {
			for _, offset := range stderrNewlineOffsets() {
				pad := "\x00"
				if stderrExhaustive() || offset == 0 {
					pad = " "
				}
				input := strings.Repeat(stderrSweepPadding(), 4096+offset) + "\n" + f.input + strings.Repeat(pad, 8192-(4096+offset+1+len(f.input)))
				checkConstructed(t, input)
			}
		}
	})
	t.Run("SW6", func(t *testing.T) {
		for _, size := range []int{4095, 4096, 4097} {
			for _, ending := range []string{" ", "\t", "\xc2\x85", "\xc2\xa0", "\x00", "\x1b[0m"} {
				for _, first := range []bool{false, true} {
					input := strings.Repeat("x", size-len(ending)-1) + "\n" + ending
					if first {
						input = "\n" + strings.Repeat(" ", size-len(ending)-1) + ending
					}
					got := checkConstructed(t, input)
					var tail stderrTail
					tail.Write([]byte(input))
					want := frozenStderr(tail.data)
					// Full tails also get a notice when cleanup or final-frame
					// selection leaves no content.
					if _, notice := referenceStart(tail.data); notice {
						want = fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(tail.data))
					}
					if got != want {
						t.Fatalf("SW6 %q", got)
					}
				}
			}
		}
		for _, size := range []int{4095, 4096, 4097} {
			checkConstructed(t, strings.Repeat("x", size-1)+"\n")
		}
	})
	t.Run("SW7", func(t *testing.T) {
		const digits = "4096 0 9 6"
		if got := checkConstructed(t, strings.Repeat("x", 5000)+"\n"+digits); got != digits {
			t.Fatalf("digits %q", got)
		}
	})
	t.Run("SW8", func(t *testing.T) {
		for _, f := range fixtures {
			if f.symbol == 0 {
				continue
			}
			for spaceIndex, space := range []string{"", " "} {
				for offsetIndex, offset := range stderrSweepOffsets(f.input) {
					if !stderrExhaustive() && offsetIndex%2 != spaceIndex {
						continue
					}
					// Default mode pairs the spacing and cut dimensions rather than
					// repeating their full Cartesian product; every fixture retains both
					// spacing classes and all sampled cut classes. Exhaustive runs both.
					// The cut of unredacted normalized text lands at offset in F.
					base := len(f.input) + len(space)
					p := (4096 - base + offset) / 3
					r := 4096 + offset - base - 3*p
					input := f.input + space + strings.Repeat("\xff", p) + strings.Repeat("x", r)
					observation := checkConstructedObserved(t, input)
					normalized := observation.text
					if len(input) >= 4096 || len(normalized)-4096 != offset {
						t.Fatal("SW8 construction")
					}
					got := observation.got
					if strings.ContainsRune(got, rune(f.symbol)) {
						t.Fatalf("SW8 symbol %q input %q", f.symbol, input)
					}
					if f.legacy && got != observation.frozenOutput() {
						t.Fatalf("SW8 legacy output %q", got)
					}
				}
			}
		}
		if got := checkConstructed(t, "Bearer "+strings.Repeat("\xff", 1366)+"11"); got != "[REDACTED]" {
			t.Fatalf("SW8 killing fixture %q", got)
		}
	})
	t.Run("SW8u", func(t *testing.T) {
		for _, r := range []string{"é", "€", "\xef\xbf\xbd", "\xff"} {
			for offset := -8; offset <= 8; offset++ {
				for _, input := range []string{strings.Repeat("x", 4096+offset) + r + "\nuseful", "\n" + strings.Repeat("\xff", 1360) + strings.Repeat("x", 16+offset) + r} {
					got := checkConstructed(t, input)
					var tail stderrTail
					tail.Write([]byte(input))
					if got != frozenStderr(tail.data) {
						t.Fatalf("SW8u %q", got)
					}
				}
			}
		}
	})
	t.Run("SW9", func(t *testing.T) {
		for _, f := range fixtures {
			if f.legacy {
				continue
			}
			if got := checkConstructed(t, f.input); got != f.want {
				t.Fatalf("SW9 baseline %q: %q want %q", f.input, got, f.want)
			}
			fullOffsets := stderrFullSampleOffsets(f.input)
			for _, insert := range []string{"\x1b[0m", "\x1b]0;t\x07", "\x00", "\x7f", "\xc2\x85", "\r"} {
				for _, offset := range stderrSweepOffsets(f.input) {
					input := f.input[:offset] + insert + f.input[offset:]
					variants := []string{input}
					if fullOffsets[offset] {
						variants = append(variants, stderrFullWindow(input))
					}
					for _, variant := range variants {
						observation := checkConstructedObserved(t, variant)
						if observation.text != f.input {
							t.Fatal("SW9 transform precondition")
						}
						got := observation.got
						if f.symbol != 0 && strings.ContainsRune(got, rune(f.symbol)) {
							t.Fatalf("SW9 symbol %q for %q", f.symbol, input)
						}
						if insert != "\r" && got != f.want {
							t.Fatalf("SW9 equality %q got %q want %q", input, got, f.want)
						}
					}
				}
			}
			input := strings.ReplaceAll(f.input, "\n", "\r\n")
			norm, _, _ := referenceRegion(t, []byte(input), 0)
			if norm != f.input {
				t.Fatal("CRLF precondition")
			}
			got := checkConstructed(t, input)
			if f.symbol != 0 && strings.ContainsRune(got, rune(f.symbol)) {
				t.Fatal("SW9 CRLF")
			}
		}
	})
}

var referenceNames = regexp.MustCompile(`[A-Za-z0-9_-]+`)
var referenceSensitiveName = regexp.MustCompile(`(?i)(?:password|passwd|secret|credentials?|authorization|cookie|privatekey|private_key|private-key|token|apikey|api_key|api-key|accesskey|access_key|secretkey|secret_key|.+_key|.+-key)$`)

func TestStderrGeneratedSuperset(t *testing.T) {
	rng := rand.New(rand.NewSource(1704))
	shapes := []string{"sk-%s", "ghp_%s", "Bearer %s", "access_token=%s&ok=yes", `{"token":"%s"}`, "api_key=%s", "password: %s", "--token %s", "https://u:%s@example.test/"}
	noises := []string{"", "\x1b[0m", "\x1b]0;t\x07", "\r", "\r\n", "\x00", "\x7f", "\xff", " useful ", "\x1b]0;"}
	classes := map[string]int{}
	shapeSeen := make([]bool, len(shapes))
	noiseSeen := make([]bool, len(noises))
	sizeSeen := make([]bool, 6)
	cases := stderrDefaultGeneratedCases
	if stderrExhaustive() {
		cases = 20000
	}
	for iteration := 0; iteration < cases; iteration++ {
		var input strings.Builder
		instances := 1 + rng.Intn(9)
		symbols := stderrSentinels[:instances]
		for j := 0; j < instances; j++ {
			symbol := symbols[j]
			value := strings.Repeat(string(symbol), 1+rng.Intn(8))
			shapeIndex := rng.Intn(len(shapes))
			shapeSeen[shapeIndex] = true
			shape := fmt.Sprintf(shapes[shapeIndex], value)
			at := rng.Intn(len(shape) + 1)
			noiseIndex := rng.Intn(len(noises))
			noiseSeen[noiseIndex] = true
			input.WriteString(noises[noiseIndex])
			input.WriteString(shape[:at])
			noiseIndex = rng.Intn(len(noises))
			noiseSeen[noiseIndex] = true
			input.WriteString(noises[noiseIndex])
			input.WriteString(shape[at:])
			input.WriteString("\n")
		}
		stream := input.String()
		// An entire instance inside a terminated OSC produces ERASED rather than
		// merely modifying its grammar. Its sentinel still belongs to that instance.
		if iteration%13 == 0 {
			stream = "\x1b]0;" + stream + "\x07useful\n"
		}

		// Prefix and suffix sizes cover empty, short, exact-window and evicted data.
		if iteration%20 == 0 {
			sizeIndex := rng.Intn(6)
			// Stratify default size classes so a bounded sample cannot omit one.
			// Exhaustive mode preserves the original fixed-seed random sequence.
			if !stderrExhaustive() {
				sizeIndex = iteration / 20 % 6
			}
			sizeSeen[sizeIndex] = true
			size := []int{0, 4095, 4096, 4097, 8192, 20000}[sizeIndex]
			if size < len(stream) {
				stream = stream[:size]
			} else {
				padding := size - len(stream)
				before := rng.Intn(padding + 1)
				stream = strings.Repeat("x", before) + stream + strings.Repeat("x", padding-before)
			}
		}
		// Each sentinel was introduced only within its own instance. None of the
		// grammar, noise or filler strings contains any sentinel.
		for j := 0; j < instances; j++ {
			if strings.ContainsRune("useful example.test x", rune(symbols[j])) {
				t.Fatal("generator sentinel collision")
			}
		}
		var tail stderrTail
		for pos := 0; pos < len(stream); {
			n := 1 + rng.Intn(97)
			if iteration%7 == 0 {
				n = 1
			}
			if iteration%11 == 0 {
				n = len(stream)
			}
			if n > len(stream)-pos {
				n = len(stream) - pos
			}
			tail.Write([]byte(stream[pos : pos+n]))
			pos += n
		}
		raw := tail.data
		observation := checkConstructedObserved(t, stream)
		got := observation.got
		checkMainSupersetObserved(t, observation)
		if !bytes.Equal(raw, observation.raw) {
			t.Fatal("chunk-dependent tail")
		}
		start, notice := observation.start, observation.notice
		if notice {
			continue
		}
		text, src, mask := observation.text, observation.source, observation.mask
		for j := 0; j < instances; j++ {
			symbol := symbols[j]
			rawCount := bytes.Count(raw[start:], []byte{symbol})
			normalizedCount, maskedCount, visibleCount := 0, 0, 0
			for i, b := range []byte(text) {
				if b == symbol {
					normalizedCount++
					if mask[src[i]] {
						maskedCount++
					} else {
						visibleCount++
					}
				}
			}
			class := "MIXED"
			switch {
			case rawCount == 0:
				class = "DROPPED"
			case normalizedCount == 0:
				class = "ERASED"
			case visibleCount == 0 && maskedCount > 0:
				class = "HIDDEN"
			}
			classes[class]++
			if strings.Count(got, string(symbol)) > visibleCount {
				t.Fatalf("%s instance %q raw %q output %q visible %d", class, symbol, raw, got, visibleCount)
			}
		}
	}
	for class, seen := range map[string][]bool{"shape": shapeSeen, "noise": noiseSeen, "size": sizeSeen} {
		for i, yes := range seen {
			if !yes {
				t.Fatalf("generator missed %s class %d", class, i)
			}
		}
	}
	for _, class := range []string{"DROPPED", "ERASED", "HIDDEN", "MIXED"} {
		if classes[class] == 0 {
			t.Fatalf("generator missed %s", class)
		}
	}
	t.Logf("cases: %d; instance classes: %v", cases, classes)
}

func FuzzSanitizedStderrSuperset(f *testing.F) {
	for _, ending := range []string{"progress 50%\r", "progress 50%\r\x1b[0m", "password: 1111\r"} {
		f.Add([]byte(strings.Repeat("x", 5000) + "\n" + ending))
	}
	for _, cases := range portedDirectCases {
		for _, c := range cases {
			f.Add([]byte(c.input))
		}
	}
	for _, input := range []string{"sk-11é77", "sk-11\xc3\x1b[0m\xa977", "sk-11\xff77", "ghp_11-77", "Bearer 11\t77", "access_token=11&77", `"token": "11\\"77`, "Bearer \xef\xbf\xbd11 ok"} {
		f.Add([]byte(input))
	}

	for _, c := range newShapeCases {
		f.Add([]byte(c.input))
	}
	for _, input := range legacyGuardInputs() {
		f.Add([]byte(input))
	}
	for _, input := range []string{"\xc3\x1b[0m\xa9", "Bearer \xff11 ok", "\x1b]0;t\nx\x07y", "\x1b[0m\x1b[0msk-1111 ok", "--token =1111", "secret: \"1111\" useful", "xſk-11", "ſk-11", "[stderr withheld: 4096 bytes with no complete line]"} {
		f.Add([]byte(input))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		var tail stderrTail
		tail.Write(input)
		raw := tail.data
		// Cleaned content and frame selection choose the notice branch,
		// even on a literal notice collision.
		start, notice := referenceStart(raw)
		got := sanitizedStderr(raw)
		if notice {
			want := fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(raw))
			if got != want {
				t.Fatalf("notice %q", got)
			}
			return
		}
		text, _, _ := referenceRegion(t, raw[start:], start)
		uncut := frozenToken.ReplaceAllString(text, "[REDACTED]")
		for b := 0; b < 256; b++ {
			if strings.IndexByte("[REDACTED]", byte(b)) >= 0 {
				continue
			}
			if bytes.Count([]byte(got), []byte{byte(b)}) > bytes.Count([]byte(uncut), []byte{byte(b)}) {
				t.Fatalf("byte %d increased input %q got %q", b, raw, got)
			}
		}
		checkMainSuperset(t, raw)
	})
}

func stderrAdversarialInputs() map[string]string {
	distinct := ""
	for k := 1; len(distinct)+k+7 < 4095; k++ {
		distinct += "token:" + strings.Repeat("\\", k) + "\""
	}
	return map[string]string{
		"invalid":          strings.Repeat("\xff", 4095),
		"ansi":             strings.Repeat("\x1b[m", 1365),
		"legacy":           strings.Repeat("sk-x ", 819),
		"alternating":      strings.Repeat("sk-x\xff", 819),
		"distinct-quotes":  distinct,
		"candidates":       strings.Repeat("token=", 682),
		"schemes":          strings.Repeat("a://", 1024),
		"schemes-short":    strings.Repeat("a://", 1023),
		"full-schemes":     "\n" + strings.Repeat("a://", 1023) + "xxx",
		"userinfo":         strings.Repeat("https://a@", 409),
		"quoted":           strings.Repeat(`"token":"\`, 409),
		"cr":               strings.Repeat("x\r", 2047),
		"full-ansi":        "\n" + strings.Repeat("\x1b[m", 1365),
		"full-legacy":      "\n" + strings.Repeat("sk-x ", 819),
		"full-alternating": "\n" + strings.Repeat("sk-x\xff", 819),
		"authorization":    "Authorization: " + strings.Repeat("x", 4000),
		"json":             `{"token":"` + strings.Repeat("x", 4000),
		"cookie":           "Cookie: " + strings.Repeat("x ", 2000),
		"form":             "token=" + strings.Repeat("x", 4000),
		"url":              "https://u:" + strings.Repeat("x", 3900) + "@example.test/",
		"osc":              "\x1b]0;" + strings.Repeat("x", 4000) + "\x07useful",
		"multiline":        strings.Repeat("useful\n", 585),
		"plain":            strings.Repeat("x", 4095),
	}
}

func TestSanitizeAllocBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("race instrumentation and randomized sync.Pool eviction distort regexp allocations; run this unchanged 1 MiB gate without -race")
	}

	inputs := stderrAdversarialInputs()
	for _, name := range []string{"full-ansi", "full-legacy", "full-alternating"} {
		input := []byte(inputs[name])
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				sanitizedStderr(input)
			}
		})
		t.Logf("%s: %d B/op %d allocs/op", name, result.AllocedBytesPerOp(), result.AllocsPerOp())
		if result.AllocedBytesPerOp() > 1<<20 {
			t.Fatalf("allocation budget %s: %d B/op", name, result.AllocedBytesPerOp())
		}
	}
}
func BenchmarkSanitizeAdversarial(b *testing.B) {
	for name, input := range stderrAdversarialInputs() {
		b.Run(name, func(b *testing.B) {
			raw := []byte(input)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sanitizedStderr(raw)
			}
		})
	}
}
func BenchmarkSanitizeTypical(b *testing.B) {
	raw := []byte("Error: quota exceeded; retry later")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sanitizedStderr(raw)
	}
}
func BenchmarkTailWrite(b *testing.B) {
	for name, input := range map[string]string{"one-byte": "x", "64KiB": strings.Repeat("x", 65536), "cr": strings.Repeat("x\r", 2048)} {
		b.Run(name, func(b *testing.B) {
			var tail stderrTail
			raw := []byte(input)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tail.Write(raw)
			}
		})
	}
}

func TestStderrSweepOffsetCoverage(t *testing.T) {
	input := "token=\"" + strings.Repeat("1", 80) + "\\\"11\"\x1b[0m\r\nuseful"
	offsets := stderrSweepOffsets(input)
	selected := make(map[int]bool, len(offsets))
	for _, offset := range offsets {
		selected[offset] = true
	}
	// These positions exercise opener, delimiter, quote/escape, CSI and CR/LF
	// boundaries independently of the selector's lexical-run implementation.
	required := []int{0, 1, 4, 5, 6, 7, len(input) - 1, len(input)}
	for i, b := range []byte(input) {
		if b == '"' || b == '\\' || b == '\x1b' || b == '\r' || b == '\n' {
			for _, offset := range []int{i - 1, i, i + 1} {
				if offset >= 0 && offset <= len(input) {
					required = append(required, offset)
				}
			}
		}
	}
	for _, offset := range required {
		if !selected[offset] {
			t.Fatalf("boundary %d omitted", offset)
		}
	}
	for i := 1; i < len(offsets); i++ {
		if offsets[i]-offsets[i-1] > 16 {
			t.Fatal("stride exceeded")
		}
	}
	if stderrExhaustive() && len(offsets) != len(input)+1 {
		t.Fatal("exhaustive offsets omitted")
	}
	for _, fixture := range []struct{ input, suffix string }{{`"AUTH_TOKEN":"1111`, "TOKEN"}, {`DB_PASSWORD=1111`, "PASSWORD"}, {`ANTHROPIC_API_KEY=1111`, "API_KEY"}, {`CACHE_KEY=1111`, "E_KEY"}} {
		points := map[int]bool{}
		for _, offset := range stderrSweepOffsets(fixture.input) {
			points[offset] = true
		}
		opener := strings.Index(fixture.input, fixture.suffix)
		for _, offset := range []int{opener - 1, opener, opener + 1} {
			if !points[offset] {
				t.Fatalf("credential suffix opener %q offset %d omitted", fixture.input, offset)
			}
		}
	}
	for _, want := range []int{-64, -2, -1, 0, 1, 2, 64} {
		found := false
		for _, got := range stderrNewlineOffsets() {
			found = found || got == want
		}
		if !found {
			t.Fatalf("newline boundary %d omitted", want)
		}
	}
}

func TestStderrObservationMatchesFrozen(t *testing.T) {
	inputs := legacyGuardInputs()
	for _, c := range newShapeCases {
		inputs = append(inputs, c.input)
	}
	inputs = append(inputs, "Bearer \xff11 ok", "\xc3\x1b[0m\xa9", strings.Repeat("\xff", 4095))
	for _, input := range inputs {
		var tail stderrTail
		tail.Write([]byte(input))
		observation := observeStderr(t, tail.data, "")
		if got, want := observation.frozenOutput(), frozenStderr(tail.data); got != want {
			t.Fatalf("shared frozen output %q != %q for %q", got, want, tail.data)
		}
	}
}

func TestStderrFullOffsetCoverage(t *testing.T) {
	for _, fixture := range stderrSweepFixtures(t) {
		full := stderrFullSampleOffsets(fixture.input)
		classes := map[stderrOffsetClass]bool{}
		for offset := range full {
			classes[stderrOffsetClassAt(fixture.input, offset)] = true
		}
		for _, offset := range stderrSweepOffsets(fixture.input) {
			if !classes[stderrOffsetClassAt(fixture.input, offset)] {
				t.Fatalf("full-window offset class lost for %q at %d", fixture.input, offset)
			}
		}
		if stderrExhaustive() && len(full) != len(fixture.input)+1 {
			t.Fatal("exhaustive full context offset lost")
		}
	}
	for _, fixture := range []struct{ input, suffix string }{{`"AUTH_TOKEN":"1111`, "TOKEN"}, {`DB_PASSWORD=1111`, "PASSWORD"}, {`ANTHROPIC_API_KEY=1111`, "API_KEY"}, {`CACHE_KEY=1111`, "E_KEY"}} {
		full := stderrFullSampleOffsets(fixture.input)
		opener := strings.Index(fixture.input, fixture.suffix)
		for _, offset := range []int{opener - 1, opener, opener + 1} {
			if !full[offset] {
				t.Fatalf("full-window suffix boundary lost at %d", offset)
			}
		}
	}
}
