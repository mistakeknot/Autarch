// Package homeask is the Go side of the Home ask protocol: the shared filing
// request schema (schema/home-ask/v1), its validation and defaults, and the
// three hashes derived from it (Identity, Revision, SemanticKey).
//
// The TypeScript twin lives in integrations/bb-plugin-autarch/model.ts. Both
// sides must produce the byte-for-byte outputs in schema/home-ask/v1/vectors.json.
package homeask

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Option is one answer mk can pick.
type Option struct {
	ID          string    `json:"id,omitempty"`
	Label       string    `json:"label"`
	Kind        string    `json:"kind,omitempty"`
	Reversible  bool      `json:"reversible,omitempty"`
	Instruction string    `json:"instruction,omitempty"`
	Approval    *Approval `json:"approval,omitempty"`
}

// Approval is the spec an option may carry (P-12). It makes the option
// non-reversible and non-delegable.
type Approval struct {
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Identity string `json:"identity"`
	TTL      int    `json:"ttl,omitempty"`
}

// Machine describes a blocker no agent in the thread can clear.
type Machine struct {
	Class       string `json:"class"`
	Detail      string `json:"detail"`
	OwnerThread string `json:"owner_thread,omitempty"`
}

// Ask is the filing request for every caller.
type Ask struct {
	V              int      `json:"v"`
	Kind           string   `json:"kind"`
	RequestID      string   `json:"request_id,omitempty"`
	AskKey         string   `json:"ask_key,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Project        string   `json:"project"`
	ProjectRoot    string   `json:"project_root"`
	Asker          string   `json:"asker"`
	Thread         string   `json:"thread,omitempty"`
	Question       string   `json:"question"`
	Recommendation string   `json:"recommendation,omitempty"`
	Supersedes     string   `json:"supersedes,omitempty"`
	MentionOf      string   `json:"mention_of,omitempty"`
	Options        []Option `json:"options,omitempty"`
	Steps          []string `json:"steps,omitempty"`
	Machine        *Machine `json:"machine,omitempty"`
}

const (
	maxInstruction = 2000
	maxLabel       = 80
	maxKey         = 120
	maxQuestion    = 2000
	maxSteps       = 20
	maxTTL         = 7 * 24 * 3600
)

var (
	optionIDRe   = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	requestIDRe  = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,128}$`)
	refIDRe      = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,128}$`)
	machineClass = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	approvalTok  = regexp.MustCompile(`APPROVED-[A-Z]+`)
)

// Parse decodes a request strictly (unknown fields are refused) and returns it
// normalized and validated.
func Parse(data []byte) (Ask, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var a Ask
	if err := dec.Decode(&a); err != nil {
		return Ask{}, fmt.Errorf("invalid ask: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Ask{}, errors.New("invalid ask: trailing data after the JSON value")
	}
	return Normalize(a)
}

// normText is the normalization ask_key, subject and the semantic key's
// question share: NFC, ASCII whitespace collapsed, ASCII lowercased. Only ASCII
// is case-folded, so Go and TypeScript agree byte for byte.
func normText(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	space := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

func runes(s string) int { return utf8.RuneCountInString(s) }

func firstRunes(s string, n int) string {
	if runes(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func slug(label string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(norm.NFC.String(label)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	s := firstRunes(b.String(), 32)
	return strings.TrimRight(s, "-")
}

func clean(field, s string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if strings.ContainsRune(s, 0) {
		return fmt.Errorf("%s must not contain NUL", field)
	}
	if approvalTok.MatchString(s) {
		return errors.New("approval tokens are not accepted in Home text")
	}
	return nil
}

// Normalize validates a and returns it with defaults applied and every string
// NFC-normalized. It is idempotent.
func Normalize(in Ask) (Ask, error) {
	a := in
	a.Options = append([]Option(nil), in.Options...)
	a.Steps = append([]string(nil), in.Steps...)
	if in.Machine != nil {
		m := *in.Machine
		a.Machine = &m
	}
	for i := range a.Options {
		if a.Options[i].Approval != nil {
			p := *a.Options[i].Approval
			a.Options[i].Approval = &p
		}
	}

	// Text hygiene first, so later length checks count real characters.
	strs := []struct {
		field string
		p     *string
	}{
		{"request_id", &a.RequestID}, {"ask_key", &a.AskKey}, {"subject", &a.Subject}, {"project", &a.Project},
		{"project_root", &a.ProjectRoot}, {"thread", &a.Thread}, {"question", &a.Question},
		{"recommendation", &a.Recommendation}, {"supersedes", &a.Supersedes}, {"mention_of", &a.MentionOf},
	}
	for _, s := range strs {
		if err := clean(s.field, *s.p); err != nil {
			return Ask{}, err
		}
		*s.p = norm.NFC.String(*s.p)
	}
	for i := range a.Options {
		o := &a.Options[i]
		for _, s := range []struct {
			field string
			p     *string
		}{{"option id", &o.ID}, {"label", &o.Label}, {"instruction", &o.Instruction}} {
			if err := clean(s.field, *s.p); err != nil {
				return Ask{}, err
			}
			*s.p = norm.NFC.String(*s.p)
		}
		if o.Approval != nil {
			for _, s := range []struct {
				field string
				p     *string
			}{{"approval kind", &o.Approval.Kind}, {"approval target", &o.Approval.Target}, {"approval identity", &o.Approval.Identity}} {
				if err := clean(s.field, *s.p); err != nil {
					return Ask{}, err
				}
				*s.p = norm.NFC.String(*s.p)
			}
		}
	}
	for i := range a.Steps {
		if err := clean("step", a.Steps[i]); err != nil {
			return Ask{}, err
		}
		a.Steps[i] = norm.NFC.String(a.Steps[i])
	}
	if a.Machine != nil {
		for _, s := range []struct {
			field string
			p     *string
		}{{"machine class", &a.Machine.Class}, {"machine detail", &a.Machine.Detail}, {"owner_thread", &a.Machine.OwnerThread}} {
			if err := clean(s.field, *s.p); err != nil {
				return Ask{}, err
			}
			*s.p = norm.NFC.String(*s.p)
		}
	}

	if a.V != 1 {
		return Ask{}, errors.New("v must be 1")
	}
	switch a.Kind {
	case "decide", "steps", "machine":
	default:
		return Ask{}, errors.New("kind must be decide, steps or machine")
	}
	switch a.Asker {
	case "thread":
		if a.Thread == "" {
			return Ask{}, errors.New("thread asker needs a thread")
		}
	case "mycroft":
		if a.Thread != "" {
			return Ask{}, errors.New("mycroft asker takes no thread")
		}
	default:
		return Ask{}, errors.New("asker must be thread or mycroft")
	}
	if a.Thread != "" && !refIDRe.MatchString(a.Thread) {
		return Ask{}, errors.New("thread must match [A-Za-z0-9:_.-]{1,128}")
	}
	if strings.TrimSpace(a.Project) == "" || runes(a.Project) > 200 {
		return Ask{}, errors.New("project is required (1-200 characters)")
	}
	if !strings.HasPrefix(a.ProjectRoot, "/") || strings.TrimSpace(a.ProjectRoot) == "" {
		return Ask{}, errors.New("project_root must be an absolute path")
	}
	if strings.TrimSpace(a.Question) == "" || runes(a.Question) > maxQuestion {
		return Ask{}, fmt.Errorf("question must be 1-2000 characters (got %d)", runes(a.Question))
	}
	if a.RequestID != "" && !requestIDRe.MatchString(a.RequestID) {
		return Ask{}, errors.New("request_id must be 1-128 of [A-Za-z0-9:_.-]")
	}
	for _, r := range []struct{ field, v string }{{"supersedes", a.Supersedes}, {"mention_of", a.MentionOf}} {
		if r.v != "" && !refIDRe.MatchString(r.v) {
			return Ask{}, fmt.Errorf("%s must match [A-Za-z0-9:_.-]{1,128}", r.field)
		}
	}
	if a.Supersedes != "" && a.MentionOf != "" {
		return Ask{}, errors.New("supersedes and mention_of cannot be used together")
	}

	if a.AskKey == "" {
		a.AskKey = strings.TrimRight(firstRunes(normText(a.Question), maxKey), " ")
	} else {
		a.AskKey = normText(a.AskKey)
	}
	if n := runes(a.AskKey); n < 1 || n > maxKey {
		return Ask{}, errors.New("ask_key must be 1-120 characters")
	}
	if a.Subject != "" {
		a.Subject = normText(a.Subject)
		if n := runes(a.Subject); n < 1 || n > maxKey {
			return Ask{}, errors.New("subject must be 1-120 characters")
		}
	}

	switch a.Kind {
	case "decide":
		if len(a.Steps) > 0 || a.Machine != nil {
			return Ask{}, errors.New("decide takes no steps or machine block")
		}
		if err := normalizeOptions(&a); err != nil {
			return Ask{}, err
		}
	case "steps":
		if len(a.Options) > 0 {
			return Ask{}, errors.New("steps takes no options")
		}
		if a.Machine != nil {
			return Ask{}, errors.New("steps takes no machine block")
		}
		if a.Recommendation != "" {
			return Ask{}, errors.New("recommendation must name an option")
		}
		if len(a.Steps) < 1 || len(a.Steps) > maxSteps {
			return Ask{}, errors.New("steps needs 1-20 steps")
		}
		for _, s := range a.Steps {
			if strings.TrimSpace(s) == "" || runes(s) > maxInstruction {
				return Ask{}, errors.New("each step must be 1-2000 characters")
			}
		}
	case "machine":
		if len(a.Options) > 0 || len(a.Steps) > 0 {
			return Ask{}, errors.New("machine takes no steps or options")
		}
		if a.Recommendation != "" {
			return Ask{}, errors.New("recommendation must name an option")
		}
		if a.Machine == nil {
			return Ask{}, errors.New("machine needs a machine block")
		}
		if !machineClass.MatchString(a.Machine.Class) {
			return Ask{}, errors.New("machine class must match [a-z0-9-]{1,32}")
		}
		if strings.TrimSpace(a.Machine.Detail) == "" || runes(a.Machine.Detail) > maxInstruction {
			return Ask{}, errors.New("machine detail must be 1-2000 characters")
		}
		if a.Machine.OwnerThread != "" && !refIDRe.MatchString(a.Machine.OwnerThread) {
			return Ask{}, errors.New("owner_thread must match [A-Za-z0-9:_.-]{1,128}")
		}
	}

	if a.RequestID == "" {
		a.RequestID = identityOf(a)
	}
	return a, nil
}

func normalizeOptions(a *Ask) error {
	if n := len(a.Options); n < 2 || n > 6 {
		return errors.New("decide needs 2-6 options")
	}
	seen := map[string]bool{}
	for i := range a.Options {
		o := &a.Options[i]
		if n := runes(o.Label); strings.TrimSpace(o.Label) == "" || n > maxLabel {
			return errors.New("label must be 1-80 characters")
		}
		if o.ID == "" {
			o.ID = slug(o.Label)
			if o.ID == "" {
				return errors.New("option id is required when the label has no ASCII letters or digits")
			}
		}
		if !optionIDRe.MatchString(o.ID) {
			return errors.New("option id must match [a-z0-9-]{1,32}")
		}
		if seen[o.ID] {
			return fmt.Errorf("duplicate option id %q", o.ID)
		}
		seen[o.ID] = true

		if o.Kind == "" {
			switch {
			case o.Instruction != "":
				o.Kind = "instruction"
			case a.Asker == "mycroft":
				o.Kind = "ruling-only"
			default:
				o.Kind = "needs-context"
			}
		}
		switch o.Kind {
		case "instruction", "needs-context", "ruling-only":
		default:
			return errors.New("option kind must be instruction, needs-context or ruling-only")
		}
		if a.Asker == "mycroft" {
			if o.Kind != "ruling-only" {
				return errors.New("mycroft may offer only ruling-only options")
			}
			if o.Reversible {
				return errors.New("reversible is not allowed when asker is mycroft")
			}
		}
		if o.Kind == "instruction" && o.Instruction == "" {
			return errors.New("an instruction option needs an instruction")
		}
		if o.Kind != "instruction" && o.Instruction != "" {
			return errors.New("instruction is only allowed on an instruction option")
		}
		if o.Instruction != "" {
			if n := runes(o.Instruction); n < 1 || n > maxInstruction {
				return errors.New("instruction must be 1-2000 characters")
			}
		}
		if o.Approval != nil {
			if o.Reversible {
				return errors.New("reversible is not allowed on an option with an approval spec")
			}
			switch o.Approval.Kind {
			case "merge", "deploy", "release":
			default:
				return errors.New("approval kind must be merge, deploy or release")
			}
			if strings.TrimSpace(o.Approval.Target) == "" || strings.TrimSpace(o.Approval.Identity) == "" {
				return errors.New("approval needs a target and an identity")
			}
			if o.Approval.TTL < 0 || o.Approval.TTL > maxTTL {
				return errors.New("approval ttl must be 1-604800 seconds")
			}
		}
	}
	if a.Recommendation != "" && !seen[a.Recommendation] {
		return errors.New("recommendation must name an option: it is the id of one of the options, exactly, not a sentence")
	}
	return nil
}

// ---- canonical JSON ----

// canonValue is one of: string, int, bool, []any, map[string]any.
type canonValue = any

func optionNode(o Option) map[string]any {
	m := map[string]any{"id": o.ID, "label": o.Label, "kind": o.Kind, "instruction": o.Instruction, "reversible": o.Reversible}
	if o.Approval != nil {
		m["approval"] = map[string]any{"kind": o.Approval.Kind, "target": o.Approval.Target,
			"identity": o.Approval.Identity, "ttl": o.Approval.TTL}
	}
	return m
}

func optionsNode(os []Option) []any {
	out := make([]any, 0, len(os))
	for _, o := range os {
		out = append(out, optionNode(o))
	}
	return out
}

func stepsNode(ss []string) []any {
	out := make([]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}

func machineNode(m *Machine, withOwner bool) any {
	if m == nil {
		return nil
	}
	n := map[string]any{"class": m.Class, "detail": m.Detail}
	if withOwner {
		n["owner_thread"] = m.OwnerThread
	}
	return n
}

func scopeNode(a Ask) map[string]any {
	return map[string]any{"project": a.Project, "project_root": a.ProjectRoot, "asker": a.Asker, "thread": a.Thread}
}

func bodyNode(a Ask) map[string]any {
	return map[string]any{
		"v": a.V, "kind": a.Kind, "ask_key": a.AskKey, "subject": a.Subject, "question": a.Question,
		"recommendation": a.Recommendation, "supersedes": a.Supersedes, "mention_of": a.MentionOf,
		"options": optionsNode(a.Options), "steps": stepsNode(a.Steps), "machine": machineNode(a.Machine, true),
	}
}

// NormalizedJSON is the canonical JSON of the whole normalized ask, request_id included.
func NormalizedJSON(a Ask) string {
	n := bodyNode(a)
	n["request_id"] = a.RequestID
	n["project"], n["project_root"], n["asker"], n["thread"] = a.Project, a.ProjectRoot, a.Asker, a.Thread
	return canonString(n)
}

func hashOf(v any) string {
	sum := sha256.Sum256([]byte(canonString(v)))
	return hex.EncodeToString(sum[:])
}

func identityOf(a Ask) string {
	return hashOf(map[string]any{"scope": scopeNode(a), "body": bodyNode(a)})
}

func normalized(a Ask) (Ask, bool) {
	n, err := Normalize(a)
	return n, err == nil
}

// Identity is sha256 over the canonical {scope, body}: every field except
// request_id. A missing request_id defaults to it, so a blind retry is idempotent.
// It is empty for an invalid ask.
func Identity(a Ask) string {
	n, ok := normalized(a)
	if !ok {
		return ""
	}
	return identityOf(n)
}

// Revision is sha256 over the canonical decision including its scope. A pick
// carries the revision it saw, so any change to what would be picked makes it stale.
func Revision(a Ask) string {
	n, ok := normalized(a)
	if !ok {
		return ""
	}
	opts := make([]any, 0, len(n.Options))
	for _, o := range n.Options {
		opts = append(opts, optionNode(o))
	}
	return hashOf(map[string]any{
		"scope": scopeNode(n), "kind": n.Kind, "question": n.Question, "options": opts,
		"recommendation": n.Recommendation, "supersedes": n.Supersedes,
		"steps": stepsNode(n.Steps), "machine": machineNode(n.Machine, true),
	})
}

// SemanticKey (P-11) says two filings ask the same thing. It is empty when the
// ask has no subject, because only a structured subject allows an automatic mention.
// The scope is deliberately absent: project is compared next to it, and the asking
// thread is what a mention adds.
func SemanticKey(a Ask) string {
	n, ok := normalized(a)
	if !ok || n.Subject == "" {
		return ""
	}
	return hashOf(map[string]any{
		"kind": n.Kind, "subject": n.Subject, "question": normText(n.Question),
		"options": optionsNode(n.Options), "recommendation": n.Recommendation,
		"steps": stepsNode(n.Steps), "machine": machineNode(n.Machine, false),
	})
}

// CanonicalJSON canonicalizes arbitrary JSON: keys sorted by UTF-16 code unit,
// no whitespace, strings NFC, and empty members (null, "", false, [], {}) dropped.
// Only integers are accepted as numbers.
func CanonicalJSON(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	conv, err := fromDecoded(v)
	if err != nil {
		return "", err
	}
	return canonString(conv), nil
}

func fromDecoded(v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, string:
		return x, nil
	case json.Number:
		n, err := strconv.Atoi(x.String())
		if err != nil {
			return nil, fmt.Errorf("only integers are canonical: %s", x)
		}
		return n, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			c, err := fromDecoded(e)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			c, err := fromDecoded(e)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported JSON value %T", v)
}

func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case int:
		return x == 0
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// prune drops empty members of objects, recursively. Array elements are kept
// (their own objects are pruned), so positions never shift.
func prune(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			p := prune(e)
			if !empty(p) {
				out[k] = p
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = prune(e)
		}
		return out
	}
	return v
}

func canonString(v any) string {
	var b strings.Builder
	writeCanon(&b, prune(v))
	return b.String()
}

func utf16Less(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

func writeCanon(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case int:
		b.WriteString(strconv.Itoa(x))
	case string:
		writeString(b, norm.NFC.String(x))
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			writeCanon(b, e)
		}
		b.WriteByte(']')
	case map[string]any:
		nx := make(map[string]any, len(x))
		keys := make([]string, 0, len(x))
		for k, e := range x {
			nk := norm.NFC.String(k)
			nx[nk] = e
			keys = append(keys, nk)
		}
		sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k)
			b.WriteByte(':')
			writeCanon(b, nx[k])
		}
		b.WriteByte('}')
	}
}

// writeString matches JavaScript's JSON.stringify for well-formed strings:
// short escapes for \b \f \n \r \t, lowercase \u00xx for other controls, and
// everything else (including <, >, &, U+2028, U+2029, DEL) raw.
func writeString(b *strings.Builder, s string) {
	const hexdigits = "0123456789abcdef"
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte(hexdigits[r>>4])
			b.WriteByte(hexdigits[r&0xf])
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
