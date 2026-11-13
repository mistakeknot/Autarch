package homeask

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestOnPickSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/on-pick-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		hashGolden
		Error                bool   `json:"error"`
		DeclarationCanonical string `json:"declaration_canonical"`
		DeclarationSHA256    string `json:"declaration_sha256"`
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		t.Run(row.Name, func(t *testing.T) {
			if row.Error {
				if _, err := ParseCard(row.Description); err == nil {
					t.Fatal("accepted invalid shared vector")
				}
				return
			}
			got := goldenHashes(t, row.hashGolden)
			if !reflect.DeepEqual(got, row.Expected) {
				t.Fatalf("got %#v want %#v", got, row.Expected)
			}
			card, err := ParseCard(row.Description)
			if err != nil {
				t.Fatal(err)
			}
			ask, err := card.ToV1(row.Thread)
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := CanonicalOnPick(ask.Options[0].OnPick)
			if err != nil {
				t.Fatal(err)
			}
			if canonical != row.DeclarationCanonical || fmt.Sprintf("%x", sha256.Sum256([]byte(canonical))) != row.DeclarationSHA256 {
				t.Fatalf("complete declaration digest mismatch: %s", canonical)
			}
		})
	}
}

func TestOnPickOpaqueCanonical(t *testing.T) {
	raw := `{"on_pick":{"reversible":false,"ruling":{"bead":"bead-é","under":["basis-é"]}}}`
	got, err := CanonicalJSON([]byte(raw))
	if err != nil || got != raw {
		t.Fatalf("opaque declaration changed: %s (%v)", got, err)
	}
}
