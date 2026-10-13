// Render checks for the Idea box, its list and the phone entry point (bead mk-2zojo).
import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { filterProjects, IdeaBox, IdeaList } from "../ui/ideas.js";
import { MobileInbox } from "../ui/inbox.js";

const noop = () => {};
const projects = [{ id: "p1", name: "Autarch", prefix: "AUTA" }];
const idea = { task_id: "t1", key: "AUTA-9", project_id: "p1", project: "Autarch", prefix: "AUTA", title: "t", words: "fork <b>a</b> jawntology", filed_at: "2026-10-09T00:00:00Z" };

describe("Idea box UI", () => {
  it("offers a project choice, one text box and a File button that waits for both", () => {
    const h = renderToStaticMarkup(<IdeaBox projects={projects} project="" text="" busy={false} error={null} notice={null} onProject={noop} onText={noop} onFile={noop} />);
    expect(h).toContain('role="combobox"');
    expect(h).toContain('placeholder="Choose a project"');
    expect(h).not.toContain("<select");
    expect(h).toContain("data-idea-text");
    expect(h).toMatch(/data-idea-file[^>]*disabled|disabled[^>]*data-idea-file/);
    expect(h).toContain("not work until it is picked");
  });
  it("shows the chosen project's label in the box", () => {
    expect(renderToStaticMarkup(<IdeaBox projects={projects} project="p1" text="" busy={false} error={null} notice={null} onProject={noop} onText={noop} onFile={noop} />)).toContain('value="Autarch (AUTA)"');
  });
  it("filters by name or prefix, case-insensitively, as a substring", () => {
    const ps = [{ id: "a", name: "Autarch", prefix: "AUTA" }, { id: "b", name: "Shadow Work", prefix: "SHAD" }, { id: "c", name: "Bridger", prefix: "BRID" }];
    expect(filterProjects(ps, "").map((p) => p.id)).toEqual(["a", "b", "c"]);
    expect(filterProjects(ps, "dow").map((p) => p.id)).toEqual(["b"]);
    expect(filterProjects(ps, "auta").map((p) => p.id)).toEqual(["a"]);
    expect(filterProjects(ps, " RID ").map((p) => p.id)).toEqual(["c"]);
    expect(filterProjects(ps, "zzz")).toEqual([]);
  });
  it("shows a refusal in words", () => {
    const h = renderToStaticMarkup(<IdeaBox projects={projects} project="p1" text="x" busy={false} error="tasks unavailable" notice={null} onProject={noop} onText={noop} onFile={noop} />);
    expect(h).toContain("That did not go through: tasks unavailable");
  });
  it("lists ideas as escaped plain text with Pursue, Park and Drop, and Not this week only when asked", () => {
    const h = renderToStaticMarkup(<IdeaList ideas={[idea]} busy={false} error={null} onAct={noop} onClear={noop} />);
    expect(h).toContain("fork &lt;b&gt;a&lt;/b&gt; jawntology");
    for (const a of ["pursue", "park", "drop"]) expect(h).toContain(`data-idea-act="${a}"`);
    expect(h).toContain("Not this week");
    expect(renderToStaticMarkup(<IdeaList ideas={[idea]} busy={false} error={null} onAct={noop} />)).not.toContain("Not this week");
    expect(renderToStaticMarkup(<IdeaList ideas={[]} busy={false} error={null} onAct={noop} />)).toContain("No open ideas.");
  });
});

describe("phone entry", () => {
  const base = { asks: { owed: [], settled: [], delegation: null, machineOwners: [] } as never, moves: null, waiting: 0, onPick: noop as never, onOpen: noop, onNote: noop as never, onLater: noop as never, handlers: { onCheck: noop, onClaim: noop, onSkip: noop, onNote: noop } as never, sinceNode: <p>s</p>, sinceCount: 0, onDesktop: noop };
  it("adds an Idea button beside Full view and keeps the three tabs", () => {
    const h = renderToStaticMarkup(<MobileInbox {...base} onIdea={noop} />);
    expect(h).toContain("data-inbox-idea");
    expect(h).toContain("data-inbox-desktop");
    expect((h.match(/data-inbox-tab=/g) ?? []).length).toBe(3);
  });
  it("shows the idea body in place of the list when open, and no button without a handler", () => {
    expect(renderToStaticMarkup(<MobileInbox {...base} onIdea={noop} ideaNode={<p>idea body</p>} />)).toContain("idea body");
    expect(renderToStaticMarkup(<MobileInbox {...base} />)).not.toContain("data-inbox-idea");
  });
});
