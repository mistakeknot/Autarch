package homeask

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
)

// Group links independently filed cards for display only. Order need not be
// contiguous or unique. Nothing waits for another member to be filed or picked.
type Group struct {
	Key   string `json:"key"`
	Order int    `json:"order"`
}

var groupKey = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func validateGroup(g *Group) error {
	if g != nil && (!groupKey.MatchString(g.Key) || g.Order < 1 || g.Order > 2147483647) {
		return fmt.Errorf("invalid group: expected key token and integer order 1-2147483647")
	}
	return nil
}

// Map decoding enforces exact member names and JSON.parse's last-member wins.
func (g *Group) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil || m == nil || len(m) != 2 {
		return fmt.Errorf("invalid group object")
	}
	var key string
	var order float64
	if err := json.Unmarshal(m["key"], &key); err != nil {
		return fmt.Errorf("invalid group key")
	}
	if err := json.Unmarshal(m["order"], &order); err != nil || math.Trunc(order) != order || order < 1 || order > 2147483647 {
		return fmt.Errorf("invalid group order")
	}
	v := Group{Key: key, Order: int(order)}
	if err := validateGroup(&v); err != nil {
		return err
	}
	*g = v
	return nil
}

func groupFromWire(raw any) (*Group, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var g Group
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	return &g, nil
}
