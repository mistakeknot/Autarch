package homeask

// The CLI's v2 map route must not preserve case-folded nested keys that the
// strict TS parser refuses. Leave valid values/omissions unchanged so existing
// Request identities remain stable. The typed v1 route already re-encodes them.
func exactCardNestedKeys(ask map[string]any) error {
	options, _ := ask["options"].([]any)
	for _, raw := range options {
		option, ok := raw.(map[string]any)
		if !ok {
			continue
		} // ToV1 checks value types.
		if err := acExactKeys(option, []string{"id", "label", "kind", "reversible", "instruction", "approval", "on_pick"}, "option"); err != nil {
			return err
		}
		if approval, ok := option["approval"].(map[string]any); ok {
			if err := acExactKeys(approval, []string{"kind", "target", "identity", "ttl"}, "approval"); err != nil {
				return err
			}
		}
	}
	questions, _ := ask["questions"].([]any)
	for _, raw := range questions {
		question, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if err := acExactKeys(question, []string{"id", "prompt", "recommendation", "options"}, "question"); err != nil {
			return err
		}
		choices, _ := question["options"].([]any)
		for _, raw := range choices {
			choice, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := acExactKeys(choice, []string{"id", "label", "detail"}, "question option"); err != nil {
				return err
			}
		}
	}
	return nil
}
