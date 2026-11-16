package homeask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
	r.Ask["question"] = "See https://github.com/example/private/blob/main/notes.md and https://github.com/example/private/tree/main and https://github.com/example/private and https://github.com/example/private/pull/12 and https://GITHUB.COM/example/private/pull/12"
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
func TestReviewScriptBoundsAndRegularFiles(t *testing.T) {
	for _, kind := range []string{"oversize", "directory"} {
		t.Run(kind, func(t *testing.T) {
			p := t.TempDir()
			want := "regular"
			if kind == "oversize" {
				p = filepath.Join(p, "large.sh")
				file, e := os.Create(p)
				if e != nil {
					t.Fatal(e)
				}
				if e = file.Truncate(9 << 20); e != nil {
					t.Fatal(e)
				}
				file.Close()
				want = "limit"
			}
			r := testReq()
			r.RootRun = &RootRun{Script: p, SHA256: strings.Repeat("0", 64)}
			v := (ArtifactLinkChecker{}).Check(context.Background(), r)
			if len(v) != 1 || !strings.Contains(v[0].Reason, want) {
				t.Fatalf("must reject script by %s: %v", want, v)
			}
		})
	}
}
func TestReviewColdReadInheritedPipeBounded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	p := filepath.Join(t.TempDir(), "fake-claude")
	script := `#!/bin/sh
case " $* " in
 *" auth status "*) printf '%s' '{"loggedIn":true,"authMethod":"claude.ai","subscriptionType":"max"}' ;;
 *) printf '%s' '{"decision":"Choose","options":{},"undefined_terms":[],"verdict":"clear"}'; sleep 2 & ;;
esac
`
	if e := os.WriteFile(p, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	_, e := (ClaudeColdReader{Bin: p}).Read(context.Background(), "example")
	if time.Since(start) > 1500*time.Millisecond || e == nil {
		t.Fatalf("inherited pipe was not bounded: duration=%s err=%v", time.Since(start), e)
	}
}
func TestReviewReleaseUsesPilotSnapshot(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "1")
	f := newFake(t, registryOnCreate).filer()
	for _, key := range []string{"a", "b"} {
		r := testReq()
		r.Key = key
		r.NonBlocking = true
		if _, e := f.FileCard(context.Background(), r); e != nil {
			t.Fatal(e)
		}
	}
	enablePilot(t, f, "daycap")
	dir, _ := f.stateDir()
	once := false
	f.Now = func() time.Time {
		if !once {
			once = true
			os.WriteFile(filepath.Join(dir, "pilots.json"), []byte("{}"), 0600)
		}
		return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	}
	groups, e := f.Release(context.Background())
	if e != nil || len(groups["other"]) != 1 {
		t.Fatalf("release reread mutable pilots mid-sort/run: %v %v", groups, e)
	}
}

// The pre-pilot serialized shape, independent of the current filingEntry additions.
type reviewLegacyEntry struct {
	DeliveredCard    string      `json:"delivered_card,omitempty"`
	DeliveredKey     string      `json:"delivered_key,omitempty"`
	DeliveredProject string      `json:"delivered_project,omitempty"`
	CreationKey      string      `json:"creation_key,omitempty"`
	Uncharged        bool        `json:"uncharged,omitempty"`
	Refunded         bool        `json:"refunded,omitempty"`
	TerminalReason   string      `json:"terminal_reason,omitempty"`
	Request          CardRequest `json:"request"`
	Identity         string      `json:"identity"`
	Project          string      `json:"project"`
	Week             string      `json:"week"`
	Attempted        bool        `json:"attempted,omitempty"`
	Held             bool        `json:"held"`
	Supersedes       string      `json:"supersedes,omitempty"`
	Update           bool        `json:"update"`
}

