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

var (
	// The phrase alone triggers the check; the word after it (the head sha) and an existing " into <branch>" are
	// matched only so the annotation can be rewritten.
	mergeOptionRe = regexp.MustCompile(`Manually merge it at(?: (\S+)(?: into \S+)?)?`)
	prURLRe       = regexp.MustCompile(`https://github\.com/([A-Za-z0-9_.-]{1,100})/([A-Za-z0-9_.-]{1,100})/pull/([1-9][0-9]{0,8})\b`)
	prURLExactRe  = regexp.MustCompile(`^` + prURLRe.String() + `$`)
)

// askStrings returns the free text of an ask that can name a PR, and the options as maps.
func askStrings(ask map[string]any) ([]string, []map[string]any) {
	var out []string
	for _, k := range []string{"question", "subject", "recommendation"} {
		if s, ok := ask[k].(string); ok {
			out = append(out, s)
		}
	}
	var opts []map[string]any
	if raw, ok := ask["options"].([]any); ok {
		for _, o := range raw {
			if m, ok := o.(map[string]any); ok {
				opts = append(opts, m)
				for _, v := range m {
					if s, ok := v.(string); ok {
						out = append(out, s)
					}
				}
			}
		}
	}
	return out, opts
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
	texts, opts := askStrings(ask)
	var mergeOpts []map[string]any
	for _, o := range opts {
		if l, ok := o["label"].(string); ok && mergeOptionRe.MatchString(l) {
			mergeOpts = append(mergeOpts, o)
		}
	}
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
	into := " into " + def.DefaultBranchRef.Name
	for _, o := range mergeOpts {
		l := o["label"].(string)
		o["label"] = mergeOptionRe.ReplaceAllStringFunc(l, func(m string) string {
			// Keep the head sha, drop any branch the label already named: only the verified one may stand.
			out := "Manually merge it at"
			if sha := mergeOptionRe.FindStringSubmatch(m)[1]; sha != "" {
				out += " " + sha
			}
			return out + into
		})
	}
	if len(mergeOpts) == 0 {
		q, _ := ask["question"].(string)
		ask["question"] = strings.TrimRight(q, "\n") + "\n\nMerging PR " + pr + " goes" + into + "."
	}
	return nil
}
