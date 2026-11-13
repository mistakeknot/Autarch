package homeask

import (
 "encoding/json"
 "testing"
)

func TestEvidenceParse(t *testing.T) {
 raw, _ := json.Marshal(base())
 var m map[string]any
 json.Unmarshal(raw, &m)
 m["evidence"] = []any{map[string]any{"caption":"Run", "url":"https://example.test/run"}}
 raw, _ = json.Marshal(m)
 if _, err := Parse(raw); err != nil { t.Fatal(err) }
}
