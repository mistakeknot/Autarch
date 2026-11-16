package homeask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type LinkFailure struct {
	Item   string `json:"item"`
	Reason string `json:"reason"`
}
type LinkChecker interface {
	Check(context.Context, CardRequest) []LinkFailure
}
type ArtifactLinkChecker struct {
	Client *http.Client
	PR     func(context.Context, string) (string, error)
}

var pilotURL = regexp.MustCompile("https://[^\\s<>\"'`]+")
var pilotPR = regexp.MustCompile(`(?:https://github\.com/([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)/pull/([0-9]+)|([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)#([0-9]+))`)
var pilotSHA = regexp.MustCompile(`\b[0-9a-fA-F]{8,64}\b`)
var scriptLine = regexp.MustCompile(`(?m)(?:bash[ \t]+|script:[ \t]*)(/[^\s"` + "`" + `]+)`)
var fullSHA = regexp.MustCompile(`\b[0-9a-fA-F]{64}\b`)

func cardStrings(r CardRequest) []string {
	out := []string{r.Title}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			out = append(out, x)
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(x[k])
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(r.Ask)
	walk(r.Move)
	out = append(out, r.Attachments...)
	return out
}
func defaultPRHead(ctx context.Context, ref string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	repo, num, _ := strings.Cut(ref, "#")
	b, e := exec.CommandContext(ctx, "gh", "pr", "view", num, "--repo", repo, "--json", "headRefOid").Output()
	if e != nil {
		return "", fmt.Errorf("gh pr view failed: %w", e)
	}
	var v struct {
		Head string `json:"headRefOid"`
	}
	if e = json.Unmarshal(b, &v); e != nil || v.Head == "" {
		return "", fmt.Errorf("unreadable PR head")
	}
	return v.Head, nil
}
func (c ArtifactLinkChecker) Check(ctx context.Context, r CardRequest) []LinkFailure {
	var failures []LinkFailure
	urls := map[string]bool{}
	prs := map[string][]string{}
	scripts := map[string]string{}
	for _, s := range cardStrings(r) {
		for _, u := range pilotURL.FindAllString(s, -1) {
			urls[strings.TrimRight(u, ".,;:!?)]}")] = true
		}
		for _, m := range pilotPR.FindAllStringSubmatchIndex(s, -1) {
			v := func(i int) string {
				if m[i] < 0 {
					return ""
				}
				return s[m[i]:m[i+1]]
			}
			ref := v(2) + "#" + v(4)
			if v(6) != "" {
				ref = v(6) + "#" + v(8)
			}
			start := strings.LastIndex(s[:m[0]], "\n") + 1
			end := strings.Index(s[m[1]:], "\n")
			if end < 0 {
				end = len(s)
			} else {
				end += m[1]
			}
			near := s[start:m[0]] + " " + s[m[1]:end]
			prs[ref] = append(prs[ref], pilotSHA.FindAllString(near, -1)...)
		}
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			m := scriptLine.FindStringSubmatch(l)
			if len(m) == 0 {
				continue
			}
			lo, hi := max(0, i-1), min(len(lines), i+2)
			hashes := fullSHA.FindAllString(strings.Join(lines[lo:hi], "\n"), -1)
			if len(hashes) == 1 {
				scripts[m[1]] = hashes[0]
			}
		}
	}
	var walkScripts func(any)
	walkScripts = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if path, ok := x["path"].(string); ok {
				if hash, ok := x["sha256"].(string); ok && filepath.IsAbs(path) && fullSHA.MatchString(hash) {
					scripts[path] = hash
				}
			}
			for _, v := range x {
				walkScripts(v)
			}
		case []any:
			for _, v := range x {
				walkScripts(v)
			}
		}
	}
	walkScripts(r.Move)
	if r.RootRun != nil {
		scripts[r.RootRun.Script] = r.RootRun.SHA256
	}
	client := http.Client{Timeout: 10 * time.Second}
	if c.Client != nil {
		client = *c.Client
		client.Timeout = 10 * time.Second
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	for u := range urls {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if e == nil {
			var resp *http.Response
			resp, e = client.Do(req)
			if e == nil {
				resp.Body.Close()
				code := resp.StatusCode
				if !((code >= 200 && code < 400) || code == 401 || code == 403) {
					e = fmt.Errorf("HTTP %d", code)
				}
			}
		}
		if e != nil {
			failures = append(failures, LinkFailure{u, e.Error()})
		}
	}
	pr := c.PR
	if pr == nil {
		pr = defaultPRHead
	}
	for ref, hashes := range prs {
		head, e := pr(ctx, ref)
		if e == nil {
			for _, h := range hashes {
				if !strings.HasPrefix(strings.ToLower(head), strings.ToLower(h)) {
					e = fmt.Errorf("changed: card head %s differs from current head %s", h, head)
					break
				}
			}
		}
		if e != nil {
			failures = append(failures, LinkFailure{ref, e.Error()})
		}
	}
	for p, want := range scripts {
		b, e := os.ReadFile(p)
		if e == nil {
			h := sha256.Sum256(b)
			if !strings.EqualFold(hex.EncodeToString(h[:]), want) {
				e = fmt.Errorf("sha256 changed")
			}
		}
		if e != nil {
			failures = append(failures, LinkFailure{p, e.Error()})
		}
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].Item < failures[j].Item })
	return failures
}
func failureText(failures []LinkFailure) string {
	out := make([]string, 0, len(failures))
	for _, v := range failures {
		out = append(out, v.Item+": "+v.Reason)
	}
	return strings.Join(out, "; ")
}
func (f *CardFiler) checkLinks(ctx context.Context, r CardRequest) []LinkFailure {
	c := f.Links
	if c == nil {
		c = ArtifactLinkChecker{}
	}
	v := c.Check(ctx, r)
	result := "pass"
	if len(v) > 0 {
		result = "fail"
	}
	f.pilotEvent(r, "link-check", result, failureText(v))
	return v
}

