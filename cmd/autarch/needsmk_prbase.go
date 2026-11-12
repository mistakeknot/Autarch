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
	prURLRe      = regexp.MustCompile(`https://github\.com/([A-Za-z0-9_.-]{1,100})/([A-Za-z0-9_.-]{1,100})/pull/([1-9][0-9]{0,8})\b`)
	prURLExactRe = regexp.MustCompile(`^` + prURLRe.String() + `$`)
)

// walkAsk visits every string in the ask at any depth (options sit at the top level and inside questions), and
// every map that has a string "label" (an option).
func walkAsk(v any, str func(string), option func(map[string]any)) {
	switch x := v.(type) {
	case string:
		str(x)
	case []any:
		for _, e := range x {
			walkAsk(e, str, option)
		}
	case map[string]any:
		if _, ok := x["label"].(string); ok {
			option(x)
		}
		for _, e := range x {
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
// Asks that are not PR merges are untouched and make no gh call.
func guardMergeBase(ctx context.Context, ask map[string]any, prFlag string, prSet bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var texts []string
	var mergeOpts []map[string]any
	walkAsk(ask, func(t string) { texts = append(texts, t) }, func(o map[string]any) {
		if strings.Contains(o["label"].(string), mergePhrase+" ") {
			mergeOpts = append(mergeOpts, o)
		}
	})
	if len(mergeOpts) == 0 && !prSet {
		return nil
	}
	var m []string
	if prSet {
		if m = prURLExactRe.FindStringSubmatch(prFlag); m == nil {
			return &usageError{"--pr must be https://github.com/<owner>/<repo>/pull/<n>"}
		}
	} else {
		seen := map[string][]string{}
		for _, t := range texts {
			for _, f := range prURLRe.FindAllStringSubmatch(t, -1) {
				seen[f[0]] = f
			}
		}
		urls := make([]string, 0, len(seen))
		for u := range seen {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		switch len(urls) {
		case 0:
			return &usageError{"a merge ask (\"Manually merge it at ...\") must name its PR so the base branch can be checked: put the PR URL in the question or pass --pr; nothing was filed"}
		case 1:
			m = seen[urls[0]]
		default:
			return &usageError{"the merge ask names more than one PR (" + strings.Join(urls, ", ") + "): pass --pr for the one it merges; nothing was filed"}
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
		o["label"] = withBranch(o["label"].(string), def.DefaultBranchRef.Name)
	}
	if len(mergeOpts) == 0 {
		q, _ := ask["question"].(string)
		ask["question"] = strings.TrimRight(q, "\n") + "\n\nMerging PR " + pr + " goes into " + def.DefaultBranchRef.Name + "."
	}
	return nil
}
