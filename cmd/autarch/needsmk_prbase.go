package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ghRun runs gh and returns its stdout; tests replace it.
var ghRun = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "gh", args...).Output()
}

// ghTimeout bounds each gh read the base guard makes.
const ghTimeout = 20 * time.Second

// mergePhrase marks a merge option: "Manually merge it at <head sha>".
const mergePhrase = "Manually merge it at"

var (
	// prURLExactRe is the one accepted spelling of a PR link (a PR tab, a query or a fragment may follow).
	prURLExactRe = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]{1,100})/([A-Za-z0-9_.-]{1,100})/pull/([1-9][0-9]{0,8})$`)
	prLinkRe     = regexp.MustCompile(`^(https://github\.com/([A-Za-z0-9_.-]{1,100})/([A-Za-z0-9_.-]{1,100})/pull/([1-9][0-9]{0,8}))(?:/(?:files|commits|checks|changes))?(?:[?#][A-Za-z0-9_=&.%-]*)?$`)
	// githubTokenRe finds any word that mentions the host in any spelling (scheme, case, port, api. prefix).
	githubTokenRe = regexp.MustCompile(`(?i)[^\s"'<>(\[]*github\.com[^\s"'<>]*`)
	// shortRefRe finds #N, owner/repo#N and "PR N" style references.
	shortRefRe = regexp.MustCompile(`(?i)(?:([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)#([0-9]+)|#([0-9]+)|\b(?:prs?|pull(?: request)?s?)\s+([0-9]+))`)
)

// prRefs are the PR references found in an ask's text.
type prRefs struct {
	urls   map[string][]string // canonical URL -> [url, owner, repo, number]
	shorts [][2]string         // [owner/repo or "", number]
}

// scanPRRefs collects every PR reference in the texts. Any word that mentions github.com must be exactly a plain
// PR link; another spelling (http, other case, a port, api., dot segments, backslashes, commas) could resolve to a
// PR whose base was not checked, so it is an error. #N, owner/repo#N and "PR N" are collected as short references.
func scanPRRefs(texts []string) (prRefs, error) {
	refs := prRefs{urls: map[string][]string{}}
	bad := ""
	for _, t := range texts {
		rest := githubTokenRe.ReplaceAllStringFunc(t, func(tok string) string {
			word := strings.TrimRight(tok, ".,;:)]")
			if m := prLinkRe.FindStringSubmatch(word); m != nil && m[2] != "." && m[2] != ".." && m[3] != "." && m[3] != ".." {
				refs.urls[m[1]] = []string{m[1], m[2], m[3], m[4]}
				return " "
			}
			if bad == "" {
				bad = word
			}
			return " "
		})
		for _, m := range shortRefRe.FindAllStringSubmatch(rest, -1) {
			switch {
			case m[2] != "":
				refs.shorts = append(refs.shorts, [2]string{m[1], m[2]})
			case m[3] != "":
				refs.shorts = append(refs.shorts, [2]string{"", m[3]})
			default:
				refs.shorts = append(refs.shorts, [2]string{"", m[4]})
			}
		}
	}
	if bad != "" {
		return refs, &usageError{"the ask mentions " + bad + ", which is not a plain https://github.com/<owner>/<repo>/pull/<n> link; a merge ask may link only the PR it merges, spelled that way; nothing was filed"}
	}
	return refs, nil
}

// walkAsk visits every string in the ask at any depth (options sit at the top level and inside questions), and
// each (map, key) whose key is "label" in any letter case with a string value: the ask parser reads keys without
// regard to case, so detection must too.
func walkAsk(v any, str func(string), option func(m map[string]any, key string)) {
	switch x := v.(type) {
	case string:
		str(x)
	case []any:
		for _, e := range x {
			walkAsk(e, str, option)
		}
	case map[string]any:
		for k, e := range x {
			if _, ok := e.(string); ok && strings.EqualFold(k, "label") {
				option(x, k)
			}
			walkAsk(e, str, option)
		}
	}
}

// withBranch rewrites a merge label so it names the verified branch: the head sha stays, a branch the label
// already named is dropped, and " into <branch>" follows the sha.
func withBranch(label, branch string) string {
	i := strings.Index(label, mergePhrase)
	out := label[:i+len(mergePhrase)]
	f := strings.Fields(label[i+len(mergePhrase):])
	if len(f) > 0 && f[0] != "into" {
		out += " " + f[0]
		f = f[1:]
	}
	if len(f) > 0 && f[0] == "into" {
		f = f[min(2, len(f)):]
	}
	out += " into " + branch
	if len(f) > 0 {
		out += " " + strings.Join(f, " ")
	}
	return out
}

