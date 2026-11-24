package homeask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	CujgelMaxBytes      = 16384
	cujgelIDPattern     = `^[A-Za-z0-9][A-Za-z0-9_-]{0,127}\z`
	cujgelOptionPattern = `^[a-z0-9][a-z0-9._-]{0,127}\z`
	cujgelHashPattern   = `^[a-f0-9]{64}\z`
	cujgelMinVariants   = 2
	cujgelMaxVariants   = 4
	cujgelMinRevision   = 1
)

var (
	cujgelID              = regexp.MustCompile(cujgelIDPattern)
	cujgelOption          = regexp.MustCompile(cujgelOptionPattern)
	cujgelHash            = regexp.MustCompile(cujgelHashPattern)
	cujgelRequired        = []string{"project", "comparison_id", "question_id", "comparison_rev", "variants"}
	cujgelRevRequired     = []string{"revision", "sha256"}
	cujgelVariantRequired = []string{"variant_id", "option_id"}
)

// CujgelLink is metadata for a comparison. Navigation is derived from the IDs by the UI.
type CujgelLink struct {
	Project       string          `json:"project"`
	ComparisonID  string          `json:"comparison_id"`
	QuestionID    string          `json:"question_id"`
	ComparisonRev CujgelRevision  `json:"comparison_rev"`
	Variants      []CujgelVariant `json:"variants"`
}
type CujgelRevision struct {
	Revision json.Number `json:"revision"`
	SHA256   string      `json:"sha256"`
}
type CujgelVariant struct {
	VariantID string `json:"variant_id"`
	OptionID  string `json:"option_id"`
}

func cujgelError(field, why string) error {
	return fmt.Errorf("%w: cujgel/v1 %s: %s", ErrInvalid, field, why)
}

// ReadCujgelFile rejects links and special files before opening, and bounds the read.
func ReadCujgelFile(path string) (*CujgelLink, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, cujgelError("file", "must be a regular file, not a symlink")
	}
	f, err := openEvidenceFile(path) // no-follow, non-blocking on Unix
	if err != nil {
		return nil, cujgelError("file", "cannot read regular file")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || !os.SameFile(before, st) {
		return nil, cujgelError("file", "must be the same regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(f, CujgelMaxBytes+1))
	if err != nil {
		return nil, cujgelError("file", "cannot read regular file")
	}
	return ParseCujgel(string(raw))
}

// cujgelJSON reads tokens so duplicate keys cannot be silently overwritten. Number
// tokens remain exact; the schema's integer check is applied to the revision below.
func cujgelJSON(d *json.Decoder, path string) (any, error) {
	t, err := d.Token()
	if err != nil {
		return nil, cujgelError(path, "invalid JSON")
	}
	switch t {
	case json.Delim('{'):
		out := map[string]any{}
		for d.More() {
			t, err := d.Token()
			if err != nil {
				return nil, cujgelError(path, "invalid JSON")
			}
			key, ok := t.(string)
			if !ok {
				return nil, cujgelError(path, "invalid object key")
			}
			field := key
			if path != "" {
				field = path + "." + key
			}
			if _, ok := out[key]; ok {
				return nil, cujgelError(field, "duplicate key")
			}
			v, err := cujgelJSON(d, field)
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		if _, err := d.Token(); err != nil {
			return nil, cujgelError(path, "invalid JSON")
		}
		return out, nil
	case json.Delim('['):
		out := []any{}
		for d.More() {
			v, err := cujgelJSON(d, fmt.Sprintf("%s[%d]", path, len(out)))
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if _, err := d.Token(); err != nil {
			return nil, cujgelError(path, "invalid JSON")
		}
		return out, nil
	}
	return t, nil
}

func cujgelObject(v any, path string, required []string) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, cujgelError(path, "must be an object")
	}
	for _, key := range required {
		if _, ok := m[key]; !ok {
			return nil, cujgelError(path+"."+key, "required field")
		}
	}
	for key := range m {
		if !slices.Contains(required, key) {
			return nil, cujgelError(path+"."+key, "unknown field")
		}
	}
	return m, nil
}

func cujgelString(m map[string]any, key, path string, pattern *regexp.Regexp) (string, error) {
	s, ok := m[key].(string)
	if !ok || !pattern.MatchString(s) {
		return "", cujgelError(path+"."+key, "invalid string")
	}
	return s, nil
}

// ParseCujgel implements the pinned closed schema and the context-free semantic
// checks. Project binding and membership in this card's options are checked at filing.
func ParseCujgel(body string) (*CujgelLink, error) {
	if len(body) > CujgelMaxBytes {
		return nil, cujgelError("block", "must be no larger than 16 KiB")
	}
	if !utf8.ValidString(body) {
		return nil, cujgelError("block", "must be UTF-8 JSON")
	}
	d := json.NewDecoder(strings.NewReader(body))
	d.UseNumber()
	v, err := cujgelJSON(d, "")
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, cujgelError("block", "unexpected trailing JSON")
	}
	m, err := cujgelObject(v, "block", cujgelRequired)
	if err != nil {
		return nil, err
	}
	var link CujgelLink
	for _, field := range []struct {
		key    string
		target *string
	}{{"project", &link.Project}, {"comparison_id", &link.ComparisonID}, {"question_id", &link.QuestionID}} {
		*field.target, err = cujgelString(m, field.key, "block", cujgelID)
		if err != nil {
			return nil, err
		}
	}
	if link.QuestionID != link.ComparisonID {
		return nil, cujgelError("question_id", "must equal comparison_id in v1")
	}
	rev, err := cujgelObject(m["comparison_rev"], "comparison_rev", cujgelRevRequired)
	if err != nil {
		return nil, err
	}
	n, ok := rev["revision"].(json.Number)
	// Accept integral JSON numbers (including 1.0), without rounding or int64 overflow.
	// Reject numbers outside the finite float64 range, as required for link JSON.
	if !ok {
		return nil, cujgelError("comparison_rev.revision", "must be a finite integer of 1 or more")
	}
	if f, e := n.Float64(); e != nil || f < cujgelMinRevision {
		return nil, cujgelError("comparison_rev.revision", "must be a finite integer of 1 or more")
	}
	rat, valid := new(big.Rat).SetString(string(n))
	if !valid || !rat.IsInt() || rat.Cmp(big.NewRat(cujgelMinRevision, 1)) < 0 {
		return nil, cujgelError("comparison_rev.revision", "must be a finite integer of 1 or more")
	}
	link.ComparisonRev.Revision = n
	link.ComparisonRev.SHA256, err = cujgelString(rev, "sha256", "comparison_rev", cujgelHash)
	if err != nil {
		return nil, err
	}
	variants, ok := m["variants"].([]any)
	if !ok || len(variants) < cujgelMinVariants || len(variants) > cujgelMaxVariants {
		return nil, cujgelError("variants", "must contain 2 to 4 variants")
	}
	seen := map[string]bool{}
	for i, v := range variants {
		path := fmt.Sprintf("variants[%d]", i)
		variant, err := cujgelObject(v, path, cujgelVariantRequired)
		if err != nil {
			return nil, err
		}
		id, err := cujgelString(variant, "variant_id", path, cujgelID)
		if err != nil {
			return nil, err
		}
		option, err := cujgelString(variant, "option_id", path, cujgelOption)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, cujgelError(path+".variant_id", "must appear only once")
		}
		seen[id] = true
		link.Variants = append(link.Variants, CujgelVariant{VariantID: id, OptionID: option})
	}
	return &link, nil
}

