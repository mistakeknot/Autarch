package homeask

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestExpectedOutcomeSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/expected-outcome-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name      string          `json:"name"`
		Wire      json.RawMessage `json:"wire"`
		Stored    json.RawMessage `json:"stored"`
		Canonical string          `json:"canonical"`
		Error     bool            `json:"error"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		t.Run(row.Name, func(t *testing.T) {
			if len(row.Stored) > 0 {
				if _, err := parseAskSource(row.Stored, false, true); err == nil {
					t.Fatal("accepted malformed stored card")
				}
				return
			}
			card, err := ParseCard("Proceed?\nBlocks: thread:thr_fixture\nRequest: fixture sha256:0123456789abcdef\n```home-ask\n" + string(row.Wire) + "\n```")
			if row.Error {
				if err == nil {
					t.Fatal("accepted malformed card metadata")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ask, err := card.ToV1("thr_fixture")
			if err != nil {
				t.Fatal(err)
			}
			body := NormalizedJSON(ask)
			if body != row.Canonical {
				t.Fatalf("canonical ask:\n got %s\nwant %s", body, row.Canonical)
			}
			stored, err := parseAskSource([]byte(body), false, true)
			if err != nil || NormalizedJSON(stored) != body {
				t.Fatalf("stored card changed on readback: %v", err)
			}
			var wire map[string]any
			if err := json.Unmarshal(row.Wire, &wire); err != nil {
				t.Fatal(err)
			}
			request := CardRequest{Title: "Fixture", Key: "fixture", Ask: wire}
			description, _, err := request.Description()
			if row.Name == "absent" {
				if err != nil {
					t.Fatal(err)
				}
				filed, err := ParseCard(description)
				if err != nil {
					t.Fatal(err)
				}
				filedAsk, err := filed.ToV1("thr_fixture")
				if err != nil || NormalizedJSON(filedAsk) != row.Canonical {
					t.Fatalf("legacy filer changed the card body: %v", err)
				}
				rawFiled, err := json.Marshal(filed.Ask)
				if err != nil {
					t.Fatal(err)
				}
				got, err := CanonicalJSON(rawFiled)
				if err != nil {
					t.Fatal(err)
				}
				want, err := CanonicalJSON(row.Wire)
				if err != nil || got != want {
					t.Fatalf("legacy filer changed the wire ask: %v", err)
				}
				return
			}
			if _, err := Parse([]byte(body)); err == nil {
				t.Fatal("accepted card metadata on the non-card request path")
			}
			if err == nil || !strings.Contains(err.Error(), "is not writable yet") {
				t.Fatalf("expected reader-only refusal from card writer, got %v", err)
			}
		})
	}
}

func TestNormalizedJSONSkipsMalformedCardMetadata(t *testing.T) {
	ask := Ask{Options: []Option{{ID: "yes", Label: "Proceed"}}}
	want := NormalizedJSON(ask)
	for _, raw := range []string{`{`, `1.5`, `{} null`} {
		t.Run(raw, func(t *testing.T) {
			ask.ReviewBy = json.RawMessage(raw)
			ask.Options[0].ExpectedOutcome = json.RawMessage(raw)
			if got := NormalizedJSON(ask); got != want {
				t.Fatalf("malformed metadata changed the canonical ask: %s", got)
			}
		})
	}
}
