package agenttargets

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const stderrTailBytes = 4096

// stderrTail retains only the last bytes. The process reader owns it until Wait
// completes, so callers must only inspect it after that reader has finished.
type stderrTail struct{ data []byte }

func (t *stderrTail) Write(p []byte) (int, error) {
	n := len(p)
	if n >= stderrTailBytes {
		t.data = append(t.data[:0], p[n-stderrTailBytes:]...)
	} else {
		if excess := len(t.data) + n - stderrTailBytes; excess > 0 {
			copy(t.data, t.data[excess:])
			t.data = t.data[:len(t.data)-excess]
		}
		t.data = append(t.data, p...)
	}
	return n, nil
}

var stderrANSI = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

var stderrToken = regexp.MustCompile(`(?i)\bsk-[a-z0-9_-]+|(?:ghp_|gho_|ghs_|ghu_|ghr_|github_pat_)[a-z0-9_]+|Bearer[ \t]+[^\s]+`)

// Match complete credential names at field boundaries, rather than arbitrary
// words ending in "key". Go's regexp engine guarantees linear-time matching.
var stderrAssignment = regexp.MustCompile(`(?i)(^|[ \t{(\[,?&"'\x5c])(?:token|key|secret|password|passwd|authorization|cookie|(?:access|refresh|id)[_-]?token|client[_-]?secret|secretKey|(?:[a-z0-9]+[_-])+(?:token|key|secret|password|passwd)|api[_-]?key|x-api-key)\\?["']?[ \t]*([=:])[ \t]*(\\".*?(?:\\"|$)|"(?:[^"\\]|\\.)*(?:"|$)|'(?:[^'\\]|\\.)*(?:'|$)|[^\s&,;}\]\)]+(?:[ \t][^\s&,;}\]\)]+)*)`)
var stderrFlags = regexp.MustCompile(`(?i)(^|[ \t])--(?:api-key|token|password|passwd|secret)[ \t]+[^\s]+`)

// Preserve the original header coverage: arbitrary schemes and embedded
// Authorization/x-api-key headers are sensitive through the end of the frame.
var stderrHeader = regexp.MustCompile(`(?i)(?:Authorization|x-api-key|Set-Cookie|Cookie)[ \t]*:[^\r\n]*`)
var stderrURLCredential = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s]*@`)

func redactStderrFrame(text string) string {
	text = stderrToken.ReplaceAllString(text, "[REDACTED]")
	text = stderrHeader.ReplaceAllString(text, "[REDACTED]")
	text = stderrFlags.ReplaceAllString(text, "[REDACTED]")
	text = stderrAssignment.ReplaceAllStringFunc(text, func(field string) string {
		// These diagnostic phrases describe a credential lookup failure rather
		// than a credential value. Quoted values are always redacted.
		parts := stderrAssignment.FindStringSubmatch(field)
		if parts[2] == ":" {
			value := strings.TrimSpace(parts[3])
			if value == "unknown field" || value == "not found in vault" || value == "expired" {
				return field
			}
		}
		prefix := parts[1]
		return prefix + "[REDACTED]"
	})
	return stderrURLCredential.ReplaceAllStringFunc(text, func(value string) string {
		end := strings.Index(value, "://") + 3
		userinfo := value[end : len(value)-1]
		if colon := strings.IndexByte(userinfo, ':'); colon >= 0 {
			return value[:end] + userinfo[:colon] + ":[REDACTED]@"
		}
		return value[:end] + "[REDACTED]@"
	})
}

func sanitizedStderr(data []byte) string {
	fallback := false
	// A full tail may start inside a credential keyword or its value. Drop
	// the entire first frame when its boundary is available.
	if len(data) >= stderrTailBytes {
		// A final line terminator does not establish a complete next frame.
		// Search only within the content so a single long line uses fallback.
		data = []byte(strings.TrimRight(string(data), "\r\n"))
		if end := strings.IndexAny(string(data), "\r\n"); end >= 0 {
			data = data[end+1:]
		} else {
			// Without a frame boundary, terminal escapes and controls can hide
			// most of a credential after the raw cut. Context cannot be recovered
			// safely; suppress this ambiguous frame rather than expose a suffix.
			for _, c := range data {
				if c < 32 && c != '\t' {
					return "[REDACTED]"
				}
			}
			fallback = true
		}
	}

	text := stderrANSI.ReplaceAllString(string(data), "")
	// A carriage return overwrites the current line. Select the final frame
	// on each line before redaction so a value cannot consume the next frame.
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if end := strings.LastIndexByte(line, '\r'); end >= 0 {
			line = line[end+1:]
		}
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\t' {
				return -1
			}
			return r
		}, line)
		lines[i] = redactStderrFrame(line)
	}
	text = strings.Join(lines, "\n")
	if fallback {
		// Redact with all available context BEFORE the display cut, which may
		// land between a credential's key and value.
		if len(text) > 512 {
			text = text[len(text)-512:]
		}
		if end := strings.IndexAny(text, " \t"); end >= 0 {
			text = "[REDACTED] " + text[end+1:]
		} else {
			text = "[REDACTED]"
		}
	}
	if len(text) > stderrTailBytes {
		start := len(text) - stderrTailBytes
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		text = text[start:]
	}
	return text
}

func processError(err error, timedOut bool, tail *stderrTail) error {
	if err == nil && !timedOut {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("process timed out")
	}
	if len(tail.data) == 0 {
		return err
	}
	return fmt.Errorf("%w (timed out: %t): stderr: %s", err, timedOut, sanitizedStderr(tail.data))
}
