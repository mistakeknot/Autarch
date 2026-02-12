package signals

import "regexp"

// ANSI escape sequence patterns for terminal output cleanup.
// Order of application matters: cursor movement must be replaced with
// spaces/newlines before generic CSI sequences are stripped, otherwise
// word and line boundaries are lost.
var (
	// cursorForwardRe matches cursor-forward sequences (\x1b[nC).
	// Replaced with spaces to preserve word boundaries.
	cursorForwardRe = regexp.MustCompile(`\x1b\[\d*C`)

	// cursorDownRe matches cursor-down sequences (\x1b[nB).
	// Replaced with newlines to preserve line boundaries.
	cursorDownRe = regexp.MustCompile(`\x1b\[\d*B`)

	// oscSeqRe matches Operating System Command sequences (window titles, etc).
	// Format: ESC ] <params> BEL  or  ESC ] <params> ST
	oscSeqRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

	// ansiCSIRe matches all remaining CSI sequences (colors, cursor movement, DEC private modes).
	ansiCSIRe = regexp.MustCompile(`\x1b\[\??[0-9;]*[A-Za-z]`)
)

// StripANSI removes ANSI escape sequences from s, replacing cursor
// movement with spaces/newlines to preserve word and line boundaries.
func StripANSI(s string) string {
	s = cursorForwardRe.ReplaceAllString(s, " ")
	s = cursorDownRe.ReplaceAllString(s, "\n")
	s = oscSeqRe.ReplaceAllString(s, "")
	return ansiCSIRe.ReplaceAllString(s, "")
}

// stripANSIBytes is the []byte variant used by the parser.
func stripANSIBytes(data []byte) []byte {
	data = cursorForwardRe.ReplaceAll(data, []byte(" "))
	data = cursorDownRe.ReplaceAll(data, []byte("\n"))
	data = oscSeqRe.ReplaceAll(data, nil)
	return ansiCSIRe.ReplaceAll(data, nil)
}
