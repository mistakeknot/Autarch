package homeask

// The card convention (v1): how a tasks card's description text carries a Home ask.
// This is the Go twin of integrations/bb-plugin-autarch/cards.ts. Both read the
// shared vectors in integrations/bb-plugin-autarch/__tests__/fixtures/cards/*.json,
// and the error messages below are part of that contract.
//
// The parse is strict: any malformed block makes the whole card display-only, and the
// error is the reason shown. A card is untrusted and mutable input.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// BlockRef is one ref of a Blocks line.
type BlockRef struct {
	Ref     string `json:"ref"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Counted bool   `json:"counted"`
}

// RequestLine is the card's `Request: <key> sha256:<16 hex>` line.
type RequestLine struct {
	Key      string `json:"key"`
	Identity string `json:"identity"`
}

// RootRun is the optional fenced root-run block.
type RootRun struct {
	Script  string `json:"script"`
	SHA256  string `json:"sha256"`
	Timeout int    `json:"timeout"`
	Set     string `json:"set"`
}

// Card is a parsed card description.
type Card struct {
	Question string      `json:"question"`
	Blocks   []BlockRef  `json:"blocks"`
	Request  RequestLine `json:"request"`
	// Pull is "mycroft" for a threadless card, otherwise "".
	Pull    string         `json:"pull"`
	RootRun *RootRun       `json:"root_run"`
	Ask     map[string]any `json:"-"` // the home-ask/v2 object as written
}

var (
	cardKeyRe     = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,128}$`)
	cardIdentRe   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	cardRequestRe = regexp.MustCompile(`^Request: (\S+) sha256:(\S+)$`)
	blockKindRe   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	rootSetRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	rootSHARe     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	rootTimeoutRe = regexp.MustCompile(`^[0-9]{1,6}$`)
	rootLineRe    = regexp.MustCompile(`^([a-z0-9_]+):(.*)$`)
)

const (
	v2Schema        = "home-ask/v2"
	maxRootScript   = 4096
	maxRootTimeout  = 3600
	placeholderThrd = "thr_placeholder"
)

var (
	v2Allowed = map[string]bool{"schema": true, "project": true, "project_root": true, "question": true,
		"options": true, "ask_key": true, "subject": true, "recommendation": true, "pull": true}
	v1Only = map[string]bool{"v": true, "kind": true, "thread": true, "asker": true, "request_id": true,
		"supersedes": true, "mention_of": true, "steps": true, "machine": true}
	countedKinds = map[string]bool{"bead": true, "thread": true, "project": true}
)

// ParseCard parses a card description. It never returns a partial card: on error the
// card is display-only and err says why.
func ParseCard(description string) (Card, error) {
	lines := strings.Split(strings.ReplaceAll(description, "\r\n", "\n"), "\n")
	var (
		prose      []string
		blocksText []string
		requests   []string
		askBodies  []string
		rootBodies []string
	)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "```") {
			info := strings.TrimSpace(line[3:])
			special := info == "home-ask" || info == "root-run"
			var body []string
			closed := false
			for i++; i < len(lines); i++ {
				if strings.TrimRight(lines[i], " \t") == "```" {
					closed = true
					break
				}
				body = append(body, lines[i])
			}
			if special {
				if !closed {
					return Card{}, fmt.Errorf("unterminated %s block", info)
				}
				if info == "home-ask" {
					askBodies = append(askBodies, strings.Join(body, "\n"))
				} else {
					rootBodies = append(rootBodies, strings.Join(body, "\n"))
				}
				continue
			}
			// An ordinary code fence is prose, and nothing inside it is a field.
			prose = append(prose, line)
			prose = append(prose, body...)
			if closed {
				prose = append(prose, "```")
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "Blocks:"):
			blocksText = append(blocksText, line[len("Blocks:"):])
		case strings.HasPrefix(line, "Request:"):
			requests = append(requests, line)
		default:
			prose = append(prose, line)
		}
	}

	var c Card
	c.Question = strings.TrimSpace(strings.Join(prose, "\n"))

	blocks, err := parseBlocks(blocksText)
	if err != nil {
		return Card{}, err
	}
	c.Blocks = blocks

	switch len(requests) {
	case 0:
		return Card{}, errors.New("missing Request line")
	case 1:
		m := cardRequestRe.FindStringSubmatch(requests[0])
		if m == nil || !cardKeyRe.MatchString(m[1]) || !cardIdentRe.MatchString(m[2]) {
			return Card{}, errors.New("invalid Request line")
		}
		c.Request = RequestLine{Key: m[1], Identity: m[2]}
	default:
		return Card{}, errors.New("more than one Request line")
	}

	switch len(askBodies) {
	case 0:
		return Card{}, errors.New("missing home-ask block")
	case 1:
		if err := parseWireAsk(askBodies[0], &c); err != nil {
			return Card{}, err
		}
	default:
		return Card{}, errors.New("more than one home-ask block")
	}

	switch len(rootBodies) {
	case 0:
	case 1:
		rr, err := parseRootRun(rootBodies[0])
		if err != nil {
			return Card{}, err
		}
		c.RootRun = rr
	default:
		return Card{}, errors.New("more than one root-run block")
	}

	// The ask must survive the unchanged rev-4 parser, so every v1 rule (approval
	// tokens, option bounds, ...) applies to a card. The thread is a stand-in:
	// routing comes from the card's comments, never from its text.
	thread := placeholderThrd
	if c.Pull != "" {
		thread = ""
	}
	if _, err := c.ToV1(thread); err != nil {
		return Card{}, err
	}
	return c, nil
}

