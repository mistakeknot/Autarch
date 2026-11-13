package homeask

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const EvidenceImageMax = 2 * 1024 * 1024
const EvidenceTotalMax = 8 * 1024 * 1024

type EvidenceImage struct {
	SHA256 string `json:"sha256"`
	MIME   string `json:"mime"`
}

type EvidenceList []Evidence

func (e *EvidenceList) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return fmt.Errorf("evidence must be an array")
	}
	var list []Evidence
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	*e = list
	return nil
}

// Decode to a map before a struct: keys are exact and duplicate JSON members
// replace the whole previous value, matching JSON.parse (never struct merging).
func evidenceObject(b []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		return nil, fmt.Errorf("evidence must be an object")
	}
	keys := make(map[string]bool, len(allowed))
	for _, k := range allowed {
		keys[k] = true
	}
	for k := range m {
		if !keys[k] {
			return nil, fmt.Errorf("unknown evidence field %q", k)
		}
	}
	return m, nil
}

func evidenceShape(b []byte, pair bool) (map[string]json.RawMessage, error) {
	allowed := []string{"url", "image"}
	if pair {
		allowed = append(allowed, "caption", "before", "after")
	}
	m, err := evidenceObject(b, allowed...)
	if err != nil {
		return nil, err
	}
	_, u := m["url"]
	_, im := m["image"]
	_, before := m["before"]
	_, after := m["after"]
	if pair && (before || after) {
		if !before || !after || u || im || bytes.Equal(m["before"], []byte("null")) || bytes.Equal(m["after"], []byte("null")) {
			return nil, fmt.Errorf("evidence requires a before/after pair")
		}
	} else if u == im || (im && bytes.Equal(m["image"], []byte("null"))) {
		return nil, fmt.Errorf("evidence needs exactly one url or image")
	}
	return m, nil
}
func (e *Evidence) UnmarshalJSON(b []byte) error {
	m, err := evidenceShape(b, true)
	if err != nil {
		return err
	}
	b, _ = json.Marshal(m)
	type alias Evidence
	var v alias
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return err
	}
	*e = Evidence(v)
	return nil
}
func (e *EvidenceSide) UnmarshalJSON(b []byte) error {
	m, err := evidenceShape(b, false)
	if err != nil {
		return err
	}
	b, _ = json.Marshal(m)
	type alias EvidenceSide
	var v alias
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return err
	}
	*e = EvidenceSide(v)
	return nil
}

func (e *EvidenceImage) UnmarshalJSON(b []byte) error {
	m, err := evidenceObject(b, "sha256", "mime")
	if err != nil {
		return err
	}
	b, _ = json.Marshal(m)
	type plain EvidenceImage
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*e = EvidenceImage(v)
	return nil
}

// Retain validated structs at every filing boundary; raw caller maps never
// determine evidence member spelling or duplicate-object behavior in storage.
func evidenceFromWire(raw any) (EvidenceList, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var list EvidenceList
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	if err := ValidateEvidence(list); err != nil {
		return nil, err
	}
	return list, nil
}

type EvidenceSide struct {
	URL   string         `json:"url,omitempty"`
	Image *EvidenceImage `json:"image,omitempty"`
}
type Evidence struct {
	Caption string         `json:"caption"`
	URL     string         `json:"url,omitempty"`
	Image   *EvidenceImage `json:"image,omitempty"`
	Before  *EvidenceSide  `json:"before,omitempty"`
	After   *EvidenceSide  `json:"after,omitempty"`
}

var evidenceHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

func evidenceText(s string) bool {
	for _, r := range s {
		if r <= 0x1f || (r >= 0x7f && r <= 0x9f) {
			return false
		}
	}
	return utf8.ValidString(s)
}
func validateSide(s EvidenceSide) error {
	if (s.URL == "") == (s.Image == nil) {
		return fmt.Errorf("evidence needs exactly one url or image")
	}
	if s.Image != nil {
		if !evidenceHash.MatchString(s.Image.SHA256) || (s.Image.MIME != "image/png" && s.Image.MIME != "image/jpeg" && s.Image.MIME != "image/gif" && s.Image.MIME != "image/webp") {
			return fmt.Errorf("invalid evidence image reference")
		}
	} else {
		if !evidenceURL(s.URL) {
			return fmt.Errorf("evidence url must be absolute HTTPS without credentials or controls")
		}
	}
	return nil
}
func ValidateEvidence(list []Evidence) error {
	if len(list) > 6 {
		return fmt.Errorf("evidence takes at most 6 items")
	}
	for _, e := range list {
		if strings.TrimFunc(e.Caption, evidenceWhitespace) == "" || utf8.RuneCountInString(e.Caption) > 200 || !evidenceText(e.Caption) {
			return fmt.Errorf("evidence caption must be 1-200 plain text characters")
		}
		if e.Before != nil || e.After != nil {
			if e.Before == nil || e.After == nil || e.URL != "" || e.Image != nil {
				return fmt.Errorf("evidence needs a before/after pair or one url or image")
			}
			if err := validateSide(*e.Before); err != nil {
				return err
			}
			if err := validateSide(*e.After); err != nil {
				return err
			}
		} else if err := validateSide(EvidenceSide{e.URL, e.Image}); err != nil {
			return err
		}
	}
	return nil
}

