import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AskCard } from "../ui/asks.js";
import { CardConversation, ConversationBody, ConversationProvider, HOME_POSTED_LABEL, type ConversationApi, type ConversationData } from "../ui/conversation.js";
import { MoveCard } from "../ui/yourmove.js";

const data = (over: Partial<ConversationData> = {}): ConversationData => ({
  task_id: "T1",
  comments: [
    { id: "c1", author_class: "owner", author_name: "thr_a", body: "<img src=x onerror=alert(1)> **done** & ready", created_at: "2026-10-01T00:00:00.000Z", home_posted: false, unread: true },
    { id: "c2", author_class: "mk", author_name: "You", body: "ok, thanks", created_at: "2026-10-01T00:01:00.000Z", home_posted: true, unread: false },
  ],
  unread: 1,
  stale: false,
  last_ok_at: "2026-10-01T00:02:00.000Z",
  options: [
    { id: "a", label: "Per project", kind: "instruction", instruction: "Group per project.\nRun the tests, then report." },
    { id: "b", label: "Just record", kind: "ruling-only", instruction: null },
  ],
  ...over,
});
const api = (unread: Record<string, number> = {}): ConversationApi => ({ unread, load: async () => data(), markSeen: () => {} });

describe("conversation view", () => {
  it("shows comment text as plain text, never HTML", () => {
    const html = renderToStaticMarkup(<ConversationBody data={data()} />);
    expect(html).toContain("&lt;img src=x onerror=alert(1)&gt; **done** &amp; ready");
    expect(html).not.toContain("<img");
  });

  it("labels author classes, an unread dot only on unread comments, and Home-posted replies as unattested", () => {
    const html = renderToStaticMarkup(<ConversationBody data={data()} />);
    expect(html).toContain('data-author-class="owner"');
    expect(html).toContain('data-author-class="mk"');
    expect(html.match(/data-unread-dot/g)).toHaveLength(1);
    expect(html.match(/data-home-posted/g)).toHaveLength(1);
    expect(html).toContain(HOME_POSTED_LABEL);
  });

  it("says comments may be stale only when the last read failed", () => {
    expect(renderToStaticMarkup(<ConversationBody data={data()} />)).not.toContain("may be stale");
    expect(renderToStaticMarkup(<ConversationBody data={data({ stale: true })} />)).toContain("Comments may be stale");
  });

  it("the full instruction is a collapsible with each option's instruction text; none for a ruling-only option's text", () => {
    const html = renderToStaticMarkup(<ConversationBody data={data()} />);
    expect(html).toContain("<details");
    expect(html).toContain("Full instruction");
    expect(html).toContain("Group per project.\nRun the tests, then report.");
    expect(html).toContain("Nothing is sent for this option.");
    expect(renderToStaticMarkup(<ConversationBody data={data({ options: [{ id: "b", label: "x", kind: "ruling-only", instruction: null }] })} />)).not.toContain("Full instruction");
  });

  it("a closed conversation shows the unread dot from the counts; no provider or no task id renders nothing", () => {
    const withDot = renderToStaticMarkup(<ConversationProvider value={api({ T1: 3 })}><CardConversation taskId="T1" /></ConversationProvider>);
    expect(withDot).toContain("data-unread-dot");
    expect(withDot).not.toContain("Full instruction");
    expect(renderToStaticMarkup(<ConversationProvider value={api()}><CardConversation taskId="T1" /></ConversationProvider>)).not.toContain("data-unread-dot");
    expect(renderToStaticMarkup(<CardConversation taskId="T1" />)).toBe("");
    expect(renderToStaticMarkup(<ConversationProvider value={api()}><CardConversation taskId={null} /></ConversationProvider>)).toBe("");
  });

  it("the card itself no longer shows the instruction text: it is only behind the conversation", () => {
    const ask = {
      id: "d1", project: "p", thread: "thr-a", subject: "s", asker: "card", filed_at: "2026-10-01T00:00:00.000Z", revision: "r", task_id: "T1",
      ask: { question: "q?", options: [{ id: "a", label: "Per project", kind: "instruction", instruction: "SECRET-FULL-TEXT. More." }] },
    };
    const closed = renderToStaticMarkup(<ConversationProvider value={api()}><AskCard ask={ask} onPick={() => {}} onOpen={() => {}} /></ConversationProvider>);
    expect(closed).toContain("data-card-conversation");
    expect(closed).not.toContain("Full instruction");
    expect(closed).not.toContain("More.");
    const move = renderToStaticMarkup(
      <ConversationProvider value={api()}>
        <MoveCard m={{ task_id: "T1", generation: 1, kind: "read", state: "open", title: "t", owner: null, opened_at: "2026-10-01T00:00:00.000Z", claimed_at: null, skipped_at: null, checked_at: null, report_deadline_at: null, url: null, need: null, script: null, commands: [], report: null, closed_at: null, closed_by: null, evidence: null } as never} h={{ onCheck() {}, onClaim() {}, onSkip() {}, onNote() {} }} section="yourMove" />
      </ConversationProvider>,
    );
    expect(move).toContain('data-card-conversation="T1"');
  });
});
