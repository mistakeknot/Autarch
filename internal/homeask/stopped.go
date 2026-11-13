package homeask

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// CheckStopped validates delivery metadata without interpreting the question.
// The trim set matches JavaScript String.trim, including its treatment of FEFF.
func CheckStopped(m map[string]any) error {
	v, present := m["stopped"]
	if m["urgency"] != "blocker" {
		if present {
			return errors.New("stopped is allowed only with blocker urgency")
		}
		return nil
	}
	s, ok := v.(string)
	s = strings.Trim(s, "\u0009\u000a\u000b\u000c\u000d \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff")
	if !ok || s == "" || utf8.RuneCountInString(s) > 200 || strings.ContainsAny(s, "\r\n\u0085\u2028\u2029") {
		return errors.New("blocker urgency requires stopped: add stopped (or --stopped), a non-blank single line of at most 200 characters describing what is stopped until the answer, or file without blocker urgency (no phone push)")
	}
	m["stopped"] = s
	return nil
}
