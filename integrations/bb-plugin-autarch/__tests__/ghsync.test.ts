import { describe, expect, it } from "vitest";
import { REPO_PROJECTS, keyFamily, legacyPrKey, prCardAction, planCards, renderPlan, classify, cleanTitle, ghPrListArgv, ghRepoListArgv, mkOnlyPaths, parsePrList, renderDryRun, type GhPrRow } from "../ghsync.js";

const pr = (o: Partial<GhPrRow> | Readonly<Record<string, unknown>> = {}): GhPrRow => ({
  repo: "mistakeknot/Autarch", number: 7, url: "https://github.com/mistakeknot/Autarch/pull/7", title: "t", isDraft: false,
  authorLogin: "mistakeknot", authorIsBot: false, reviewRequests: [], reviewDecision: "", mergeable: "MERGEABLE",
  checks: ["SUCCESS"], files: ["x.go"], updatedAt: "2026-10-11T00:00:00Z", ...(o as Partial<GhPrRow>),
});

describe("classify: who a PR waits on", () => {
  it.each([
    [{ isDraft: true }, null],
    [{ files: ["AGENTS.md"] }, "mk"],
    [{ files: ["sub/CLAUDE.md"] }, "mk"],
    [{ files: [".github/workflows/ci.yml"] }, "mk"],
    [{ files: ["common/.claude/hooks/a.sh"] }, "mk"],
    [{ files: ["scripts/install-home.sh"] }, "mk"],
    [{ files: [".github/copilot-instructions.md"] }, "mk"],
    [{ files: [".agents/skills/foo/SKILL.md"] }, "mk"],
    [{ files: ["scripts/release.bash"] }, "mk"],
    [{ files: ["scripts/install"] }, "mk"],
    [{ files: ["scripts/publish"] }, "mk"],
    [{ files: [] }, "mk"],
    [{ mergeable: "CONFLICTING" }, "coordinator"],
    [{ reviewDecision: "CHANGES_REQUESTED" }, "coordinator"],
    [{ checks: ["SUCCESS", "FAILURE"] }, "coordinator"],
    [{ checks: [] }, "coordinator"],
    [{ checks: ["SUCCESS", "IN_PROGRESS"] }, "coordinator"],
    [{ mergeable: "UNKNOWN" }, "coordinator"],
    [{}, "vizier"],
  ] as const)("%j -> %s", (o, want) => {
    expect(classify(pr(o)).waitingOn).toBe(want);
  });
  it("a protected path wins over a conflict", () => {
    expect(classify(pr({ files: ["AGENTS.md"], mergeable: "CONFLICTING" })).waitingOn).toBe("mk");
  });
  it("mkOnlyPaths lists only protected paths", () => {
    expect(mkOnlyPaths(["a.go", "AGENTS.md", "docs/x.md"])).toEqual(["AGENTS.md"]);
  });
});

describe("cleanTitle", () => {
  it("strips control characters and newlines, caps length, never returns empty", () => {
    expect(cleanTitle("a\nb\u0007c d")).toBe("a b c d");
    expect(cleanTitle("x".repeat(300)).length).toBe(120);
    expect(cleanTitle("\n\t ")).toBe("(no title)");
  });
});

describe("parsePrList", () => {
  const row = { number: 3, url: "https://github.com/a/b/pull/3", title: "T", isDraft: false, author: { login: "u", is_bot: true }, reviewRequests: [{ login: "mistakeknot" }], reviewDecision: "APPROVED", mergeable: "MERGEABLE", statusCheckRollup: [{ conclusion: "SUCCESS" }, { state: "PENDING" }], files: [{ path: "x" }], updatedAt: "z" };
  it("maps a row", () => {
    const [r] = parsePrList("a/b", JSON.stringify([row]));
    expect(r).toMatchObject({ repo: "a/b", number: 3, authorIsBot: true, reviewRequests: ["mistakeknot"], checks: ["SUCCESS", "PENDING"], files: ["x"] });
  });
  it.each([["not json"], ["{}"], [JSON.stringify([{ ...row, url: "https://evil.example/a/b/pull/3" }])], [JSON.stringify([{ ...row, number: "3" }])]])("rejects %s", (j) => {
    expect(() => parsePrList("a/b", j)).toThrow();
  });
  it("rejects a row with no files array, so unknown files never classify as clean", () => {
    const { files: _f, ...nofiles } = row;
    expect(() => parsePrList("a/b", JSON.stringify([nofiles]))).toThrow();
  });
  it("rejects a file entry with no path", () => {
    expect(() => parsePrList("a/b", JSON.stringify([{ ...row, files: [{}] }]))).toThrow();
  });
  it("rejects a bad repo name", () => {
    expect(() => parsePrList("a b/c", "[]")).toThrow();
  });
});