func ghJSON(ctx context.Context, into any, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	out, err := ghRun(ctx, args...)
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			msg += ": " + strings.TrimSpace(string(ee.Stderr))
		}
		return fmt.Errorf("gh %s: %s", strings.Join(args[:2], " "), msg)
	}
	if err := json.Unmarshal(out, into); err != nil {
		return fmt.Errorf("gh %s: output is not the expected JSON: %v", strings.Join(args[:2], " "), err)
	}
	return nil
}

// guardMergeBase refuses (exit 2) a PR merge ask unless the PR's base branch is the repo's default branch, so a
// stacked PR cannot be merged into its parent's branch from a card. A PR merge ask is one with an option
// "Manually merge it at <sha>" or an explicit --pr. On success the ask names the branch: "into <branch>".
// Asks that are not PR merges are untouched and make no gh call. The PR references checked are those in the whole
// card: the ask, the --move file and the text flags (extra).
func guardMergeBase(ctx context.Context, ask, move map[string]any, extra []string, prFlag string, prSet bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var texts []string
	type labelRef struct {
		m   map[string]any
		key string
	}
	texts = append(texts, extra...)
	walkAsk(move, func(t string) { texts = append(texts, t) }, func(map[string]any, string) {})
	var mergeOpts []labelRef
	walkAsk(ask, func(t string) { texts = append(texts, t) }, func(m map[string]any, key string) {
		if strings.Contains(m[key].(string), mergePhrase+" ") {
			mergeOpts = append(mergeOpts, labelRef{m, key})
		}
	})
	if len(mergeOpts) == 0 && !prSet {
		return nil
	}
	refs, err := scanPRRefs(texts)
	if err != nil {
		return err
	}
	urls := make([]string, 0, len(refs.urls))
	for u := range refs.urls {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	var m []string
	if prSet {
		if m = prURLExactRe.FindStringSubmatch(prFlag); m == nil {
			return &usageError{"--pr must be https://github.com/<owner>/<repo>/pull/<n>"}
		}
		for _, u := range urls {
			if u != m[0] {
				return &usageError{"--pr " + m[0] + " conflicts with the ask text, which names " + u + ": the card must merge the PR whose base was checked; nothing was filed"}
			}
		}
	} else {
		switch len(urls) {
		case 0:
			return &usageError{"a merge ask (\"Manually merge it at ...\") must name its PR so the base branch can be checked: put the PR URL in the question or pass --pr; nothing was filed"}
		case 1:
			m = refs.urls[urls[0]]
		default:
			return &usageError{"the merge ask names more than one PR (" + strings.Join(urls, ", ") + "): pass --pr for the one it merges; nothing was filed"}
		}
	}
	for _, sr := range refs.shorts {
		if (sr[0] != "" && !strings.EqualFold(sr[0], m[1]+"/"+m[2])) || sr[1] != m[3] {
			ref := "#" + sr[1]
			if sr[0] != "" {
				ref = sr[0] + ref
			}
			return &usageError{"the ask mentions " + ref + " but the PR whose base is checked is " + m[1] + "/" + m[2] + "#" + m[3] + ": a merge ask may name only that PR; nothing was filed"}
		}
	}
	repo, num := m[1]+"/"+m[2], m[3]
	pr := repo + "#" + num
	var base struct {
		BaseRefName string `json:"baseRefName"`
	}
	if err := ghJSON(ctx, &base, "pr", "view", num, "--repo", repo, "--json", "baseRefName"); err != nil {
		return &usageError{"cannot check the base of PR " + pr + ": " + err.Error() + "; nothing was filed"}
	}
	var def struct {
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if err := ghJSON(ctx, &def, "repo", "view", repo, "--json", "defaultBranchRef"); err != nil {
		return &usageError{"cannot read the default branch of " + repo + ": " + err.Error() + "; nothing was filed"}
	}
	if base.BaseRefName == "" || def.DefaultBranchRef.Name == "" {
		return &usageError{"gh returned no base or default branch for PR " + pr + "; nothing was filed"}
	}
	if base.BaseRefName != def.DefaultBranchRef.Name {
		return &usageError{fmt.Sprintf("PR %s is based on %q, not the default branch %q: merging it from a card would land it on %s, not %s. Retarget the PR to %s (or merge its parent first) and file again; nothing was filed",
			pr, base.BaseRefName, def.DefaultBranchRef.Name, base.BaseRefName, def.DefaultBranchRef.Name, def.DefaultBranchRef.Name)}
	}
	for _, o := range mergeOpts {
		o.m[o.key] = withBranch(o.m[o.key].(string), def.DefaultBranchRef.Name)
	}
	if len(mergeOpts) == 0 {
		q, _ := ask["question"].(string)
		ask["question"] = strings.TrimRight(q, "\n") + "\n\nMerging PR " + pr + " goes into " + def.DefaultBranchRef.Name + "."
	}
	return nil
}
