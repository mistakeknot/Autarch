package homeask

import (
	"encoding/json"
	"fmt"
	"testing"
	"testing/quick"
)

// Decoding a normalized ask gives the same normalized ask, identity, revision and
// semantic key. This is the twin of the plugin's round-trip property over the same vectors.
func TestVectorsRoundTrip(t *testing.T) {
	for _, c := range loadVectors(t).Asks {
		first, err := Parse(c.Input)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		again, err := Parse([]byte(NormalizedJSON(first)))
		if err != nil {
			// The normalized form carries scope fields flat, which Parse reads back as an ask.
			t.Fatalf("%s: normalized form does not decode: %v", c.Name, err)
		}
		if got := NormalizedJSON(again); got != c.Normalized {
			t.Errorf("%s: round trip changed the ask:\n got %s\nwant %s", c.Name, got, c.Normalized)
		}
		if Identity(again) != c.Identity || Revision(again) != c.Revision || SemanticKey(again) != c.SemanticKey {
			t.Errorf("%s: round trip changed a hash", c.Name)
		}
	}
}

// Normalizing twice equals normalizing once, for generated asks.
func TestNormalizeIdempotent(t *testing.T) {
	words := []string{"alpha", "Beta", "gamma  delta", "É", "é", "x", "A b", " pad ", "ünï", "Z"}
	prop := func(q, s, l1, l2 uint8, n uint16) bool {
		a := Ask{
			V: 1, Kind: "decide", Project: "Autarch", ProjectRoot: "/home/dev/projects/Autarch", Asker: "thread",
			Thread: fmt.Sprintf("thr_%d", n), Question: words[int(q)%len(words)] + "?",
			Options: []Option{{Label: words[int(l1)%len(words)] + " 1"}, {Label: words[int(l2)%len(words)] + " 2"}},
		}
		if s%2 == 0 {
			a.Subject = "s " + words[int(s)%len(words)]
		}
		once, err := Normalize(a)
		if err != nil {
			t.Logf("generated ask invalid: %v", err)
			return false
		}
		raw, err := json.Marshal(once)
		if err != nil {
			return false
		}
		twice, err := Parse(raw)
		if err != nil {
			t.Logf("re-parse failed: %v", err)
			return false
		}
		return NormalizedJSON(once) == NormalizedJSON(twice) && Identity(once) == Identity(twice)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatal(err)
	}
}