describe("the gh argv allowlist", () => {
  it("is read verbs only, exact commands", () => {
    expect(ghRepoListArgv("mistakeknot").slice(0, 2)).toEqual(["repo", "list"]);
    expect(ghPrListArgv("a/b").slice(0, 2)).toEqual(["pr", "list"]);
    for (const argv of [ghRepoListArgv("o"), ghPrListArgv("a/b")]) expect(argv.join(" ")).not.toMatch(/\b(merge|close|comment|edit|create|review|api|delete|reopen|ready)\b/);
  });
});

describe("renderDryRun", () => {
  it("cleans control characters out of file paths in the reason", () => {
    const text = renderDryRun([classify(pr({ files: ["AGENTS.md\u001b[31m"] }))]);
    // eslint-disable-next-line no-control-regex
    expect(text).not.toMatch(/\u001b/);
  });
  it("sends a capped file list to mk", () => {
    const files = Array.from({ length: 100 }, (_, i) => `f${i}.go`);
    expect(classify(pr({ files })).waitingOn).toBe("mk");
  });
  it("reports counts by who is waited on, drafts apart, and unmapped repos", () => {
    const u = (n: number) => ({ number: n, url: `https://github.com/mistakeknot/Autarch/pull/${n}` });
    const rows = [classify(pr(u(1))), classify(pr({ ...u(2), isDraft: true })), classify(pr({ ...u(3), files: ["AGENTS.md"] })), classify(pr({ ...u(4), mergeable: "CONFLICTING" }))];
    const text = renderDryRun(rows, ["x/y"]);
    expect(text.split("\n")[0]).toBe("open PRs: 4; would get a card: 3 (waiting on mk 1, vizier 1, coordinator 1); drafts, no card: 1");
    expect(text).toContain("waiting on mk");
    expect(text).toContain("repos with no Tasks project (no card): x/y");
  });
});

