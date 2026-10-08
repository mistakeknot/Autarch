import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  type Ask,
  canonicalJson,
  identity,
  normalizeAsk,
  normalizedJson,
  parseAsk,
  revision,
  semanticKey,
} from "../model";

const here = dirname(fileURLToPath(import.meta.url));
const schemaDir = join(here, "..", "..", "..", "schema", "home-ask", "v1");

function base(): Ask {
  return {
    v: 1,
    kind: "decide",
    subject: "autarch/catch-up: collapse order",
    project: "Autarch",
    project_root: "/home/dev/projects/Autarch",
    asker: "thread",
    thread: "thr_abc123",
    question: "Collapse routine catch-up items per project or per day?",
    recommendation: "project",
    options: [
      { id: "project", label: "Collapse per project", kind: "instruction", reversible: true, instruction: "On branch feat/bb-catchup, group per project, run npm test, commit locally, and report." },
      { id: "day", label: "Collapse per day", kind: "instruction", reversible: true, instruction: "On branch feat/bb-catchup, group per day, run npm test, commit locally, and report." },
      { id: "ask", label: "Show me both first", kind: "needs-context" },
    ],
  } as Ask;
}

const norm = (a: Ask) => normalizeAsk(a);

describe("validate defaults", () => {
  it("infers kinds, ids, ask_key and request_id", () => {
    const a = base();
    a.options = [{ label: "Collapse per project", instruction: "do the project thing" }, { label: "Show me both first" }] as Ask["options"];
    a.recommendation = "collapse-per-project";
    a.question = "  Collapse   Routine items?  ";
    const n = norm(a);
    expect(n.options![0].kind).toBe("instruction");
    expect(n.options![1].kind).toBe("needs-context");
    expect(n.options![0].id).toBe("collapse-per-project");
    expect(n.options![1].id).toBe("show-me-both-first");
    expect(n.ask_key).toBe("collapse routine items?");
    expect(n.request_id).toBe(identity(n));
    const long = base();
    long.question = "x".repeat(300);
    expect(Array.from(norm(long).ask_key!).length).toBe(120);
  });
});

