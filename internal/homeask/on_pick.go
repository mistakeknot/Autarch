package homeask

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// OnPick is immutable decoded card metadata, not an authorization to execute.
type OnPick map[string]any

// ValidateOnPickJSON checks original bytes before encoding/json can replace
// invalid scalars. Escaped member names have the same semantics as literal ones.
func ValidateOnPickJSON(raw []byte) error {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	} // The ordinary parser reports syntax errors.
	options, _ := m["options"].([]any)
	for _, v := range options {
		if option, ok := v.(map[string]any); ok {
			if _, present := option["on_pick"]; present {
				if !scalarJSON(raw) {
					return fmt.Errorf("on_pick: invalid Unicode scalar text")
				}
				if _, approval := option["approval"]; approval {
					return fmt.Errorf("on_pick: approval member excluded")
				}
			}
		}
	}
	return nil
}

type CardValidationContext struct {
	Source                  string
	RootRun, Move, Artifact bool
}

// Keep in parity with the plugin's ON_PICK_DENY. Screen a copy, never the payload.
var OnPickDeny = []string{"merge", "deploy", "release", "publish", "root", "admin", `force[\s_-]*push`}

func ValidateOnPick(raw json.RawMessage, option Option, context CardValidationContext) (OnPick, error) {
	fail := func(reason string) (OnPick, error) { return nil, fmt.Errorf("on_pick: %s", reason) }
	if context.Source != "card" {
		return fail("card source required")
	}
	var o map[string]any
	if !scalarJSON(raw) || json.Unmarshal(raw, &o) != nil || o == nil {
		return fail("expected scalar JSON object")
	}
	exact := func(m map[string]any, allowed, required []string) bool {
		if m == nil || acExactKeys(m, allowed, "on_pick") != nil {
			return false
		}
		for _, k := range required {
			if _, ok := m[k]; !ok {
				return false
			}
		}
		return true
	}
	fields := []string{"schema", "target_thread", "reversible", "subject", "ruling"}
	if !exact(o, append(append([]string{}, fields...), "notification"), fields) {
		return fail("missing or unknown member")
	}
	if o["schema"] != float64(1) {
		return fail("unsupported schema")
	}
	if _, ok := o["reversible"].(bool); !ok {
		return fail("explicit reversibility required")
	}
	text := func(v any, max int) bool {
		s, ok := v.(string)
		return ok && utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, 0)
	}
	if !text(o["target_thread"], 128) || !regexp.MustCompile(`^thr_[A-Za-z0-9_.:-]+$`).MatchString(o["target_thread"].(string)) {
		return fail("concrete target thread required")
	}
	kind := option.Kind
	if kind == "" {
		if option.Instruction != "" {
			kind = "instruction"
		} else {
			kind = "ruling-only"
		}
	}
	if (kind != "instruction" && kind != "ruling-only") || option.Approval != nil || context.RootRun || context.Move {
		return fail("excluded option or card")
	}
	relay := option.Instruction
	notification := ""
	if kind == "instruction" {
		if _, ok := o["notification"]; ok {
			return fail("instruction forbids notification")
		}
	} else {
		if !text(o["notification"], 8000) {
			return fail("notification required")
		}
		notification = o["notification"].(string)
		relay = notification
	}
	if !text(relay, 8000) || len(relay) > 32768 || !norm.NFC.IsNormalString(relay) {
		return fail("relay text would change")
	}
	screened := norm.NFKC.String(option.Label + "\n" + option.Instruction + "\n" + notification)
	screened = strings.Map(func(r rune) rune {
		// ECMAScript WhiteSpace/LineTerminator set, matching JS regexp \\s.
		if r == 0x0b || r == 0xa0 || r == 0x1680 || (r >= 0x2000 && r <= 0x200a) || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000 || r == 0xfeff {
			return ' '
		}
		if (r >= 0x2010 && r <= 0x2015) || r == 0x2212 || r == 0xfe58 || r == 0xfe63 || r == 0xff0d {
			return '-'
		}
		return r
	}, screened)
	for _, p := range OnPickDeny {
		if regexp.MustCompile("(?i)" + p).MatchString(screened) {
			return fail("privileged text excluded")
		}
	}
	s, _ := o["subject"].(map[string]any)
	if s["kind"] == "none" {
		if !exact(s, []string{"kind"}, []string{"kind"}) || context.Artifact {
			return fail("none contradicts artifact evidence")
		}
	} else {
		keys := []string{"kind", "repo", "number", "head"}
		if !exact(s, keys, keys) || s["kind"] != "github-pr" {
			return fail("invalid PR subject")
		}
		repo, _ := s["repo"].(string)
		head, _ := s["head"].(string)
		number, _ := s["number"].(float64)
		if !acRepoRe.MatchString(repo) || !acHeadRe.MatchString(head) || number <= 0 || number > 9007199254740991 || number != float64(int64(number)) {
			return fail("invalid PR subject")
		}
	}
	r, _ := o["ruling"].(map[string]any)
	if !exact(r, []string{"bead", "under"}, []string{"bead", "under"}) || (r["bead"] != nil && !text(r["bead"], 128)) {
		return fail("invalid ruling")
	}
	under, ok := r["under"].([]any)
	if !ok || len(under) > 16 {
		return fail("invalid under references")
	}
	for _, v := range under {
		if !text(v, 128) {
			return fail("invalid under reference")
		}
	}
	return OnPick(o), nil
}

// encoding/json replaces lone escaped surrogates. Refuse them before decoding.
func scalarJSON(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		var r uint16
		if _, err := fmt.Sscanf(string(raw[i+1:i+5]), "%04x", &r); err != nil {
			return false
		}
		i += 4
		if r >= 0xdc00 && r <= 0xdfff {
			return false
		}
		if r >= 0xd800 && r <= 0xdbff {
			if i+6 >= len(raw) || string(raw[i+1:i+3]) != `\u` {
				return false
			}
			var low uint16
			if _, err := fmt.Sscanf(string(raw[i+3:i+7]), "%04x", &low); err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