func EvidenceRoot() string {
	if p := os.Getenv("AUTARCH_HOME_EVIDENCE"); p != "" && filepath.IsAbs(p) {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".autarch", "home-evidence")
}

// Compare delivery text while treating an evidence-only re-filing as a no-op.
func descriptionWithoutEvidence(desc string) string {
	const fence = "```home-ask\n"
	start := strings.Index(desc, fence)
	if start < 0 {
		return desc
	}
	start += len(fence)
	end := strings.Index(desc[start:], "\n```")
	if end < 0 {
		return desc
	}
	end += start
	var m map[string]any
	if json.Unmarshal([]byte(desc[start:end]), &m) != nil {
		return desc
	}
	delete(m, "evidence")
	b, _ := json.Marshal(m)
	return desc[:start] + string(b) + desc[end:]
}
func evidenceMIME(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return "image/png"
	case len(b) >= 4 && bytes.Equal(b[:3], []byte{255, 216, 255}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")):
		return "image/gif"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}
func evidenceRead(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("evidence image must be a regular file")
	}
	f, err := openEvidenceFile(path)
	if err != nil {
		return nil, fmt.Errorf("evidence image unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, fmt.Errorf("evidence image must be a regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, EvidenceImageMax+1))
	if err != nil || len(b) > EvidenceImageMax {
		return nil, fmt.Errorf("evidence image exceeds 2 MB")
	}
	if evidenceMIME(b) == "" {
		return nil, fmt.Errorf("evidence image must be PNG, JPEG, GIF or WebP by magic bytes")
	}
	return b, nil
}

// PrepareEvidence accepts image_path only at the CLI boundary. No local path survives
// into the stored ask. Existing references are checked against the same byte limits.
func PrepareEvidence(ask map[string]any) error {
	raw, ok := ask["evidence"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var list []map[string]any
	if err = json.Unmarshal(data, &list); err != nil || len(list) > 6 {
		return fmt.Errorf("evidence must be a list of at most 6 items")
	}
	total := 0
	var imports = map[string][]byte{}
	var prepare func(map[string]any) error
	prepare = func(side map[string]any) error {
		if path, ok := side["image_path"]; ok {
			p, ok := path.(string)
			if !ok || p == "" || side["url"] != nil || side["image"] != nil {
				return fmt.Errorf("image_path must be the only image source")
			}
			b, err := evidenceRead(p)
			if err != nil {
				return err
			}
			hash := sha256.Sum256(b)
			h := hex.EncodeToString(hash[:])
			imports[h] = b
			delete(side, "image_path")
			side["image"] = map[string]any{"sha256": h, "mime": evidenceMIME(b)}
		}
		if im, ok := side["image"].(map[string]any); ok {
			h, _ := im["sha256"].(string)
			if !evidenceHash.MatchString(h) {
				return fmt.Errorf("invalid evidence hash")
			}
			b, ok := imports[h]
			if !ok {
				b, err = evidenceRead(filepath.Join(EvidenceRoot(), h))
				if err != nil {
					return err
				}
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != h || evidenceMIME(b) != im["mime"] {
				return fmt.Errorf("evidence image reference does not match bytes")
			}
			total += len(b)
			if total > EvidenceTotalMax {
				return fmt.Errorf("evidence images exceed 8 MB per card")
			}
		}
		return nil
	}
	for _, item := range list {
		if err := prepare(item); err != nil {
			return err
		}
		for _, key := range []string{"before", "after"} {
			if s, ok := item[key].(map[string]any); ok {
				if err := prepare(s); err != nil {
					return err
				}
			}
		}
	}
	valid, err := evidenceFromWire(list)
	if err != nil {
		return err
	}
	if len(imports) > 0 {
		root := EvidenceRoot()
		if err := os.MkdirAll(root, 0700); err != nil {
			return fmt.Errorf("cannot create evidence store")
		}
		if st, err := os.Lstat(root); err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid evidence store")
		}
		if err := os.Chmod(root, 0700); err != nil {
			return err
		}
		for h, b := range imports {
			f, err := os.CreateTemp(root, ".evidence-")
			if err != nil {
				return err
			}
			name := f.Name()
			err = f.Chmod(0600)
			if err == nil {
				_, err = f.Write(b)
			}
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
			if err == nil {
				err = os.Rename(name, filepath.Join(root, h))
			}
			if err != nil {
				os.Remove(name)
				return fmt.Errorf("cannot store evidence image")
			}
		}
	}
	ask["evidence"] = valid
	return nil
}