func parseBlocks(texts []string) ([]BlockRef, error) {
	refs := []BlockRef{}
	seen := map[string]bool{}
	for _, text := range texts {
		toks := strings.Fields(text)
		if len(toks) == 0 {
			return nil, errors.New("Blocks line has no refs")
		}
		for _, tok := range toks {
			kind, id, ok := strings.Cut(tok, ":")
			if !ok || !blockKindRe.MatchString(kind) || !cardKeyRe.MatchString(id) || kind == "thread" && !strings.HasPrefix(id, "thr_") {
				return nil, fmt.Errorf("invalid Blocks ref %q", tok)
			}
			if seen[tok] {
				continue
			}
			seen[tok] = true
			refs = append(refs, BlockRef{Ref: tok, Kind: kind, ID: id, Counted: countedKinds[kind]})
		}
	}
	return refs, nil
}

func parseWireAsk(body string, c *Card) error {
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("invalid home-ask JSON: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("invalid home-ask JSON: trailing data after the JSON value")
	}
	m, ok := v.(map[string]any)
	if !ok {
		return errors.New("home-ask must be a JSON object")
	}
	if s, _ := m["schema"].(string); s != v2Schema {
		return errors.New("schema must be " + v2Schema)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v1Only[k] {
			return fmt.Errorf("v1-only field %q in home-ask/v2", k)
		}
		if !v2Allowed[k] {
			return fmt.Errorf("unknown field %q in home-ask/v2", k)
		}
	}
	if p, present := m["pull"]; present {
		if s, _ := p.(string); s != "mycroft" {
			return errors.New("pull must be mycroft")
		}
		c.Pull = "mycroft"
	}
	c.Ask = m
	return nil
}

// ToV1 converts the wire ask into a rev-4 home-ask/v1 Ask through the unchanged Parse.
// A thread card needs the asking thread (from AskingThread); a pull:mycroft card
// ignores the thread argument and becomes a mycroft ask.
func (c Card) ToV1(thread string) (Ask, error) {
	v1 := make(map[string]any, len(c.Ask)+4)
	for k, v := range c.Ask {
		if k != "schema" && k != "pull" {
			v1[k] = v
		}
	}
	v1["v"] = 1
	v1["kind"] = "decide"
	if c.Pull == "mycroft" {
		v1["asker"] = "mycroft"
	} else {
		v1["asker"] = "thread"
		v1["thread"] = thread
	}
	raw, err := json.Marshal(v1)
	if err != nil {
		return Ask{}, err
	}
	a, err := Parse(raw)
	if err != nil {
		return Ask{}, fmt.Errorf("home-ask: %w", err)
	}
	return a, nil
}

func parseRootRun(body string) (*RootRun, error) {
	vals := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := rootLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil, errors.New("invalid root-run line")
		}
		switch m[1] {
		case "script", "sha256", "timeout", "set":
		default:
			return nil, fmt.Errorf("unknown root-run key %q", m[1])
		}
		if _, dup := vals[m[1]]; dup {
			return nil, fmt.Errorf("duplicate root-run key %q", m[1])
		}
		vals[m[1]] = strings.TrimSpace(m[2])
	}
	for _, k := range []string{"script", "sha256", "timeout", "set"} {
		if _, ok := vals[k]; !ok {
			return nil, errors.New("root-run needs script, sha256, timeout and set")
		}
	}
	rr := &RootRun{Script: vals["script"], SHA256: vals["sha256"], Set: vals["set"]}
	if !validRootScript(rr.Script) {
		return nil, errors.New("script must be an absolute path with no .. segment or control character, at most 4096 bytes")
	}
	if !rootSHARe.MatchString(rr.SHA256) {
		return nil, errors.New("sha256 must be 64 lowercase hex characters")
	}
	t := vals["timeout"]
	n, err := strconv.Atoi(t)
	if !rootTimeoutRe.MatchString(t) || err != nil || n < 1 || n > maxRootTimeout {
		return nil, errors.New("timeout must be 1-3600 seconds")
	}
	rr.Timeout = n
	if !rootSetRe.MatchString(rr.Set) {
		return nil, errors.New("set must match ^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$")
	}
	return rr, nil
}

func validRootScript(s string) bool {
	if len(s) == 0 || len(s) > maxRootScript || !utf8.ValidString(s) || s[0] != '/' {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// Comment is the slice of a tasks comment AskingThread reads.
type Comment struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	ThreadID  string `json:"threadId"`
	CreatedAt string `json:"createdAt"`
}

// AskingThread is the one selector for who asked: the threadId of the earliest
// kind:"agent" comment, ordering by (createdAt, id). It is "" when there is no agent
// comment, or when the earliest one carries no thread. authorName is never read.
// The input is not modified.
func AskingThread(comments []Comment) string {
	var first *Comment
	for i := range comments {
		c := &comments[i]
		if c.Kind != "agent" {
			continue
		}
		if first == nil || c.CreatedAt < first.CreatedAt || c.CreatedAt == first.CreatedAt && c.ID < first.ID {
			first = c
		}
	}
	if first == nil {
		return ""
	}
	return first.ThreadID
}
