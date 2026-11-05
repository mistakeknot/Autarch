//go:build unix

package homerun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// maxJCSInt is the largest integer magnitude that survives an IEEE-754 double, the bound RFC 8785 relies
// on. The envelope has no fractional numbers, so anything else is refused rather than formatted.
const maxJCSInt = 1<<53 - 1

var jcsInt = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// Canonicalize re-encodes one JSON value in RFC 8785 form: object keys sorted by UTF-16 code units, no
// insignificant whitespace, the JCS string escapes, integers only. It refuses non-integer numbers, invalid
// UTF-8, and trailing data.
func Canonicalize(raw []byte) ([]byte, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("canonicalize: input is not valid UTF-8")
	}
	if err := checkSurrogates(raw); err != nil {
		return nil, err
	}
	if err := checkNoDuplicateKeys(raw); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("canonicalize: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("canonicalize: trailing data after the JSON value")
	}
	return canonicalValue(v)
}

// canonicalValue encodes a value built from json.Decoder (UseNumber) output.
func canonicalValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeJCS(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeJCS(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		return writeJCSString(buf, t)
	case json.Number:
		s := t.String()
		if !jcsInt.MatchString(s) || s == "-0" {
			return fmt.Errorf("canonicalize: number %q is not a plain integer", s)
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n > maxJCSInt || n < -maxJCSInt {
			return fmt.Errorf("canonicalize: number %q is outside the safe integer range", s)
		}
		buf.WriteString(s)
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJCS(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			if !utf8.ValidString(k) {
				return errors.New("canonicalize: object key is not valid UTF-8")
			}
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return lessUTF16(keys[i], keys[j]) })
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJCSString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := writeJCS(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("canonicalize: unsupported value type %T", v)
	}
	return nil
}

// lessUTF16 orders strings by UTF-16 code units, as RFC 8785 section 3.2.3 requires.
func lessUTF16(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

// writeJCSString writes the JCS string form: only the two-character escapes for backspace, tab, line feed,
// form feed, carriage return, quote and backslash, \u00xx (lower-case hex) for other control characters,
// and every other character as raw UTF-8. (encoding/json would also escape <, > and & and U+2028/9.)
func writeJCSString(buf *bytes.Buffer, s string) error {
	if !utf8.ValidString(s) {
		return errors.New("canonicalize: string is not valid UTF-8")
	}
	const hex = "0123456789abcdef"
	buf.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			buf.WriteString(`\"`)
		case r == '\\':
			buf.WriteString(`\\`)
		case r == '\b':
			buf.WriteString(`\b`)
		case r == '\t':
			buf.WriteString(`\t`)
		case r == '\n':
			buf.WriteString(`\n`)
		case r == '\f':
			buf.WriteString(`\f`)
		case r == '\r':
			buf.WriteString(`\r`)
		case r < 0x20:
			buf.WriteString(`\u00`)
			buf.WriteByte(hex[r>>4])
			buf.WriteByte(hex[r&0xf])
		default:
			buf.WriteRune(r)
		}
	}
	buf.WriteByte('"')
	return nil
}

// checkSurrogates refuses a \u escape that is half of a surrogate pair (RFC 8785 requires well-formed
// strings; Go's decoder would silently turn it into U+FFFD).
func checkSurrogates(raw []byte) error {
	inStr := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !inStr {
			inStr = c == '"'
			continue
		}
		switch c {
		case '"':
			inStr = false
		case '\\':
			if i+1 >= len(raw) {
				return nil // the decoder reports the truncation
			}
			if raw[i+1] != 'u' {
				i++
				continue
			}
			u, ok := hex4(raw, i+2)
			if !ok {
				return nil // malformed escape: the decoder reports it
			}
			i += 5
			switch {
			case u >= 0xD800 && u < 0xDC00:
				lo, ok := uint16(0), false
				if i+6 < len(raw) && raw[i+1] == '\\' && raw[i+2] == 'u' {
					var v rune
					if v, ok = hex4(raw, i+3); ok && v >= 0xDC00 && v < 0xE000 {
						lo = uint16(v)
					} else {
						ok = false
					}
				}
				if !ok || lo == 0 {
					return errors.New("canonicalize: unpaired surrogate escape")
				}
				i += 6
			case u >= 0xDC00 && u < 0xE000:
				return errors.New("canonicalize: unpaired surrogate escape")
			}
		}
	}
	return nil
}

func hex4(raw []byte, at int) (rune, bool) {
	if at+4 > len(raw) {
		return 0, false
	}
	v, err := strconv.ParseUint(string(raw[at:at+4]), 16, 32)
	return rune(v), err == nil
}

// checkNoDuplicateKeys refuses an object that repeats a member name (RFC 8785 / I-JSON).
func checkNoDuplicateKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return nil // the real decode reports syntax errors
		}
		d, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch d {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil
				}
				k, _ := kt.(string)
				if seen[k] {
					return fmt.Errorf("canonicalize: duplicate object key %q", k)
				}
				seen[k] = true
				if err := walk(); err != nil {
					return err
				}
			}
			dec.Token()
		case '[':
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			dec.Token()
		}
		return nil
	}
	return walk()
}
