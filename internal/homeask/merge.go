package homeask

import "strings"

// MergePhrase identifies the merge option shared by every card filing path.
const MergePhrase = "Manually merge it at"

// IsMergeAsk recognizes merge option labels at any nesting depth, including
// multi-question asks. Other prose containing the phrase is not a merge option.
func IsMergeAsk(v any) bool {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if IsMergeAsk(item) {
				return true
			}
		}
	case map[string]any:
		for key, item := range x {
			if label, ok := item.(string); ok && strings.EqualFold(key, "label") && strings.Contains(label, MergePhrase+" ") {
				return true
			}
			if IsMergeAsk(item) {
				return true
			}
		}
	}
	return false
}
