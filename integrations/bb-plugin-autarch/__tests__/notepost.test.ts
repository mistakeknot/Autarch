import { describe, expect, it } from "vitest";
import { postNoteOnce, serialPerKey } from "../notepost.js";

const marker = (id: string) => `hello\nhome-note: ${id}`;

/** A comment store whose create is slow, like a tasks write that is still pending when the retry arrives. */
function slowTasks() {
  const comments: { body: string }[] = [];
  return {
    comments,
    listComments: async () => comments.map((c) => ({ ...c })),
    createComment: async (_t: string, body: string) => {
      await new Promise((r) => setTimeout(r, 20));
      comments.push({ body });
    },
  };
}

describe("postNoteOnce", () => {
  it("two concurrent requests with one note id post exactly one comment", async () => {
    const t = slowTasks();
    await Promise.all([postNoteOnce(t, "T1", "n1", marker("n1")), postNoteOnce(t, "T1", "n1", marker("n1"))]);
    expect(t.comments).toHaveLength(1);
  });
  it("different note ids still both post, and the same id on another task posts too", async () => {
    const t = slowTasks();
    await Promise.all([postNoteOnce(t, "T1", "n1", marker("n1")), postNoteOnce(t, "T1", "n2", marker("n2")), postNoteOnce(t, "T2", "n1", marker("n1"))]);
    expect(t.comments).toHaveLength(3);
  });
  it("a failed first attempt does not block the retry", async () => {
    const t = slowTasks();
    let fail = true;
    const flaky = { ...t, createComment: async (x: string, b: string) => { if (fail) { fail = false; throw new Error("down"); } return t.createComment(x, b); } };
    await expect(postNoteOnce(flaky, "T1", "n1", marker("n1"))).rejects.toThrow("down");
    await postNoteOnce(flaky, "T1", "n1", marker("n1"));
    expect(t.comments).toHaveLength(1);
  });
  it("runs same-key work in order and never overlaps it", async () => {
    let active = 0, peak = 0;
    const job = () => serialPerKey("k", async () => { active++; peak = Math.max(peak, active); await new Promise((r) => setTimeout(r, 5)); active--; });
    await Promise.all([job(), job(), job()]);
    expect(peak).toBe(1);
  });
});