describe("planCards", () => {
  it("renderDryRun counts a PR read twice once", () => {
    const row = classify(pr());
    expect(renderDryRun([row, row]).split("\n")[0]).toMatch(/^open PRs: 1;/);
  });
  it("lists a PR read twice once", () => {
    const row = classify(pr({ repo: "mistakeknot/dotfiles", number: 3, url: "https://github.com/mistakeknot/dotfiles/pull/3", files: ["AGENTS.md"] }));
    const plan = planCards([row, row]);
    expect(plan.cards.filter((c) => c.urls.includes(row.pr.url))).toHaveLength(1);
  });
  const bot = (n: number, repo = "mistakeknot/Autarch") => classify(pr({ repo, number: n, url: `https://github.com/${repo}/pull/${n}`, authorIsBot: true, authorLogin: "app/dependabot", files: [".github/workflows/ci.yml"] }));
  it("batches a repo's bot CI-only PRs into one card and gives other mk PRs one card each", () => {
    const rows = [bot(1), bot(2), bot(3, "mistakeknot/Clavain"), classify(pr({ number: 9, url: "https://github.com/mistakeknot/Autarch/pull/9", files: ["AGENTS.md"] }))];
    const plan = planCards(rows);
    expect(plan.cards.map((c) => [c.kind, c.urls.length])).toEqual([["pr", 1], ["batch", 2], ["batch", 1]]);
    const single = plan.cards.find((c) => c.kind === "pr")!;
    expect(single.key).toBe("gh-ask:mistakeknot:Autarch:9");
    expect(legacyPrKey(single.key)).toBe("gh-pr:mistakeknot:Autarch:9");
    expect((single.ask.options as { id: string }[]).map((o) => o.id)).toEqual(["merge", "close", "leave"]);
    expect(single.ask.question).toContain("https://github.com/mistakeknot/Autarch/pull/9");
    expect(single.ask.question).toMatch(/^https:\/\/github\.com\/mistakeknot\/Autarch\/pull\/9\n/);
    expect(single.title).toContain("https://github.com/mistakeknot/Autarch/pull/9");
    expect(single.move).toBeNull();
    expect((plan.cards.find((c) => c.urls.length === 2)!.ask.options as { id: string }[]).map((o) => o.id)).toEqual(["merge-all", "close-all", "leave"]);
  });
  it("makes no card for vizier, coordinator or draft PRs, and one MSQ catch-all card per unmapped repo", () => {
    const plan = planCards([classify(pr()), classify(pr({ mergeable: "CONFLICTING" })), classify(pr({ isDraft: true })), bot(5, "mistakeknot/unlisted-repo"), bot(6, "mistakeknot/unlisted-repo")]);
    expect(plan.cards).toHaveLength(1);
    expect(plan.cards[0]).toMatchObject({ prefix: "MSQ", kind: "unmapped", move: null });
    expect(plan.cards[0]!.key).toMatch(/^gh-unmapped:mistakeknot:unlisted-repo:[0-9a-f]{8}$/);
    expect(plan.cards[0]!.urls).toHaveLength(2);
    expect(plan.unmapped).toEqual([{ repo: "mistakeknot/unlisted-repo", urls: ["https://github.com/mistakeknot/unlisted-repo/pull/5", "https://github.com/mistakeknot/unlisted-repo/pull/6"] }]);
  });
  it("stay within the filer's 128 characters for a 100-character repo name, and share a family across sets", () => {
    const long = "x".repeat(100);
    const plan = planCards([bot(1, `mistakeknot/${long}`), bot(2, `mistakeknot/${long}`)]);
    expect(plan.cards[0]!.key.length).toBeLessThanOrEqual(128);
    const a = planCards([bot(1, "mistakeknot/unlisted-repo")]).cards[0]!.key;
    const b = planCards([bot(1, "mistakeknot/unlisted-repo"), bot(2, "mistakeknot/unlisted-repo")]).cards[0]!.key;
    expect(a).not.toBe(b);
    expect(keyFamily(a)).toBe(keyFamily(b));
    const l1 = planCards([bot(1, `mistakeknot/${"y".repeat(60)}a`)]).cards[0]!.key;
    const l2 = planCards([bot(1, `mistakeknot/${"y".repeat(60)}b`)]).cards[0]!.key;
    expect(keyFamily(l1)).not.toBe(keyFamily(l2));
  });
  it("plans no move for any card: what mk owes a PR is its decision (bead mk-1k8pg)", () => {
    const rows = [bot(1), bot(2), classify(pr({ number: 9, url: "https://github.com/mistakeknot/Autarch/pull/9", files: ["AGENTS.md"] })), bot(5, "mistakeknot/unlisted-repo")];
    expect(planCards(rows).cards.map((c) => c.move)).toEqual([null, null, null]);
  });
  it("sends a prefix Home cannot file under to the MSQ catch-all, with its PRs listed", () => {
    const rows = [bot(1, "mistakeknot/ushas"), classify(pr({ repo: "mistakeknot/ushas", number: 3, url: "https://github.com/mistakeknot/ushas/pull/3", files: ["AGENTS.md"] }))];
    const plan = planCards(rows, new Set(["AUTA", "MSQ"]));
    expect(plan.cards).toHaveLength(1);
    expect(plan.cards[0]).toMatchObject({ prefix: "MSQ", kind: "unmapped" });
    expect(plan.cards[0]!.urls).toHaveLength(2);
    expect(planCards(rows).cards.map((c) => c.prefix)).toEqual(expect.arrayContaining(["USH"]));
  });
  it("keys are filer-safe, stable, and the batch key follows the set of URLs", () => {
    const a = planCards([bot(1), bot(2)]).cards[0]!;
    expect(planCards([bot(2), bot(1)]).cards[0]!.key).toBe(a.key);
    expect(planCards([bot(1), bot(2), bot(3)]).cards[0]!.key).not.toBe(a.key);
    expect(a.key).toMatch(/^[A-Za-z0-9:_.-]{1,128}$/);
  });
  it("every mapped prefix is a prefix, and the plan renders", () => {
    const plan = planCards([classify(pr({ number: 9, url: "https://github.com/mistakeknot/Autarch/pull/9", title: "x\ny\u0007", files: ["AGENTS.md"] }))]);
    for (const p of Object.values(REPO_PROJECTS)) expect(p).toMatch(/^[A-Z]{1,6}$/);
    expect(renderPlan(plan)).toContain("cards to file: 1");
  });
});

