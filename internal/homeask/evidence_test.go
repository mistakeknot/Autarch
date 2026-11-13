package homeask

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvidenceOnlyUpdateNoop(t *testing.T) {
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "same"
	r.Ask["evidence"] = []any{map[string]any{"caption": "First", "url": "https://example.test/first"}}
	if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	first := f.tasks[0].Desc
	r.Ask["evidence"] = []any{map[string]any{"caption": "Second", "url": "https://example.test/second"}}
	if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	if f.tasks[0].Desc != first || f.count("tasks", "update") != 0 {
		t.Fatal("evidence-only update was not a no-op")
	}
}

func TestEvidenceParse(t *testing.T) {
	raw, _ := json.Marshal(base())
	var m map[string]any
	json.Unmarshal(raw, &m)
	m["evidence"] = []any{map[string]any{"caption": "Run", "url": "https://example.test/run"}}
	raw, _ = json.Marshal(m)
	if _, err := Parse(raw); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceHashesUnchanged(t *testing.T) {
	for _, c := range loadVectors(t).Asks {
		a, err := Parse(c.Input)
		if err != nil {
			t.Fatal(err)
		}
		a.Evidence = EvidenceList{{Caption: "Run", URL: "https://example.test/run"}}
		if Identity(a) != c.Identity || Revision(a) != c.Revision || SemanticKey(a) != c.SemanticKey || NormalizedJSON(a) != c.Normalized {
			t.Fatal("existing fixture hashes changed", c.Name)
		}
	}
	a := mustNorm(t, base())
	b := a
	b.Evidence = EvidenceList{{Caption: "Run", URL: "https://example.test/run"}}
	if Identity(a) != Identity(b) || Revision(a) != Revision(b) || SemanticKey(a) != SemanticKey(b) || NormalizedJSON(a) != NormalizedJSON(b) {
		t.Fatal("evidence leaked into hashes or normalized JSON")
	}
	req := testReq()
	first, err := RequestIdentity(req)
	if err != nil {
		t.Fatal(err)
	}
	req.Ask["evidence"] = []any{map[string]any{"caption": "Run", "url": "https://example.test/run"}}
	second, err := RequestIdentity(req)
	if err != nil || first != second {
		t.Fatal("evidence changed Request identity", err)
	}
	desc, _, err := req.Description()
	if err != nil || !strings.Contains(desc, "https://example.test/run") {
		t.Fatal("delivery lost evidence", err)
	}
}

func TestEvidenceSecurity(t *testing.T) {
	for _, u := range []string{"http://example.test", "javascript:alert(1)", "data:image/png,hi", "file:///test", "https://user:pass@example.test", "https://@example.test", "https://example.test/\n", "https://example.test/" + strings.Repeat("a", 2048), "https:///missing", "https://example.test/\\bad"} {
		a := base()
		a.Evidence = EvidenceList{{Caption: "Run", URL: u}}
		if _, err := Normalize(a); err == nil {
			t.Errorf("accepted URL %q", u)
		}
	}
	for _, e := range []Evidence{
		{Caption: "", URL: "https://example.test"}, {Caption: strings.Repeat("a", 201), URL: "https://example.test"},
		{Caption: "x\nx", URL: "https://example.test"}, {Caption: "x", Before: &EvidenceSide{URL: "https://example.test"}},
		{Caption: "x", URL: "https://example.test", Image: &EvidenceImage{SHA256: strings.Repeat("a", 64), MIME: "image/png"}},
		{Caption: "x", Image: &EvidenceImage{SHA256: "../test", MIME: "image/png"}},
		{Caption: "x", Image: &EvidenceImage{SHA256: strings.Repeat("a", 64), MIME: "image/svg+xml"}},
	} {
		if err := ValidateEvidence([]Evidence{e}); err == nil {
			t.Errorf("accepted %#v", e)
		}
	}
	for _, s := range []string{`null`, `[{"caption":"x","image_path":"/example"}]`, `[{"caption":"x","url":"","image":{"sha256":"` + strings.Repeat("a", 64) + `","mime":"image/png"}}]`, `[{"caption":"x","before":null,"after":{"url":"https://example.test"}}]`, `[{"caption":"x","image":{"sha256":"` + strings.Repeat("a", 64) + `","mime":"image/png","path":"/example"}}]`} {
		raw, _ := json.Marshal(base())
		var m map[string]any
		json.Unmarshal(raw, &m)
		m["evidence"] = json.RawMessage(s)
		raw, _ = json.Marshal(m)
		if _, err := Parse(raw); err == nil {
			t.Errorf("accepted raw evidence %s", s)
		}
	}
	if err := ValidateEvidence(make([]Evidence, 7)); err == nil {
		t.Fatal("accepted 7 items")
	}
}

func imageBytes(n int) []byte {
	b := make([]byte, n)
	copy(b, []byte{137, 80, 78, 71, 13, 10, 26, 10})
	return b
}
func TestEvidenceImport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	t.Setenv("AUTARCH_HOME_EVIDENCE", root)
	path := filepath.Join(t.TempDir(), "image.txt")
	os.WriteFile(path, imageBytes(32), 0600)
	ask := map[string]any{"evidence": []any{map[string]any{"caption": "Screenshot", "image_path": path}}}
	if err := PrepareEvidence(ask); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(ask)
	if bytes.Contains(raw, []byte(path)) || bytes.Contains(raw, []byte("image_path")) {
		t.Fatal("local path written into stored ask")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	data, err := os.ReadFile(filepath.Join(root, entries[0].Name()))
	if err != nil || !bytes.Equal(data, imageBytes(32)) {
		t.Fatal("stored bytes", err)
	}
	for _, p := range []string{root, filepath.Join(root, entries[0].Name())} {
		st, _ := os.Stat(p)
		want := os.FileMode(0600)
		if st.IsDir() {
			want = 0700
		}
		if st.Mode().Perm() != want {
			t.Fatal("wrong permissions")
		}
	}
	os.WriteFile(path, []byte("not an image"), 0600)
	bad := map[string]any{"evidence": []any{map[string]any{"caption": "Bad", "image_path": path + ".png"}}}
	os.WriteFile(path+".png", []byte("not an image"), 0600)
	if err := PrepareEvidence(bad); err == nil {
		t.Fatal("accepted extension-only image")
	}
}
func TestEvidenceSizeBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image")
	t.Setenv("AUTARCH_HOME_EVIDENCE", filepath.Join(t.TempDir(), "store"))
	for _, n := range []int{EvidenceImageMax, EvidenceImageMax + 1} {
		os.WriteFile(path, imageBytes(n), 0600)
		a := map[string]any{"evidence": []any{map[string]any{"caption": "Size", "image_path": path}}}
		err := PrepareEvidence(a)
		if (err == nil) != (n == EvidenceImageMax) {
			t.Fatalf("size %d: %v", n, err)
		}
	}
	os.WriteFile(path, imageBytes(EvidenceImageMax), 0600)
	for _, n := range []int{4, 5} {
		var items []any
		for i := 0; i < n; i++ {
			items = append(items, map[string]any{"caption": "Size", "image_path": path})
		}
		err := PrepareEvidence(map[string]any{"evidence": items})
		if (err == nil) != (n == 4) {
			t.Fatalf("total images %d: %v", n, err)
		}
	}
	// Exactly one byte over the card cap, while every individual file is valid.
	small := filepath.Join(t.TempDir(), "small")
	os.WriteFile(small, imageBytes(8), 0600)
	almost := filepath.Join(t.TempDir(), "almost")
	os.WriteFile(almost, imageBytes(EvidenceImageMax-7), 0600)
	var items []any
	for _, p := range []string{path, path, path, almost, small} {
		items = append(items, map[string]any{"caption": "Total", "image_path": p})
	}
	if err := PrepareEvidence(map[string]any{"evidence": items}); err == nil {
		t.Fatal("accepted 8 MB plus one byte")
	}
}

func TestEvidenceFormatsAndPairs(t *testing.T) {
	for _, b := range [][]byte{imageBytes(8), {255, 216, 255, 224}, []byte("GIF87a"), []byte("GIF89a"), []byte("RIFF0000WEBP")} {
		if evidenceMIME(b) == "" {
			t.Fatal("format refused")
		}
	}
	a := base()
	a.Evidence = EvidenceList{{Caption: strings.Repeat("é", 200), Before: &EvidenceSide{URL: "https://example.test/before"}, After: &EvidenceSide{Image: &EvidenceImage{SHA256: strings.Repeat("a", 64), MIME: "image/png"}}}}
	raw, _ := json.Marshal(a)
	if _, err := Parse(raw); err != nil {
		t.Fatal(err)
	}
}