describe("validate rules", () => {
  const cases: [string, (a: Ask) => void, string][] = [
    ["bad version", (a) => { (a as any).v = 2; }, "v must be 1"],
    ["bad kind", (a) => { (a as any).kind = "vote"; }, "kind must be"],
    ["instruction on needs-context", (a) => { a.options![2].instruction = "x"; }, "instruction is only allowed on an instruction option"],
    ["instruction kind without text", (a) => { a.options![0].instruction = ""; }, "an instruction option needs an instruction"],
    ["instruction too long", (a) => { a.options![0].instruction = "a".repeat(2001); }, "instruction must be 1-2000"],
    ["instruction NUL", (a) => { a.options![0].instruction = "a\u0000b"; }, "NUL"],
    ["instruction lone surrogate", (a) => { a.options![0].instruction = "a\ud800b"; }, "valid UTF-8"],
    ["duplicate ids", (a) => { a.options![1].id = "project"; }, "duplicate option id"],
    ["bad id", (a) => { a.options![1].id = "Day!"; }, "option id must match"],
    ["one option", (a) => { a.options = a.options!.slice(0, 1); a.recommendation = ""; }, "2-6 options"],
    ["seven options", (a) => { a.options = ["a", "b", "c", "d", "e", "f", "g"].map((id) => ({ id, label: id })) as Ask["options"]; a.recommendation = ""; }, "2-6 options"],
    ["label too long", (a) => { a.options![0].label = "l".repeat(81); }, "label must be 1-80"],
    ["empty label", (a) => { a.options![0].label = ""; }, "label must be 1-80"],
    ["recommendation not an option", (a) => { a.recommendation = "nope"; }, "recommendation must name an option"],
    ["request id chars", (a) => { a.request_id = "bad id"; }, "request_id must be"],
    ["request id long", (a) => { a.request_id = "r".repeat(129); }, "request_id must be"],
    ["ask_key too long", (a) => { a.ask_key = "k".repeat(121); }, "ask_key must be 1-120"],
    ["subject too long", (a) => { a.subject = "s".repeat(121); }, "subject must be 1-120"],
    ["asker", (a) => { (a as any).asker = "bob"; }, "asker must be"],
    ["thread asker without thread", (a) => { a.thread = ""; }, "thread asker needs a thread"],
    ["mycroft with thread", (a) => { a.asker = "mycroft"; }, "mycroft asker takes no thread"],
    ["missing project", (a) => { a.project = ""; }, "project is required"],
    ["relative root", (a) => { a.project_root = "rel/path"; }, "project_root must be an absolute path"],
    ["supersedes with mention_of", (a) => { a.supersedes = "dec_1"; a.mention_of = "dec_2"; }, "supersedes and mention_of"],
    ["approval token in question", (a) => { a.question = "ok? APPROVED-MERGE owner/repo#1 abc"; }, "approval tokens are not accepted in Home text"],
    ["approval token in label", (a) => { a.options![0].label = "APPROVED-DEPLOY"; }, "approval tokens are not accepted in Home text"],
    ["approval token in instruction", (a) => { a.options![0].instruction = "x\nAPPROVED-RELEASE y"; }, "approval tokens are not accepted in Home text"],
    ["reversible with approval", (a) => { a.options![0].approval = { kind: "merge", target: "o/r#1", identity: "abc" }; }, "reversible is not allowed on an option with an approval"],
    ["mycroft instruction", (a) => {
      a.asker = "mycroft"; a.thread = "";
      a.options = [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "instruction", instruction: "x" }] as Ask["options"];
      a.recommendation = "";
    }, "mycroft may offer only ruling-only"],
    ["mycroft reversible", (a) => {
      a.asker = "mycroft"; a.thread = "";
      a.options = [{ id: "a", label: "A", kind: "ruling-only", reversible: true }, { id: "b", label: "B", kind: "ruling-only" }] as Ask["options"];
      a.recommendation = "";
    }, "reversible is not allowed when asker is mycroft"],
    ["decide with steps", (a) => { a.steps = ["x"]; }, "decide takes no steps or machine"],
    ["approval kind", (a) => { a.options![2].approval = { kind: "lunch" as any, target: "t", identity: "i" }; }, "approval kind must be"],
  ];
  for (const [name, edit, want] of cases) {
    it(name, () => {
      const a = base();
      edit(a);
      expect(() => norm(a)).toThrow(want);
    });
  }

  it("steps and machine kinds", () => {
    const s = base();
    s.kind = "steps"; s.options = undefined; s.recommendation = "";
    s.steps = ["Turn on Tailscale", "Approve the token"];
    norm(s);
    s.options = [{ label: "x" }, { label: "y" }] as Ask["options"];
    expect(() => norm(s)).toThrow("steps takes no options");
    s.options = undefined; s.steps = undefined;
    expect(() => norm(s)).toThrow("steps needs 1-20 steps");

    const m = base();
    m.kind = "machine"; m.options = undefined; m.recommendation = "";
    m.machine = { class: "dns", detail: "zone is stale" };
    norm(m);
    m.machine = undefined;
    expect(() => norm(m)).toThrow("machine needs a machine block");
    m.machine = { class: "DNS!", detail: "x" };
    expect(() => norm(m)).toThrow("machine class must match");
    m.machine = { class: "dns", detail: "x", owner_thread: "thr_1" };
    m.steps = ["x"];
    expect(() => norm(m)).toThrow("machine takes no steps");
  });

  it("an approval option may be ruling-only", () => {
    const a = base();
    a.options![2].approval = { kind: "merge", target: "o/r#1", identity: "abc123" };
    norm(a);
  });

  it("refuses unknown fields", () => {
    expect(() => parseAsk({ v: 1, kind: "steps", single_use: true })).toThrow("unknown field");
    expect(() => parseAsk({ v: 1, kind: "decide", options: [{ id: "a", label: "A", approval: { kind: "merge", target: "t", identity: "i", single_use: true } }] })).toThrow("unknown field");
  });
});

describe("identity and revision", () => {
  it("cover the scope", () => {
    const a = norm(base());
    const id0 = identity(a), rev0 = revision(a);
    expect(id0).toHaveLength(64);
    expect(rev0).toHaveLength(64);
    const edits: ((a: Ask) => void)[] = [
      (a) => { a.project = "Other"; },
      (a) => { a.project_root = "/tmp/other"; },
      (a) => { a.thread = "thr_zzz"; },
      (a) => { a.asker = "mycroft"; a.thread = ""; a.options = [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }] as Ask["options"]; a.recommendation = ""; },
    ];
    for (const e of edits) {
      const b = base();
      e(b);
      const n = norm(b);
      expect(identity(n)).not.toBe(id0);
      expect(revision(n)).not.toBe(rev0);
    }
  });

  it("request_id never affects identity; ask_key moves identity but not revision", () => {
    const a = norm(base());
    const b = base(); b.request_id = "req_custom";
    expect(identity(norm(b))).toBe(identity(a));
    const c = base(); c.ask_key = "different key";
    const nc = norm(c);
    expect(identity(nc)).not.toBe(identity(a));
    expect(revision(nc)).toBe(revision(a));
    const d = base(); d.options![0].instruction += " extra";
    expect(revision(norm(d))).not.toBe(revision(a));
    const e = base(); e.supersedes = "dec_1";
    expect(revision(norm(e))).not.toBe(revision(a));
  });

  it("ignore key order and NFC form", () => {
    const a = parseAsk(JSON.parse('{"v":1,"kind":"decide","project":"P","project_root":"/r","asker":"thread","thread":"t1","question":"Caf\\u00e9?","options":[{"label":"A"},{"label":"B"}]}'));
    const b = parseAsk(JSON.parse('{"options":[{"label":"A"},{"label":"B"}],"question":"Cafe\\u0301?","thread":"t1","asker":"thread","project_root":"/r","project":"P","kind":"decide","v":1}'));
    expect(identity(a)).toBe(identity(b));
    expect(revision(a)).toBe(revision(b));
    expect(a.request_id).toBe(b.request_id);
  });
});

