package homeask

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestRound2FullCapSkipsChecksAndWakes(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "1")
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	if e := f.SetPilot("daycap", true); e != nil {
		t.Fatal(e)
	}
	r := testReq()
	r.Key = "delivered"
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	r.Key = "held"
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	if e := f.SetPilot("linkcheck", true); e != nil {
		t.Fatal(e)
	}
	calls, wakes := 0, 0
	f.Links = reviewLinks(func(context.Context, CardRequest) []LinkFailure {
		calls++
		return []LinkFailure{{Item: "https://preview.invalid/gone", Reason: "HTTP 502"}}
	})
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) > 1 && args[0] == "thread" && args[1] == "tell" {
			wakes++
			return BBResult{}
		}
		return bb.run(ctx, env, args...)
	}
	report, e := f.ReleaseDetailed(context.Background())
	if e != nil || calls != 0 || wakes != 0 || len(report.Failures) != 0 {
		t.Fatalf("full cap must skip slow checks: calls=%d wakes=%d report=%+v err=%v", calls, wakes, report, e)
	}
}
func TestRound2AttemptedHeldRetryRecoversCanonicalCard(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	r := testReq()
	first, e := f.FileCard(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	s, p, done, e := f.openState()
	if e != nil {
		t.Fatal(e)
	}
	v := s[r.Key]
	v.Held = true
	v.Attempted = true
	s[r.Key] = v
	if e = saveFilingState(p, s); e != nil {
		t.Fatal(e)
	}
	done()
	if e = f.SetPilot("daycap", true); e != nil {
		t.Fatal(e)
	}
	res, e := f.FileCard(context.Background(), r)
	if e != nil || res.Queued || !res.Replay || res.ID != first.ID {
		t.Fatalf("attempted delivery masked by held replay: %+v %v", res, e)
	}
}
func TestRound2PrivateGitHubHostsAreUnknown(t *testing.T) {
	for _, host := range []string{"www.github.com", "api.github.com", "raw.githubusercontent.com", "gist.github.com"} {
		t.Run(host, func(t *testing.T) {
			gets, unknown := 0, 0
			c := ArtifactLinkChecker{Client: &http.Client{Transport: reviewTransport(func(*http.Request) (*http.Response, error) {
				gets++
				return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
			})}, Unknown: func(string) { unknown++ }}
			r := testReq()
			r.Ask["question"] = "Read https://" + host + "/example/private/blob/main/notes.md ?"
			v := c.Check(context.Background(), r)
			if len(v) != 0 || gets != 0 || unknown != 1 {
				t.Fatalf("private content fetched anonymously: failures=%v gets=%d unknown=%d", v, gets, unknown)
			}
		})
	}
}
func TestRound2HostLookalikeStillChecked(t *testing.T) {
	calls := 0
	c := ArtifactLinkChecker{Client: &http.Client{Transport: reviewTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
	})}}
	r := testReq()
	r.Ask["question"] = "https://github.com.example.invalid/example/private"
	v := c.Check(context.Background(), r)
	if calls != 1 || len(v) != 1 || !strings.Contains(v[0].Reason, "404") {
		t.Fatal(calls, v)
	}
}