describe("planCards batch size", () => {
  it("splits a big batch so every question fits the ask limit", () => {
    const rows = Array.from({ length: 60 }, (_, i) =>
      classify({ repo: "mistakeknot/Autarch", number: 100 + i, title: "bump", url: `https://github.com/mistakeknot/Autarch/pull/${100 + i}`, isDraft: false, authorIsBot: true, files: [".github/workflows/ci.yml"], mergeable: "MERGEABLE", reviewDecision: "", checks: "pass" } as never),
    );
    const plan = planCards(rows);
    const batches = plan.cards.filter((c) => c.kind === "batch");
    expect(batches.length).toBeGreaterThan(1);
    expect(batches.reduce((n, c) => n + c.urls.length, 0)).toBe(60);
    for (const b of batches) expect(String((b.ask as { question: string }).question).length).toBeLessThanOrEqual(2000);
  });
});

describe("sync script helpers", () => {
  it("pageDescriptions fails closed on a row without a description", async () => {
    const { pageDescriptions } = await import("../ghsync.js");
    expect(pageDescriptions({ tasks: [{ description: "a" }] })).toEqual(["a"]);
    expect(() => pageDescriptions({ tasks: [{ description: "a" }, {}] })).toThrow(/no description/);
  });
  it("nextCursor rejects a repeated cursor", async () => {
    const { nextCursor } = await import("../ghsync.js");
    const seen = new Set<string>();
    expect(nextCursor(seen, "a")).toBe("a");
    expect(nextCursor(seen, null)).toBeNull();
    expect(() => nextCursor(seen, "a")).toThrow(/repeated/);
    expect(() => nextCursor(seen, undefined)).toThrow(/no nextCursor/);
  });
  it("parseSyncArgs rejects unknown flags", async () => {
    const { parseSyncArgs } = await import("../ghsync.js");
    expect(parseSyncArgs(["--apply", "--roots", "r.json"])).toEqual({ apply: true, roots: "r.json" });
    expect(() => parseSyncArgs(["--help"])).toThrow(/unknown argument/);
    expect(() => parseSyncArgs(["--roots"])).toThrow(/unknown argument/);
  });
  it("requestKey reads the exact Request line", async () => {
    const { requestKey } = await import("../ghsync.js");
    expect(requestKey("x\nRequest: gh-pr:a:b:1 sha256:abc\n")).toBe("gh-pr:a:b:1");
    expect(requestKey("mentions Request: gh-pr:a:b:1 sha256:abc inline")).toBeNull();
  });
  it("mentionsPr matches whole PR URLs only", async () => {
    const { mentionsPr } = await import("../ghsync.js");
    const batch = "Look at:\nhttps://github.com/o/r/pull/10\nhttps://github.com/o/r/pull/11";
    expect(mentionsPr(batch, "https://github.com/o/r/pull/10")).toBe(true);
    expect(mentionsPr(batch, "https://github.com/o/r/pull/1")).toBe(false);
  });
});

describe("prCardAction", () => {
  const url = "https://github.com/mistakeknot/Autarch/pull/9";
  const card = (key: string, extra = "") => `Request: ${key} sha256:abcd\n${extra}`;
  it("treats an open card under the same key as filed, whatever its wording", () => {
    expect(prCardAction([card("gh-ask:mistakeknot:Autarch:9", "old wording")], "gh-ask:mistakeknot:Autarch:9", url)).toBe("filed");
  });
  it("holds on the old gh-pr key and on a batch that lists the PR, files otherwise", () => {
    expect(prCardAction([card("gh-pr:mistakeknot:Autarch:9")], "gh-ask:mistakeknot:Autarch:9", url)).toBe("legacy");
    expect(prCardAction([card("gh-bots:mistakeknot:Autarch:aaaaaaaa", `- ${url}`)], "gh-ask:mistakeknot:Autarch:9", url)).toBe("in-set");
    expect(prCardAction([card("gh-ask:mistakeknot:Autarch:10")], "gh-ask:mistakeknot:Autarch:9", url)).toBe("file");
  });
});
