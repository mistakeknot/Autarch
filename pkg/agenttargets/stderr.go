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
var stderrToken = regexp.MustCompile(`(?i)\bsk-[a-z0-9_-]+|(?:ghp_|gho_|ghs_|ghu_|ghr_|github_pat_)[a-z0-9_]+|Bearer[ \t]+[^\s]+|(?:Authorization|x-api-key)[ \t]*:[^\r\n]*|"(?:access_token|api_key|token|refresh_token|id_token|client_secret|apiKey)"\s*:\s*"(?:[^"\\]|\\.)*(?:"|$)|access_token=[^&\s]*`)

func sanitizedStderr(data []byte) string {
	// A full tail may start inside a credential keyword or its value. Drop
	// the entire first line: retaining only its suffix loses redaction context.
	if len(data) >= stderrTailBytes {
		retainedBytes := len(data)
		if end := strings.IndexByte(string(data), '\n'); end >= 0 {
			data = data[end+1:]
		} else {
			data = nil
		}
		if strings.TrimSpace(string(data)) == "" {
			return fmt.Sprintf("[stderr withheld: %d bytes with no complete line]", retainedBytes)
		}
	}

	text := stderrANSI.ReplaceAllString(string(data), "")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	text = stderrToken.ReplaceAllString(text, "[REDACTED]")
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
