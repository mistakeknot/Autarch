package homeask

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCujgelSharedNegativeCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/cujgel/negative-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string
		Body string
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no shared negative cases")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if _, err := parseCujgelFixture(t, tc.Body); err == nil {
				t.Fatal("invalid Cujgel block accepted")
			}
		})
	}
}

func TestCujgelSharedPositiveCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/cujgel/positive-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string
		Body string
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no shared positive cases")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if _, err := parseCujgelFixture(t, tc.Body); err != nil {
				t.Fatalf("valid Cujgel block refused: %v", err)
			}
		})
	}
}
