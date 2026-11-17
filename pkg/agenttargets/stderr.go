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

// stderrTail retains only the last bytes. The process reader owns it until Wait
// completes, so callers must only inspect it after that reader has finished.
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
	return fmt.Errorf("%w (timed out: %t): stderr: %s", err, timedOut, sanitizedStderr(tail.data))
}