// Notification identity excludes unstable OS/network error text; URL and status remain visible.
func failureSet(v []LinkFailure) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (f *CardFiler) wakeLinkOwner(ctx context.Context, key string, r CardRequest, v []LinkFailure) error {
	s, p, done, e := f.openState()
	if e != nil {
		return e
	}
	entry := s[key]
	set := failureSet(v)
	if entry.LinkFailureSet == set {
		done()
		return nil
	}
	owner := r.routeThread()
	if owner == "" {
		done()
		return nil
	}
	entry.LinkFailureSet = set
	s[key] = entry
	e = saveFilingState(p, s)
	done()
	if e == nil && entry.Held {
		f.pilotEvent(r, "held", "", "link-check: "+failureText(v))
	}
	if e != nil {
		return e
	}
	res := f.exec(ctx, cardEnv(""), "thread", "tell", owner, "--message", "needs-mk link check failed for request "+key+": "+failureText(v))
	if res.failed() {
		return fmt.Errorf("owner wake outcome unknown: %s", res.text())
	}
	return nil
}

// RecheckLinks deliberately leaves delivered cards visible; holding requires a different authority.
// Dry-run does not open locks, write metrics/state, or wake threads, even with pilots enabled.
func (f *CardFiler) RecheckLinks(ctx context.Context, w io.Writer, dry bool) error {
	d, e := f.stateDir()
	if e != nil {
		return e
	}
	s := filingState{}
	b, e := os.ReadFile(filepath.Join(d, "filings.json"))
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if e == nil {
		if e = json.Unmarshal(b, &s); e != nil {
			return e
		}
	}
	if !dry && !f.pilot("linkcheck") {
		fmt.Fprintln(w, "linkcheck pilot is off; no checks or wakes")
		return nil
	}
	// Home is the authority for open delivered cards, including cards predating this state log.
	var rows []ListRow
	if e = f.callEnv(ctx, unscopedEnv(""), &rows, "home", "list", "--json"); e != nil {
		return e
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TaskID < rows[j].TaskID })
	seen := map[string]bool{}
	for _, row := range rows {
		id := row.TaskID
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		view, e := f.Card(ctx, id)
		if e != nil {
			fmt.Fprintf(w, "%s read error: %v\n", id, e)
			continue
		}
		if view.State != "open" || view.DecisionState == "picked" || view.DecisionState == "ruled" {
			continue
		}
		shown, e := f.show(ctx, id)
		if e != nil {
			fmt.Fprintf(w, "%s read error: %v\n", id, e)
			continue
		}
		if shown.Task.Status == "done" || shown.Task.Status == "canceled" {
			continue
		}
		card, e := ParseCard(shown.Task.Description)
		if e != nil {
			fmt.Fprintf(w, "%s parse error: %v\n", view.CardKey, e)
			continue
		}
		key := card.Request.Key
		entry, known := s[key]
		r := entry.Request
		if !known {
			r = CardRequest{Key: key, Project: shown.Task.ProjectID, Thread: view.AskingThread}
			if r.Thread == "" {
				r.Thread = row.Thread
			}
		}
		if view.AskingThread != "" {
			r.Owner = view.AskingThread
		}
		r.Ask = card.Ask
		r.Title = shown.Task.Title
		r.RootRun = card.RootRun
		r.Move = nil
		if card.Move != nil {
			r.Move = card.Move.Raw
		}
		c := f.Links
		if c == nil {
			c = ArtifactLinkChecker{}
		}
		v := c.Check(ctx, r)
		result := "pass"
		if len(v) > 0 {
			result = "fail"
		}
		fmt.Fprintf(w, "%s %s %s %s\n", view.CardKey, key, result, failureText(v))
		if !dry {
			if !known {
				current, p, done, e := f.openState()
				if e != nil {
					return e
				}
				if _, ok := current[key]; !ok {
					ident, _ := RequestIdentity(r)
					current[key] = filingEntry{Request: r, Identity: ident, Project: strings.ToLower(row.Project), Uncharged: true, DeliveredCard: id, DeliveredCardKey: view.CardKey, DeliveredAt: row.FiledAt}
					e = saveFilingState(p, current)
				}
				done()
				if e != nil {
					return e
				}
			}
			f.pilotEvent(r, "link-check", result, failureText(v))
			if len(v) > 0 {
				if e = f.wakeLinkOwner(ctx, key, r, v); e != nil {
					return e
				}
			}
		}
	}
	fmt.Fprintln(w, "Delivered cards remain visible; failures wake owners only. Dry-run writes nothing and wakes nobody.")
	return nil
}