func (link *CujgelLink) checkOptions(ask map[string]any) error {
	options := map[string]bool{}
	list, _ := ask["options"].([]any)
	for _, v := range list {
		if m, ok := v.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				options[id] = true
			}
		}
	}
	for i, v := range link.Variants {
		if !optionIDRe.MatchString(v.OptionID) || !options[v.OptionID] {
			return cujgelError(fmt.Sprintf("variants[%d].option_id", i), "must be one of this card's option ids (lowercase letters, digits and hyphens, 1-32 characters)")
		}
	}
	return nil
}

func (r CardRequest) cujgelBlockChecked() (string, error) {
	if r.Secret != nil {
		return "", cujgelError("block", "--cujgel cannot be combined with --secret")
	}
	raw, err := json.Marshal(r.Cujgel)
	if err != nil {
		return "", cujgelError("block", "invalid JSON")
	}
	link, err := ParseCujgel(string(raw))
	if err != nil {
		return "", err
	}
	if err = link.checkOptions(r.Ask); err != nil {
		return "", err
	}
	return "```cujgel/v1\n" + string(raw) + "\n```\n", nil
}

func cujgelEqual(a, b *CujgelLink) bool {
	x, ex := json.Marshal(a)
	y, ey := json.Marshal(b)
	return ex == nil && ey == nil && bytes.Equal(x, y)
}

// checkCujgelFiling uses the target Tasks board's explicit BB link, never the
// caller's ambient BB_PROJECT_ID or the Home project's display name.
func (f *CardFiler) checkCujgelFiling(ctx context.Context, r CardRequest) error {
	if r.Cujgel == nil {
		return nil
	}
	if strings.TrimSpace(r.Project) == "" {
		return cujgelError("project", "target Tasks project is required")
	}
	if _, err := r.cujgelBlockChecked(); err != nil {
		return err
	}
	var result struct {
		Projects []struct {
			ID                string `json:"id"`
			Name              string `json:"name"`
			Prefix            string `json:"prefix"`
			LinkedBBProjectID string `json:"linkedBbProjectId"`
		} `json:"projects"`
	}
	if err := f.call(ctx, "", &result, "tasks", "project", "list", "--json"); err != nil {
		return cujgelError("project", "cannot verify the Tasks linkedBbProjectId")
	}
	count := 0
	linked := ""
	for _, p := range result.Projects {
		if p.ID == r.Project || strings.EqualFold(p.Name, r.Project) || strings.EqualFold(p.Prefix, r.Project) {
			count++
			linked = p.LinkedBBProjectID
		}
	}
	if count != 1 || linked == "" {
		return cujgelError("project", "requires one Tasks project with a linkedBbProjectId")
	}
	if linked != r.Cujgel.Project {
		return cujgelError("project", "must equal the Tasks project's linkedBbProjectId")
	}
	return nil
}
