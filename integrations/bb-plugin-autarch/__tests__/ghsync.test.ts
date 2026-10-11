import { describe, expect, it } from "vitest";
import { classify, cleanTitle, ghPrListArgv, ghRepoListArgv, mkOnlyPaths, parsePrList, renderDryRun, type GhPrRow } from "../ghsync.js";

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
    expect(text).not.toMatch(/\u001b/);
  });
  it("sends a capped file list to mk", () => {
    const files = Array.from({ length: 100 }, (_, i) => `f${i}.go`);
    expect(classify(pr({ files })).waitingOn).toBe("mk");
  });
  it("reports counts by who is waited on, drafts apart, and unmapped repos", () => {
    const rows = [classify(pr()), classify(pr({ isDraft: true })), classify(pr({ files: ["AGENTS.md"] })), classify(pr({ mergeable: "CONFLICTING" }))];
    const text = renderDryRun(rows, ["x/y"]);
    expect(text.split("\n")[0]).toBe("open PRs: 4; would get a card: 3 (waiting on mk 1, vizier 1, coordinator 1); drafts, no card: 1");
    expect(text).toContain("waiting on mk");
    expect(text).toContain("repos with no Tasks project (no card): x/y");
  });
});
