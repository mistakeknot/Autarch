package orchcompat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func dropEmpty(v any) (any, bool) {
	switch x := v.(type) {
	case nil:
		return nil, false
	case string:
		return x, x != ""
	case bool:
		return x, x
	case []any:
		out := []any{}
		for _, e := range x {
			if c, ok := dropEmpty(e); ok {
				out = append(out, c)
			}
		}
		return out, len(out) > 0
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if c, ok := dropEmpty(e); ok {
				out[k] = c
			}
		}
		return out, len(out) > 0
	}
	return v, true
}

func oracleCanon(t *testing.T, v any) string {
	t.Helper()
	c, _ := dropEmpty(v)
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

type oracleReq struct {
	project string
	title   string
	blocks  []string
	ask     map[string]any
	rootRun map[string]any
	move    map[string]any
}

func (o oracleReq) wire() map[string]any {
	m := map[string]any{}
	for k, v := range o.ask {
		m[k] = v
	}
	m["schema"] = "home-ask/v2"
	return m
}

func oracleIdent(t *testing.T, o oracleReq) string {
	t.Helper()
	seen := map[string]bool{}
	var blocks []any
	var bs []string
	for _, b := range o.blocks {
		if !seen[b] {
			seen[b] = true
			bs = append(bs, b)
		}
	}
	sort.Strings(bs)
	for _, b := range bs {
		blocks = append(blocks, b)
	}
	wire := o.wire()
	node := map[string]any{"project": wire["project"], "title": o.title, "blocks": blocks, "ask": wire}
	if o.rootRun != nil {
		node["root_run"] = o.rootRun
	}
	if o.move != nil {
		node["move"] = o.move
	}
	sum := sha256.Sum256([]byte(oracleCanon(t, node)))
	return hex.EncodeToString(sum[:])[:16]
}

func oracleKey(thread, ident string) string {
	ns := uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://autarch.invalid/needs-mk"))
	return uuid.NewSHA1(ns, []byte("thread\x00"+thread+"\x00"+ident)).String()
}

func oracleDesc(o oracleReq, key, ident string, tail string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(o.ask["question"].(string)))
	b.WriteString("\n\n")
	var uniq []string
	seen := map[string]bool{}
	for _, x := range o.blocks {
		if !seen[x] {
			seen[x] = true
			uniq = append(uniq, x)
		}
	}
	if len(uniq) > 0 {
		b.WriteString("Blocks: " + strings.Join(uniq, " ") + "\n")
	}
	body, _ := json.Marshal(o.wire())
	b.WriteString("Request: " + key + " sha256:" + ident + "\n\n```home-ask\n" + string(body) + "\n```\n")
	b.WriteString(tail)
	return b.String()
}

func askMap(t *testing.T, extra map[string]any) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(askJSON(extra)), &m); err != nil {
		t.Fatal(err)
	}
	m["project"] = "alpha"
	return m
}

func heads(cs []fbCall, n int) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = strings.Join(c.Args[:min(n, len(c.Args))], " ")
	}
	return out
}
