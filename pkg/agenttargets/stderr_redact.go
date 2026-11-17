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
		maskStderrNew(diff, region)
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
	return renderStderrMasked(raw, start, region, mask)
}

// Rendering hides every byte main's uncut redaction hides. The final 4096-byte
// cut can keep text that main's cut dropped when additional masking shortens it.
func renderStderrMasked(raw []byte, start int, region stderrRegion, mask []int32) string {
	frames := stderrFrameStarts(raw, start, region.ansi)
	var out strings.Builder
	out.Grow(len(region.text))
	hiddenRun := false
	emitted := false
	for i := 0; i < len(region.text); {
		_, n := utf8.DecodeRune(region.text[i:])
		hidden := false
		selected, anySelected := true, false
		for _, p := range region.source[i : i+n] {
			hidden = hidden || mask[p] > 0
			after := p >= frames[p]
			selected = selected && after
			anySelected = anySelected || after
		}
		if hidden {
			if !hiddenRun {
				emitted = false
			}
			// Delay a hidden run's marker until its first source in the selected
			// frame. A hidden run straddling CR still emits just one marker.
			if anySelected && !emitted {
				out.WriteString("[REDACTED]")
				emitted = true
			}
		} else if selected {
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

func stderrBlank(b byte) bool { return b == ' ' || b == '\t' }
func stderrSpace(b byte) bool { return stderrBlank(b) || b == '\n' }
func stderrNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

type stderrNameKind uint8

const (
	stderrNameNone stderrNameKind = iota
	stderrNameStrong
	stderrNameWeak
	stderrNameKeySuffix
)

type stderrValueKind uint8

const (
	stderrValueEquals stderrValueKind = iota
	stderrValueScalar
	stderrValueWord
	stderrValueLine
)

func stderrNameClass(name []byte) stderrNameKind {
	lower := strings.TrimLeft(strings.ToLower(string(name)), "-")
	for _, suffix := range []string{"password", "passwd", "secret", "credential", "credentials", "authorization", "cookie", "privatekey", "private_key", "private-key"} {
		if strings.HasSuffix(lower, suffix) {
			return stderrNameStrong
		}
	}
	for _, suffix := range []string{"token", "apikey", "api_key", "api-key", "accesskey", "access_key", "secretkey", "secret_key"} {
		if strings.HasSuffix(lower, suffix) {
			return stderrNameWeak
		}
	}
	if len(lower) > 4 && (strings.HasSuffix(lower, "_key") || strings.HasSuffix(lower, "-key")) {
		return stderrNameKeySuffix
	}
	return stderrNameNone
}

type stderrQuoteClass struct {
	quote   byte
	slashes int
}
type stderrQuoteIndex map[stderrQuoteClass][]int32

func indexStderrQuotes(text []byte) stderrQuoteIndex {
	counts := make(map[stderrQuoteClass]int)
	walk := func(visit func(stderrQuoteClass, int)) {
		slashes := 0
		for i, b := range text {
			if b == '\'' || b == '"' {
				visit(stderrQuoteClass{b, slashes}, i)
				if slashes%2 == 0 {
					visit(stderrQuoteClass{b, -1}, i)
				}
			}
			if b == '\\' {
				slashes++
			} else {
				slashes = 0
			}
		}
	}
	walk(func(k stderrQuoteClass, _ int) { counts[k]++ })
	index := make(stderrQuoteIndex, len(counts))
	for k, n := range counts {
		index[k] = make([]int32, 0, n)
	}
	walk(func(k stderrQuoteClass, i int) { index[k] = append(index[k], int32(i)) })
	return index
}
func (index stderrQuoteIndex) close(quote byte, slashes, start, end int) int {
	class := slashes
	if slashes == 0 {
		class = -1
	}
	bucket := index[stderrQuoteClass{quote, class}]
	lo, hi := 0, len(bucket)
	for lo < hi {
		m := (lo + hi) / 2
		if int(bucket[m]) < start {
			lo = m + 1
		} else {
			hi = m
		}
	}
	if lo == len(bucket) || int(bucket[lo]) >= end {
		return end
	}
	return int(bucket[lo]) + 1
}

// Each stop-class cursor is queried in nondecreasing value-start order. It
// advances at most once over each byte, including overlapping candidates.
type stderrStopCursor struct {
	pos, last   int
	failed      bool
	failedStart int
}

// Tests install a strict assertion during initialization; production leaves it nil.
var stderrStopOrderCheck func(start, last int)

func (cursor *stderrStopCursor) end(text []byte, start int, stop func(byte) bool) int {
	return cursor.endChecked(text, start, stop, stderrStopOrderCheck)
}

func (cursor *stderrStopCursor) endChecked(text []byte, start int, stop func(byte) bool, check func(int, int)) int {
	if start < cursor.last {
		if !cursor.failed || start < cursor.failedStart {
			cursor.failedStart = start
		}
		cursor.failed = true
		if check != nil {
			check(start, cursor.last)
		}
		// The caller masks [start, end). Never trust a forward cursor for an
		// earlier start: hiding the entire remainder preserves the safety bound.
		return len(text)
	}
	cursor.last = start
	if cursor.pos < start {
		cursor.pos = start
	}
	for cursor.pos < len(text) && !stop(text[cursor.pos]) {
		cursor.pos++
	}
	return cursor.pos
}

// Apply the fallback independently of how a caller uses the returned end
// (for example, a line end also caps quoted values).
func maskStderrCursorFallback(diff []int32, region stderrRegion, cursors ...*stderrStopCursor) {
	for _, cursor := range cursors {
		if cursor.failed {
			maskStderrSpan(diff, region, cursor.failedStart, len(region.text))
		}
	}
}

func maskStderrNew(diff []int32, region stderrRegion) {
	text := region.text
	quotes := indexStderrQuotes(text)
	var eq, scalar, word, line stderrStopCursor
	quoteToken := func(pos int) (int, int, byte, bool) {
		begin := pos
		for pos < len(text) && text[pos] == '\\' {
			pos++
		}
		if pos < len(text) && (text[pos] == '"' || text[pos] == '\'') {
			return pos + 1, pos - begin, text[pos], true
		}
		return begin, 0, 0, false
	}
	skip := func(pos int, multiline bool) int {
		for pos < len(text) && (stderrBlank(text[pos]) || multiline && text[pos] == '\n') {
			pos++
		}
		return pos
	}
	value := func(start int, kind stderrValueKind, multiline bool) {
		if start >= len(text) || text[start] == '\n' {
			return
		}
		cap := len(text)
		if !multiline {
			cap = line.end(text, start, func(b byte) bool { return b == '\n' })
		}
		after, k, q, quoted := quoteToken(start)
		end := start
		if kind == stderrValueLine {
			end = cap
		} else if quoted {
			end = quotes.close(q, k, after, cap)
		} else {
			switch kind {
			case stderrValueEquals:
				end = eq.end(text, start, func(b byte) bool { return stderrSpace(b) || b == '&' })
			case stderrValueScalar:
				end = scalar.end(text, start, func(b byte) bool { return stderrSpace(b) || b == ',' || b == '}' || b == ']' })
			case stderrValueWord:
				end = word.end(text, start, stderrSpace)
			}
		}
		maskStderrSpan(diff, region, start, end)
	}
	for i := 0; i < len(text); {
		if !stderrNameByte(text[i]) {
			i++
			continue
		}
		begin := i
		for i < len(text) && stderrNameByte(text[i]) {
			i++
		}
		class := stderrNameClass(text[begin:i])
		if class == stderrNameNone {
			continue
		}

		type form struct {
			start     int
			kind      stderrValueKind
			multiline bool
		}
		var forms [3]form
		count := 0
		add := func(start int, kind stderrValueKind, multiline bool) {
			forms[count] = form{start, kind, multiline}
			count++
		}
		close, _, _, hasQuote := quoteToken(i)
		// The optional key close quote is part of the separator, never the value.
		keyEnd := i
		if hasQuote {
			keyEnd = close
		}
		sep := skip(keyEnd, false)
		if sep < len(text) && text[sep] == '=' {
			start := skip(sep+1, false)
			add(start, stderrValueEquals, false)
		}
		if hasQuote {
			sep = skip(close, true)
			if sep < len(text) && text[sep] == ':' {
				start := skip(sep+1, true)
				add(start, stderrValueScalar, true)
			}
		} else if class != stderrNameKeySuffix {
			sep = skip(i, false)
			if sep < len(text) && text[sep] == ':' {
				start := skip(sep+1, false)
				kind := stderrValueWord
				if class == stderrNameStrong {
					kind = stderrValueLine
				}
				add(start, kind, false)
			}
		}
		if class != stderrNameKeySuffix && i-begin >= 2 && text[begin] == '-' && text[begin+1] == '-' && i < len(text) && stderrBlank(text[i]) {
			add(skip(i, false), stderrValueWord, false)
		}
		for j := 1; j < count; j++ {
			for k := j; k > 0 && forms[k].start < forms[k-1].start; k-- {
				forms[k], forms[k-1] = forms[k-1], forms[k]
			}
		}
		for _, f := range forms[:count] {
			value(f.start, f.kind, f.multiline)
		}
	}
	maskStderrCursorFallback(diff, region, &eq, &scalar, &word, &line)
	maskStderrURLs(diff, region)
}

func maskStderrURLs(diff []int32, region stderrRegion) {
	text := region.text
	// Last @ in each non-whitespace run, and last @ before each position.
	lastAt := make([]int32, len(text))
	previousAt := make([]int32, len(text)+1)
	previousAt[0] = -1
	for i, b := range text {
		previousAt[i+1] = previousAt[i]
		if b == '@' {
			previousAt[i+1] = int32(i)
		}
	}
	last := int32(-1)
	for i := len(text) - 1; i >= 0; i-- {
		if stderrSpace(text[i]) {
			last = -1
		} else if text[i] == '@' && last < 0 {
			last = int32(i)
		}
		lastAt[i] = last
	}
	var run, colon, delimiter stderrStopCursor
	schemeByte := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '+' || b == '.' || b == '-'
	}
	for i := 0; i+2 < len(text); i++ {
		if text[i] != ':' || text[i+1] != '/' || text[i+2] != '/' {
			continue
		}
		letter := false
		for p := i - 1; p >= 0 && schemeByte(text[p]); p-- {
			letter = letter || text[p] >= 'a' && text[p] <= 'z' || text[p] >= 'A' && text[p] <= 'Z'
		}
		if !letter {
			continue
		}
		start := i + 3
		if start == len(text) {
			continue
		}
		end := run.end(text, start, stderrSpace)
		c := colon.end(text, start, func(b byte) bool { return b == ':' || stderrSpace(b) })
		d := delimiter.end(text, start, func(b byte) bool { return b == '/' || b == '?' || b == '#' || stderrSpace(b) })
		last := int(lastAt[start])
		if c < end && last > c {
			maskStderrSpan(diff, region, c+1, last)
		} else {
			at := int(previousAt[d])
			if at >= start {
				maskStderrSpan(diff, region, start, at)
			}
		}
	}
	maskStderrCursorFallback(diff, region, &run, &colon, &delimiter)
}

// stderrFrameStarts records the last CR outside ANSI for each raw line. One
// CR immediately before LF is a terminator, not a new (empty) frame. The ANSI
// spans are those already found while normalizing region (a).
func stderrFrameStarts(raw []byte, start int, ansi [][]int) []int32 {
	frames := make([]int32, len(raw))
	lineStart, cutoff, next := start, start, 0
	fill := func(end int) {
		for p := lineStart; p < end; p++ {
			frames[p] = int32(cutoff)
		}
	}
	for i := start; i < len(raw); i++ {
		for next < len(ansi) && i >= start+ansi[next][1] {
			next++
		}
		outside := next == len(ansi) || i < start+ansi[next][0]
		if raw[i] == '\r' && outside && (i+1 == len(raw) || raw[i+1] != '\n') {
			cutoff = i + 1
		}
		if raw[i] == '\n' {
			fill(i + 1)
			lineStart = i + 1
			cutoff = lineStart
		}
	}
	fill(len(raw))
	return frames
}
