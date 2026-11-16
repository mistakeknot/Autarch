package homeask

import "testing"

func TestMergeClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		ask  any
		want bool
	}{
		{"option", map[string]any{"options": []any{map[string]any{"label": MergePhrase + " abc123"}}}, true},
		{"nested", map[string]any{"questions": []any{map[string]any{"options": []any{map[string]any{"LABEL": MergePhrase + " abc123"}}}}}, true},
		{"question only", map[string]any{"question": MergePhrase + " abc123"}, false},
		{"no head", map[string]any{"label": MergePhrase}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsMergeAsk(tc.ask); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