describe("semantic key", () => {
  it("moves with meaning and not with bookkeeping", () => {
    const k0 = semanticKey(norm(base()));
    expect(k0).toHaveLength(64);
    const same = base(); same.ask_key = "another"; same.request_id = "req_x";
    expect(semanticKey(norm(same))).toBe(k0);
    const other = base(); other.thread = "thr_other";
    expect(semanticKey(norm(other))).toBe(k0);
    const diffs: Record<string, (a: Ask) => void> = {
      kind: (a) => { a.kind = "steps"; a.options = undefined; a.recommendation = ""; a.steps = ["do it"]; },
      subject: (a) => { a.subject = "autarch/catch-up: other"; },
      question: (a) => { a.question = "Something else entirely?"; },
      label: (a) => { a.options![0].label = "Collapse per project!"; },
      instruction: (a) => { a.options![0].instruction += "."; },
      reversible: (a) => { a.options![0].reversible = false; },
      approval: (a) => { a.options![2].approval = { kind: "merge", target: "o/r#1", identity: "sha1" }; },
      recommendation: (a) => { a.recommendation = "day"; },
    };
    for (const [name, edit] of Object.entries(diffs)) {
      const b = base(); edit(b);
      expect(semanticKey(norm(b)), name).not.toBe(k0);
    }
    const p1 = base(), p2 = base();
    p1.options![2].approval = { kind: "merge", target: "o/r#1", identity: "sha1" };
    p2.options![2].approval = { kind: "merge", target: "o/r#1", identity: "sha2" };
    expect(semanticKey(norm(p1))).not.toBe(semanticKey(norm(p2)));
  });

  it("separates feature A from B, blockers by detail, and gives no key without a subject", () => {
    const yn = (q: string): Ask => ({ v: 1, kind: "decide", subject: "autarch: remove feature", project: "P", project_root: "/r", asker: "thread", thread: "t", question: q, options: [{ id: "yes", label: "Yes" }, { id: "no", label: "No" }] }) as Ask;
    expect(semanticKey(norm(yn("Remove feature A?")))).not.toBe(semanticKey(norm(yn("Remove feature B?"))));
    const mach = (detail: string): Ask => ({ v: 1, kind: "machine", subject: "blocker", project: "P", project_root: "/r", asker: "thread", thread: "t", question: "blocked", machine: { class: "dns", detail } }) as Ask;
    expect(semanticKey(norm(mach("zone a")))).not.toBe(semanticKey(norm(mach("zone b"))));
    const ns = base(); ns.subject = "";
    expect(semanticKey(norm(ns))).toBe("");
  });
});

describe("vectors (byte for byte with Go)", () => {
  const v = JSON.parse(readFileSync(join(schemaDir, "vectors.json"), "utf8"));

  it("has the expected shape", () => {
    expect(v.canonical.length).toBeGreaterThanOrEqual(6);
    expect(v.asks.length).toBeGreaterThanOrEqual(6);
    expect(v.invalid.length).toBeGreaterThanOrEqual(4);
  });
  for (const c of v.canonical) {
    it(`canonical: ${c.name}`, () => expect(canonicalJson(c.input)).toBe(c.expected));
  }
  for (const c of v.asks) {
    it(`ask: ${c.name}`, () => {
      const a = parseAsk(c.input);
      expect(normalizedJson(a)).toBe(c.normalized);
      expect(identity(a)).toBe(c.identity);
      expect(revision(a)).toBe(c.revision);
      expect(semanticKey(a)).toBe(c.semantic_key);
    });
  }
  for (const c of v.invalid) {
    it(`invalid: ${c.name}`, () => expect(() => parseAsk(c.input)).toThrow(c.error));
  }
});

describe("schema", () => {
  it("lists exactly the ask fields", () => {
    const s = JSON.parse(readFileSync(join(schemaDir, "ask.schema.json"), "utf8"));
    expect(Object.keys(s.properties).sort()).toEqual(
      ["v", "kind", "request_id", "ask_key", "subject", "project", "project_root", "asker", "thread", "question", "recommendation", "supersedes", "mention_of", "options", "steps", "machine"].sort(),
    );
  });
});
