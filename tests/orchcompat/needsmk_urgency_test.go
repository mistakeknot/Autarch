package orchcompat

import (
	"os/exec"
	"strings"
	"testing"
)

func urgencySupported(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command(autarchBin, "needs-mk", "file", "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("needs-mk file --help: %v\n%s", err, out)
	}
	return strings.Contains(string(out), "--urgency")
}

func TestUrgencyBlockerWhereTheBuildHasIt(t *testing.T) {
	if !urgencySupported(t) {
		t.Skip("this build predates the urgency field; TestUrgencyAbsentWhereTheBuildLacksIt pins that side")
	}
	for _, c := range []struct {
		name  string
		extra map[string]any
		more  []string
	}{
		{name: "urgency flag", more: []string{"--urgency", "blocker"}},
		{name: "urgency in the ask file", extra: map[string]any{"urgency": "blocker"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			ask := e.writeFile("ask.json", askJSON(c.extra))
			r := e.autarch(fileArgs(ask, c.more...)...)
			wantCode(t, r, 0)
			wire := askMap(t, c.extra)
			wire["urgency"] = "blocker"
			o := oracleReq{project: "alpha", title: "Ship it?", ask: wire}
			ident := oracleIdent(t, o)
			key := oracleKey("thr_a", ident)
			if got := parseFiled(t, r).Request; got != key {
				t.Errorf("request = %s, want %s", got, key)
			}
			if d := e.st().Tasks[0].Desc; d != oracleDesc(o, key, ident, "") {
				t.Errorf("description:\n%s\nwant:\n%s", d, oracleDesc(o, key, ident, ""))
			}
		})
	}
	for _, c := range []struct {
		name  string
		extra map[string]any
		more  []string
		err   string
	}{
		{"urgency other than blocker in the ask", map[string]any{"urgency": "low"}, nil, `"urgency" in the ask must be "blocker" when set`},
		{"urgency flag other than blocker", nil, []string{"--urgency", "high"}, `--urgency: the only value is "blocker"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			ask := e.writeFile("ask.json", askJSON(c.extra))
			r := e.autarch(fileArgs(ask, c.more...)...)
			wantCode(t, r, 2, c.err)
			if r.Stdout != "" {
				t.Errorf("stdout = %q, want empty", r.Stdout)
			}
			nothingFiled(t, e)
		})
	}
}

func TestUrgencyAbsentWhereTheBuildLacksIt(t *testing.T) {
	if urgencySupported(t) {
		t.Skip("this build has the urgency field; TestUrgencyBlockerWhereTheBuildHasIt pins that side")
	}
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	r := e.autarch(fileArgs(ask, "--urgency", "blocker")...)
	wantCode(t, r, 1, "unknown flag: --urgency")
	nothingFiled(t, e)

	e = newEnv(t)
	ask = e.writeFile("ask.json", askJSON(map[string]any{"urgency": "blocker"}))
	r = e.autarch(fileArgs(ask)...)
	wantCode(t, r, 2, `unknown field "urgency" in home-ask/v2`)
	nothingFiled(t, e)
}
