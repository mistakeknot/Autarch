package agenttargets

// Frozen verbatim sanitizer and literals from fc5de917 (mk-rkkao.2).
import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var frozenANSI = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)
var frozenToken = regexp.MustCompile(`(?i)\bsk-[a-z0-9_-]+|(?:ghp_|gho_|ghs_|ghu_|ghr_|github_pat_)[a-z0-9_]+|Bearer[ \t]+[^\s]+|(?:Authorization|x-api-key)[ \t]*:[^\r\n]*|"(?:access_token|api_key|token|refresh_token|id_token|client_secret|apiKey)"\s*:\s*"(?:[^"\\]|\\.)*(?:"|$)|access_token=[^&\s]*`)

func frozenStderr(data []byte) string {
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

	text := frozenANSI.ReplaceAllString(string(data), "")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	text = frozenToken.ReplaceAllString(text, "[REDACTED]")
	if len(text) > stderrTailBytes {
		start := len(text) - stderrTailBytes
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		text = text[start:]
	}
	return text
}

func TestLegacyLiteralFrozen(t *testing.T) {
	if stderrANSI.String() != frozenANSI.String() || stderrToken.String() != frozenToken.String() {
		t.Fatal("legacy literal changed")
	}
}
func TestMainRedactionSuperset(t *testing.T) {
	for _, c := range newShapeCases {
		checkMainSuperset(t, []byte(c.input))
	}
	for _, input := range legacyGuardInputs() {
		checkMainSuperset(t, []byte(input))
		var tail stderrTail
		tail.Write([]byte(input))
		checkMainSuperset(t, tail.data)
	}
	for b := 32; b < 127; b++ {
		checkMainSuperset(t, []byte(string(rune(b))+"access_token=1111"))
	}
}