func TestReviewAbsentPilotsDirectAndReleaseBytes(t *testing.T) {
	for _, mode := range []string{"direct", "release"} {
		t.Run(mode, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			f.Now = func() time.Time { return time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC) }
			r := testReq()
			r.NonBlocking = mode == "release"
			res, e := f.FileCard(context.Background(), r)
			if e != nil {
				t.Fatal(e)
			}
			ident, _ := RequestIdentity(r)
			entry := reviewLegacyEntry{CreationKey: r.Key, Request: r, Identity: ident, Project: "autarch", Week: "2026-W02"}
			if mode == "direct" {
				want := fmt.Sprintf(`{"card":"T1","request":%q,"project":"P1","replay":false}`, r.Key)
				got, _ := json.Marshal(res)
				if string(got) != want {
					t.Fatalf("direct result bytes: %s != %s", got, want)
				}
			} else {
				groups, e := f.Release(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				want := fmt.Sprintf(`{"other":[{"card":"T1","request":%q,"project":"P1","replay":false}]}`, r.Key)
				got, _ := json.Marshal(groups)
				if string(got) != want {
					t.Fatalf("release result bytes: %s != %s", got, want)
				}
				entry.DeliveredCard = "T1"
				entry.Attempted = true
			}
			want, _ := json.Marshal(map[string]reviewLegacyEntry{r.Key: entry})
			d, _ := f.stateDir()
			got, e := os.ReadFile(filepath.Join(d, "filings.json"))
			if e != nil || !bytes.Equal(got, want) {
				t.Fatalf("legacy state bytes changed: %v\nwant %s\ngot %s", e, want, got)
			}
			if _, e := os.Stat(filepath.Join(d, "pilots-metrics.jsonl")); !os.IsNotExist(e) {
				t.Fatal("absent pilots wrote metrics", e)
			}
		})
	}
}
func TestReviewHeldRetrySkipsColdReader(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	r := testReq()
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	enablePilot(t, f, "coldread")
	calls := 0
	f.ColdReader = reviewCold(func(context.Context, string) ([]byte, error) {
		calls++
		return []byte(`{"decision":"Choose","options":{},"undefined_terms":["missing"],"verdict":"unclear"}`), nil
	})
	res, e := f.FileCard(context.Background(), r)
	if e != nil || !res.Queued || !res.Replay || calls != 0 {
		t.Fatalf("held retry invoked reader: %+v %v calls=%d", res, e, calls)
	}
	r.Title = "Different ask"
	if _, e = f.FileCard(context.Background(), r); e == nil || calls != 0 {
		t.Fatal("held replay accepted changed identity", e, calls)
	}
}
func TestReviewUnknownGitHubMetric(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	enablePilot(t, f, "linkcheck")
	r := testReq()
	r.Ask["question"] = "Read https://github.com/example/private/blob/main/notes.md ?"
	var warn string
	f.Warn = func(s string) { warn += s }
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	d, _ := f.stateDir()
	b, e := os.ReadFile(filepath.Join(d, "pilots-metrics.jsonl"))
	if e != nil || !strings.Contains(string(b), `"result":"unknown"`) || strings.Contains(string(b), `"result":"pass"`) || !strings.Contains(warn, "authenticated") {
		t.Fatalf("unknown URL was not reported as unknown: %v %s %s", e, b, warn)
	}
}
func TestReviewRegistryReplayPrecedesLocalRefusal(t *testing.T) {
	for _, mode := range []string{"refunded", "unreadable"} {
		t.Run(mode, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			r := testReq()
			if _, e := f.FileCard(context.Background(), r); e != nil {
				t.Fatal(e)
			}
			enablePilot(t, f, "daycap")
			s, p, done, e := f.openState()
			if e != nil {
				t.Fatal(e)
			}
			v := s[r.Key]
			v.Refunded = true
			s[r.Key] = v
			if e = saveFilingState(p, s); e != nil {
				t.Fatal(e)
			}
			done()
			if mode == "unreadable" {
				if e = os.WriteFile(p, []byte("{broken"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			res, e := f.FileCard(context.Background(), r)
			if e != nil || !res.Replay || res.ID == "" || bb.count("tasks", "create") != 1 {
				t.Fatalf("local state masked existing canonical card: %+v %v", res, e)
			}
		})
	}
}
func TestReviewExplicitPRHeadsInVisibleFormatting(t *testing.T) {
	for _, text := range []string{"example/demo#12 at fedcba0987654321", "`example/demo#12` head `fedcba0987654321`", "[PR](https://github.com/example/demo/pull/12) at fedcba0987654321"} {
		t.Run(text, func(t *testing.T) {
			r := testReq()
			r.Ask["question"] = text
			c := ArtifactLinkChecker{PR: func(context.Context, string) (string, error) { return "abcdef0123456789", nil }}
			v := c.Check(context.Background(), r)
			if len(v) != 1 || !strings.Contains(v[0].Reason, "changed:") {
				t.Fatalf("explicit head was not checked: %v", v)
			}
		})
	}
}
