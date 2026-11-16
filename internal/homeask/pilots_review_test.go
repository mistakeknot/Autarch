package homeask

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type reviewTransport func(*http.Request) (*http.Response, error)

func (f reviewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestReviewPrivateGitHubURLNotAnonymousDead(t *testing.T) {
	calls := 0
	prs := 0
	c := ArtifactLinkChecker{Client: &http.Client{Transport: reviewTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 404, Body: http.NoBody, Header: make(http.Header)}, nil
	})}, PR: func(context.Context, string) (string, error) { prs++; return "abcdef0123456789", nil }}
	r := testReq()
	r.Ask["question"] = "See https://github.com/example/private/blob/main/notes.md and https://github.com/example/private/tree/main and https://github.com/example/private and https://github.com/example/private/pull/12"
	v := c.Check(context.Background(), r)
	if len(v) != 0 || calls != 0 || prs != 1 {
		t.Fatalf("private GitHub URLs must not use anonymous GET: failures=%v GETs=%d PRs=%d", v, calls, prs)
	}
}
func TestReviewPRHeadMustBeBound(t *testing.T) {
	for _, text := range []string{"example/demo#12 discussed on 20261115", "script digest abcdef0123456789 is unrelated to example/demo#12", "example/demo#12 and example/demo#13 head fedcba0987654321"} {
		t.Run(text, func(t *testing.T) {
			r := testReq()
			r.Ask["question"] = text
			c := ArtifactLinkChecker{PR: func(_ context.Context, ref string) (string, error) {
				if strings.HasSuffix(ref, "#13") {
					return "fedcba0987654321", nil
				}
				return "11111111abcdef01", nil
			}}
			if v := c.Check(context.Background(), r); len(v) != 0 {
				t.Fatal("unbound hex must not become a PR head", v)
			}
		})
	}
}
func TestReviewWakeKeyStableStatusClass(t *testing.T) {
	a := []LinkFailure{{Item: "https://preview.invalid/demo", Reason: "dial tcp: connection refused"}}
	b := []LinkFailure{{Item: a[0].Item, Reason: "Get failed: DNS timeout"}}
	if failureSet(a) != failureSet(b) {
		t.Fatal("same dead connection class changed wake key")
	}
	a[0].Reason = "HTTP 502"
	b[0].Reason = "HTTP 503"
	if failureSet(a) != failureSet(b) {
		t.Fatal("same 5xx class changed wake key")
	}
	b[0].Reason = "HTTP 404"
	if failureSet(a) == failureSet(b) {
		t.Fatal("distinct status classes share key")
	}
}

type reviewLinks func(context.Context, CardRequest) []LinkFailure

func (f reviewLinks) Check(ctx context.Context, r CardRequest) []LinkFailure { return f(ctx, r) }
func TestReviewWakeKeyClearedAfterRecovery(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	r := testReq()
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	s, p, done, e := f.openState()
	if e != nil {
		t.Fatal(e)
	}
	v := s[r.Key]
	v.LinkFailureSet = "old-failure"
	s[r.Key] = v
	if e = saveFilingState(p, s); e != nil {
		t.Fatal(e)
	}
	done()
	f.Links = reviewLinks(func(context.Context, CardRequest) []LinkFailure { return nil })
	if v := f.checkLinks(context.Background(), r); len(v) != 0 {
		t.Fatal(v)
	}
	s, _, done, e = f.openState()
	if e != nil {
		t.Fatal(e)
	}
	defer done()
	if s[r.Key].LinkFailureSet != "" {
		t.Fatal("recovery did not clear failure set")
	}
}
func TestReviewUrgentPrecedentOnlyWarns(t *testing.T) {
	for _, kind := range []string{"design", "product"} {
		for _, exempt := range []string{"urgent", "keep-list"} {
			t.Run(kind+exempt, func(t *testing.T) {
				f := newFake(t, registryOnCreate).filer()
				enablePilot(t, f, "coldread")
				f.ColdReader = &fakeColdReader{body: `{"decision":"Choose","options":{"yes":"Ship","no":"Wait"},"undefined_terms":[],"verdict":"clear"}`}
				r := testReq()
				r.Type = kind
				r.Urgent = exempt == "urgent"
				if exempt == "keep-list" {
					r.KeepList = "spend"
				}
				warn := ""
				f.Warn = func(s string) { warn += s }
				if e := f.checkColdRead(context.Background(), r); e != nil || !strings.Contains(warn, "Precedent:") {
					t.Fatalf("exempt missing precedent must warn: %v %s", e, warn)
				}
			})
		}
	}
}
func TestReviewHeldRetrySkipsChecks(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	r := testReq()
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	enablePilot(t, f, "linkcheck")
	calls := 0
	f.Links = reviewLinks(func(context.Context, CardRequest) []LinkFailure {
		calls++
		return []LinkFailure{{Item: "https://preview.invalid", Reason: "HTTP 502"}}
	})
	res, e := f.FileCard(context.Background(), r)
	if e != nil || !res.Queued || !res.Replay || calls != 0 {
		t.Fatalf("held replay reran checks: %+v %v calls=%d", res, e, calls)
	}
}
func TestReviewSlowChecksOutsideDeliveryGate(t *testing.T) {
	for _, mode := range []string{"submit-links", "submit-cold", "release-links"} {
		t.Run(mode, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			r := testReq()
			r.Key = "slow"
			if mode == "release-links" {
				r.NonBlocking = true
				if _, e := f.FileCard(context.Background(), r); e != nil {
					t.Fatal(e)
				}
			}
			enablePilot(t, f, "daycap")
			pname := "linkcheck"
			if mode == "submit-cold" {
				pname = "coldread"
			}
			if e := f.SetPilot(pname, true); e != nil {
				t.Fatal(e)
			}
			entered := make(chan struct{})
			resume := make(chan struct{})
			var calls atomic.Int32
			slow := func() {
				if calls.Add(1) == 1 {
					close(entered)
					<-resume
				}
			}
			if mode == "submit-cold" {
				f.ColdReader = reviewCold(func(context.Context, string) ([]byte, error) {
					slow()
					return []byte(`{"decision":"Choose","options":{"yes":"Ship","no":"Wait"},"undefined_terms":[],"verdict":"clear"}`), nil
				})
			} else {
				f.Links = reviewLinks(func(context.Context, CardRequest) []LinkFailure { slow(); return nil })
			}
			done := make(chan error, 1)
			go func() {
				if mode == "release-links" {
					_, e := f.Release(context.Background())
					done <- e
				} else {
					_, e := f.FileCard(context.Background(), r)
					done <- e
				}
			}()
			<-entered
			gate := make(chan error, 1)
			go func() {
				unlock, e := f.deliveryGate()
				if e == nil {
					unlock()
				}
				gate <- e
			}()
			var gateErr error
			select {
			case gateErr = <-gate:
			case <-time.After(150 * time.Millisecond):
				gateErr = fmt.Errorf("global delivery lock held during %s", mode)
			}
			close(resume)
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			if gateErr != nil {
				t.Fatal(gateErr)
			}
		})
	}
}

type reviewCold func(context.Context, string) ([]byte, error)

func (f reviewCold) Read(ctx context.Context, s string) ([]byte, error) { return f(ctx, s) }
