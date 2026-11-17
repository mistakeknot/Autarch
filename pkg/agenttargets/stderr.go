package agenttargets

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const stderrTailBytes = 4096
const stderrHistoryBytes = 8192

// stderrTail retains a trailing window and, lazily, bounded complete history.
// The process reader owns it until Wait completes; inspect only after it joins.
type stderrTail struct {
	data        []byte
	started     bool
	seen        int
	historyLost bool
	history     []byte
}

func (t *stderrTail) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	if !t.started {
		t.started = true
		if len(t.data) != 0 {
			t.historyLost = true
		}
	}
	if !t.historyLost {
		if n > stderrHistoryBytes-t.seen {
			t.historyLost = true
			t.history = nil
			t.seen = stderrHistoryBytes + 1
		} else {
			total := t.seen + n
			if total > stderrTailBytes {
				if t.seen <= stderrTailBytes {
					t.history = make([]byte, 0, stderrHistoryBytes)
					t.history = append(t.history, t.data...)
				}
				t.history = append(t.history, p...)
			}
			t.seen = total
		}
	}
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

// completeHistory certifies process-start history independently of its content.
func (t *stderrTail) completeHistory() ([]byte, bool) {
	if !t.started || t.historyLost || t.seen < stderrTailBytes || t.seen > stderrHistoryBytes || len(t.data) != stderrTailBytes {
		return nil, false
	}
	if t.seen == stderrTailBytes {
		if len(t.history) != 0 {
			return nil, false
		}
		return t.data, true
	}
	if len(t.history) != t.seen || !bytes.Equal(t.data, t.history[len(t.history)-stderrTailBytes:]) {
		return nil, false
	}
	return t.history, true
}

var stderrANSI = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)
var stderrToken = regexp.MustCompile(`(?i)\bsk-[a-z0-9_-]+|(?:ghp_|gho_|ghs_|ghu_|ghr_|github_pat_)[a-z0-9_]+|Bearer[ \t]+[^\s]+|(?:Authorization|x-api-key)[ \t]*:[^\r\n]*|"(?:access_token|api_key|token|refresh_token|id_token|client_secret|apiKey)"\s*:\s*"(?:[^"\\]|\\.)*(?:"|$)|access_token=[^&\s]*`)

func sanitizedStderr(data []byte) string {
	raw := data
	// A full tail may start inside a credential keyword or its value. Drop
	// the entire first line: retaining only its suffix loses redaction context.
	if len(data) >= stderrTailBytes {
		retainedBytes := len(data)
		if end := strings.IndexByte(string(data), '\n'); end >= 0 {
			data = data[end+1:]
		} else {
			data = nil
		}
		// Decide content availability after ANSI and control removal, before
		// redaction or frame selection. No credential rule changes here.
		cleaned := stderrANSI.ReplaceAllString(string(data), "")
		cleaned = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, cleaned)
		if strings.TrimSpace(cleaned) == "" {
			return fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", retainedBytes)
		}
	}

	rendered := renderStderr(raw, len(raw)-len(data))
	// A nonblank cleaned remainder can still select an empty final CR frame.
	// The notice contains no stderr content; redaction and rendering stay intact.
	if len(raw) >= stderrTailBytes && rendered == "" {
		return fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", len(raw))
	}
	return rendered
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
	return fmt.Errorf("%w (timed out: %t): stderr: %s", err, timedOut, sanitizedStderrTail(tail))
}

func sanitizedStderrTail(t *stderrTail) string {
	var text string
	var recovered bool
	raw := t.data
	if history, ok := t.completeHistory(); ok {
		text, recovered = recoverStderrComplete(history)
		if recovered {
			// Inspect the same complete source recovery rendered; the trailing
			// window alone may begin inside a UTF-8 rune or ANSI sequence.
			raw = history
		}
	}
	if !recovered {
		text = sanitizedStderr(t.data)
	}
	if !stderrHasVisibleText(text) {
		// Only replace blank output when frame selection hid visible content.
		// Preserve streams containing only whitespace, format characters or
		// stripped controls. Never disclose their length in a wrapper notice.
		cleaned := stderrANSI.ReplaceAllString(string(raw), "")
		cleaned = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, cleaned)
		if stderrHasVisibleText(cleaned) {
			return "[stderr withheld: no visible final frame]"
		}
	}
	return text
}

func stderrHasVisibleText(text string) bool {
	for _, r := range text {
		// Cf includes marks some fonts or terminals draw: U+0600–U+0605,
		// U+06DD, U+070F, U+110BD, U+FFF9–U+FFFB, and soft hyphen U+00AD.
		// A final frame of only these marks gets the constant notice when
		// frame selection hides visible content. This is deliberately fail-safe:
		// the fixed text discloses nothing, and such a frame has no diagnostic
		// value. Streams made only of format characters keep their old output.
		if !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}
