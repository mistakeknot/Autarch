package homeask

// The home-move/v1 block (what mk owes on a card). This is the Go twin of
// integrations/bb-plugin-autarch/moves.ts parseMove: the filer refuses what the plugin would refuse, so a
// card is never filed that Home would show as display-only. Nothing here executes: a move is untrusted
// card text, and the plugin derives every command from the validated fields.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// MoveLabel is the label a card carries when Home's filer made it for a move; only such cards are closed
// when the move closes (the plugin's HOME_MOVE_LABEL).
const MoveLabel = "mk-move"

const (
	moveSchema  = "home-move/v1"
	maxMovePath = 1024
	maxMoveArgs = 8
	maxMoveNeed = 2000
)

var (
	movePathRe = regexp.MustCompile(`^/[A-Za-z0-9._/+-]+$`)
	moveSHARe  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	moveArgRe  = regexp.MustCompile(`^[A-Za-z0-9._/:=+@-]{1,200}$`)
	movePRRe   = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[1-9][0-9]{0,8}$`)
	moveHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
	moveASCII  = regexp.MustCompile(`^[\x21-\x7e]+$`)
)

// Move is a validated home-move/v1 object.
type Move struct {
	Kind string
	// Raw is the object as written; the filer writes it back out unchanged.
	Raw map[string]any
}

func isObj(v any) (map[string]any, bool) { m, ok := v.(map[string]any); return m, ok }

func onlyKeys(o map[string]any, allowed []string, what string) error {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ok := false
		for _, a := range allowed {
			ok = ok || a == k
		}
		if !ok {
			return fmt.Errorf("unknown field %q in %s", k, what)
		}
	}
	return nil
}

func validMovePath(p any) bool {
	s, ok := p.(string)
	if !ok || len(s) > maxMovePath || !movePathRe.MatchString(s) {
		return false
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func moveScriptRef(v any, what string) error {
	o, ok := isObj(v)
	if !ok {
		return fmt.Errorf("%s must be an object", what)
	}
	if err := onlyKeys(o, []string{"path", "sha256", "args", "recover"}, what); err != nil {
		return err
	}
	if !validMovePath(o["path"]) {
		return fmt.Errorf("%s path must be an absolute path of letters, digits and ._/+- with no .. segment", what)
	}
	if s, ok := o["sha256"].(string); !ok || !moveSHARe.MatchString(s) {
		return fmt.Errorf("%s sha256 must be 64 lowercase hex characters", what)
	}
	return nil
}

// EnvMoveReadHosts lists extra hosts a read move may point at (comma-separated bare host names).
// github.com is always allowed. The list is read at call time, like the plugin's.
const EnvMoveReadHosts = "HOME_MOVE_READ_HOSTS"

func moveReadHostAllowed(host string) bool {
	if host == "github.com" {
		return true
	}
	for _, h := range strings.Split(os.Getenv(EnvMoveReadHosts), ",") {
		h = strings.ToLower(strings.TrimSpace(h))
		if moveHostRe.MatchString(h) && h == host {
			return true
		}
	}
	return false
}

func moveReadURL(v any) error {
	s, ok := v.(string)
	if !ok || len(s) > 2048 {
		return errors.New("read url must be a string")
	}
	u, err := url.Parse(s)
	// Stricter than the plugin on purpose: printable ASCII only, so the Go and URL-class parses cannot differ.
	if err != nil || !moveASCII.MatchString(s) {
		return errors.New("read url is not a URL")
	}
	if u.Scheme != "https" || !moveReadHostAllowed(u.Hostname()) || u.User != nil || u.Port() != "" || u.Host != u.Hostname() {
		return errors.New("read url must be https on an allowed host")
	}
	if u.String() != s || u.Path == "" {
		return errors.New("read url must be in canonical form")
	}
	return nil
}

// ParseMove validates a home-move/v1 body. The error text is the display-only reason, the same as the plugin's.
func ParseMove(body string) (Move, error) {
	dec := json.NewDecoder(strings.NewReader(body))
	var v any
	if err := dec.Decode(&v); err != nil {
		return Move{}, fmt.Errorf("invalid home-move JSON: %v", err)
	}
	if dec.More() {
		return Move{}, errors.New("invalid home-move JSON: trailing data after the JSON value")
	}
	o, ok := isObj(v)
	if !ok {
		return Move{}, errors.New("home-move must be a JSON object")
	}
	if o["schema"] != moveSchema {
		return Move{}, errors.New("schema must be " + moveSchema)
	}
	kind, _ := o["kind"].(string)
	switch kind {
	case "script", "pr", "read", "context":
	default:
		return Move{}, errors.New("kind must be script, pr, read or context")
	}
	if err := onlyKeys(o, []string{"schema", "kind", kind}, "home-move/v1"); err != nil {
		return Move{}, err
	}
	m, present := o[kind]
	if !present {
		return Move{}, fmt.Errorf("%s move needs a %q member", kind, kind)
	}
	switch kind {
	case "script":
		if err := moveScriptRef(m, "script"); err != nil {
			return Move{}, err
		}
		so := m.(map[string]any)
		if a, has := so["args"]; has {
			list, ok := a.([]any)
			if !ok || len(list) > maxMoveArgs {
				return Move{}, fmt.Errorf("args must be an array of at most %d strings", maxMoveArgs)
			}
			for _, x := range list {
				if s, ok := x.(string); !ok || !moveArgRe.MatchString(s) {
					return Move{}, errors.New("each arg must match ^[A-Za-z0-9._/:=+@-]{1,200}$")
				}
			}
		}
		if r, has := so["recover"]; has {
			if ro, ok := isObj(r); ok {
				if _, a := ro["args"]; a {
					return Move{}, errors.New("recover takes only path and sha256")
				}
				if _, a := ro["recover"]; a {
					return Move{}, errors.New("recover takes only path and sha256")
				}
			}
			if err := moveScriptRef(r, "recover"); err != nil {
				return Move{}, err
			}
		}
	case "pr":
		po, ok := isObj(m)
		if !ok {
			return Move{}, errors.New("pr must be an object")
		}
		if err := onlyKeys(po, []string{"url", "summary", "verdict", "review_url", "why"}, "pr"); err != nil {
			return Move{}, err
		}
		if s, ok := po["url"].(string); !ok || !movePRRe.MatchString(s) {
			return Move{}, errors.New("pr url must be https://github.com/<owner>/<repo>/pull/<n>")
		}
		// Optional merge-card facts; the same rules as the Home plugin's parser (moves.ts).
		if v, has := po["summary"]; has {
			s, ok := v.(string)
			if !ok || strings.TrimSpace(s) == "" || len([]rune(s)) > maxMoveSummary || !utf8.ValidString(s) || strings.ContainsAny(s, "\r\n\u0085\u2028\u2029") {
				return Move{}, fmt.Errorf("pr summary must be one line of 1-%d characters", maxMoveSummary)
			}
		}
		if v, has := po["verdict"]; has {
			if s, ok := v.(string); !ok || !movePRVerdicts[s] {
				return Move{}, errors.New("pr verdict must be one of PASS, PASS-WITH-NOTES, HOLD, FAIL")
			}
		}
		if v, has := po["why"]; has {
			if s, ok := v.(string); !ok || (s != "design" && s != "taste" && s != "spend") {
				return Move{}, errors.New("pr why must be one of design, taste, spend")
			}
		}
		if v, has := po["review_url"]; has {
			if err := moveReadURL(v); err != nil {
				return Move{}, err
			}
		}
	case "read":
		ro, ok := isObj(m)
		if !ok {
			return Move{}, errors.New("read must be an object")
		}
		if err := onlyKeys(ro, []string{"url"}, "read"); err != nil {
			return Move{}, err
		}
		if err := moveReadURL(ro["url"]); err != nil {
			return Move{}, err
		}
	case "context":
		co, ok := isObj(m)
		if !ok {
			return Move{}, errors.New("context must be an object")
		}
		if err := onlyKeys(co, []string{"need"}, "context"); err != nil {
			return Move{}, err
		}
		s, ok := co["need"].(string)
		if !ok || strings.TrimSpace(s) == "" || len([]rune(s)) > maxMoveNeed || !utf8.ValidString(s) {
			return Move{}, fmt.Errorf("context need must be 1-%d characters of text", maxMoveNeed)
		}
	}
	return Move{Kind: kind, Raw: o}, nil
}

// moveBlock is the fenced block a card carries for a move: the validated object as one JSON line.
const maxMoveSummary = 200

var movePRVerdicts = map[string]bool{"PASS": true, "PASS-WITH-NOTES": true, "HOLD": true, "FAIL": true}

func moveBlock(raw map[string]any) (string, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(raw); err != nil {
		return "", err
	}
	return "```home-move\n" + strings.TrimRight(buf.String(), "\n") + "\n```\n", nil
}
