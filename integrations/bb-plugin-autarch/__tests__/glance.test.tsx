// Play and walk cards (bead mk-a4o0.19): what the card is, the link, the time, what is being judged and what each
// answer does sit at the top of the card, above the long text, on phone and desktop. The four cards below have the
// shapes of the parked check cards (a phone check, a playtest, a picture check with a file, a walk with a command),
// with made-up links and names. Pure extraction is tested directly; the block is tested through markup order.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { AskCard, type Option, type OwedAsk } from "../ui/asks.js";
import { glance } from "../ui/glance.js";
import { inboxRows } from "../ui/inbox.js";

const RULING = (id: string, label: string) => ({ id, label, kind: "ruling-only" });
const card = (subject: string, question: string, options: Option[] = [RULING("a", "A: looks right"), RULING("b", "B: something is off")], over: Partial<OwedAsk> = {}): OwedAsk => ({
  id: "dec1", project: "Sylveste", thread: "thr-a", subject, asker: "thread", filed_at: "2026-10-09T06:00:00Z", revision: "rev1", task_id: "task-1",
  ask: { question, recommendation: "a", options }, ...over,
});

const PHONE = card(
  "Check the new phone Home (about 2 minutes)",
  "The phone inbox you picked is live at https://home.example.test. On your phone: 1) you see 'Waiting on you' with three tabs; 2) tapping a row opens the full card, with Later on the left; 3) one tap takes the card off the list; 4) Full view (top right) shows the desktop layout, and 'Back' returns. Only render tests have run so far.",
);
const PLAY = card(
  "Prototype G1: play the slice now and give a verdict?",
  'The slice is ready for your playtest at https://play.example.test (the head of https://github.com/example/proto/pull/9; review passed; 153/153 tests). Play at least one round and click Export. Then answer in your own words: 1) Does it feel like making the thing asked for? 2) Are cutting and sewing enjoyable, not chores? 3) Do the mannequin and "why?" show your choices? 4) Do you want another round? Put your verdict in the note. Merging PR #9 is yours too.',
  [RULING("play-now", "I play now; verdict in the note"), RULING("merge-first", "Merge PR #9 first, then I play"), RULING("later", "Hold: I will play later")],
);
const SHEET = card(
  "Street check: three yes/no questions from pictures (2 min)",
  "This replaces the walk you said was not ready. Open /srv/example/sheet/street-sheet.md; the renders sit beside it. It asks three yes/no questions: (1) do the people look the right size; (2) is the sidewalk about the right width; (3) does the street-level view feel like that street. Caveats on the sheet: it is a software render with a third-person camera only. Answer in your own words.",
  [RULING("all-yes", "All three look right"), RULING("some-no", "Some are wrong"), RULING("cant", "Cannot judge from pictures")],
);
const WALK = card(
  "Walk the destinations PR before merging",
  "PR #21 (destinations first, Phase 1) changes default gameplay: the street clock drives the counter. Code passed review, but nobody has checked yet whether a player knows what to do, or can read the signs. The walk takes a display, so run it on the Mac.\n\nscp host:/srv/x/walk.sh /tmp/x.sh && bash /tmp/x.sh --check\n\nPR: https://github.com/example/app/pull/21",
  [RULING("a", "A: I walk it first, merge if it reads right"), RULING("b", "B: merge now, walk later"), RULING("c", "C: hold the PR")],
);
const PLAIN = card("Which day should the release go out?", "Pick a day. See https://example.test/notes for the notes.", [RULING("mon", "Monday"), RULING("tue", "Tuesday")], { id: "dec2" });

describe("glance: pulling the context out of a check card", () => {
  it("finds the link, the stated time and the numbered things to judge in a phone check", () => {
    const g = glance(PHONE)!;
    expect(g.link).toEqual({ kind: "url", href: "https://home.example.test", text: "home.example.test" });
    expect(g.time).toBe("About 2 min");
    expect(g.judging).toHaveLength(4);
    expect(g.judging[0]).toBe("you see 'Waiting on you' with three tabs");
    expect(g.judging[3]).toBe("Full view (top right) shows the desktop layout, and 'Back' returns");
    expect(g.lead).toMatch(/^The phone inbox you picked is live at/);
  });
  it("takes the first link (the build to play, not the PR), and ends the last question at its sentence", () => {
    const g = glance(PLAY)!;
    expect(g.link?.href).toBe("https://play.example.test");
    expect(g.time).toBeNull();
    expect(g.judging).toEqual(["Does it feel like making the thing asked for?", "Are cutting and sewing enjoyable, not chores?", 'Do the mannequin and "why?" show your choices?', "Do you want another round?"]);
  });
  it("shows a file path as a copyable path, reads (1) (2) (3) lists, and a time in the title", () => {
    const g = glance(SHEET)!;
    expect(g.link).toEqual({ kind: "path", href: "/srv/example/sheet/street-sheet.md", text: "/srv/example/sheet/street-sheet.md" });
    expect(g.time).toBe("2 min");
    expect(g.judging).toEqual(["do the people look the right size", "is the sidewalk about the right width", "does the street-level view feel like that street"]);
  });
  it("on a walk with no list, judges what the card says nobody has checked, and takes its link from the prose, not the command", () => {
    const g = glance(WALK)!;
    expect(g.link?.href).toBe("https://github.com/example/app/pull/21");
    expect(g.time).toBeNull();
    expect(g.judging).toEqual(["whether a player knows what to do, or can read the signs"]);
  });
  it("leaves an ordinary decision alone, even one with a link", () => {
    expect(glance(PLAIN)).toBeNull();
  });
  it("never makes a link out of a non-web scheme", () => {
    const g = glance(card("Play the build", "Open javascript:alert(1) or https://ok.example.test now."))!;
    expect(g.link?.href).toBe("https://ok.example.test");
    expect(glance(card("Play the build", "Open javascript:alert(1) now."))?.link ?? null).toBeNull();
  });
  it("does not read a version or PR number as a list, and needs the list to start at 1", () => {
    expect(glance(card("Check the build", "Look at https://x.example.test. v2) is out, 3) here too."))!.judging).toEqual([]);
  });
  it("takes a list only when its markers count 1, 2, 3 in order, with no skip, repeat or swap", () => {
    const j = (q: string) => glance(card("Check the build", `Look at https://x.example.test. ${q}`))!.judging;
    expect(j("1) first; 3) third; 2) second")).toEqual([]);
    expect(j("1) first; 2) second; 2) again")).toEqual([]);
    expect(j("1) first; 3) third")).toEqual([]);
    expect(j("1) first; 2) second; 3) third")).toEqual(["first", "second", "third"]);
  });
  it("rejects a list whose first marker is not 1, whatever follows", () => {
    const j = (q: string) => glance(card("Check the build", `Look at https://x.example.test. ${q}`))!.judging;
    expect(j("2) second; 1) first; 2) second")).toEqual([]);
  });
  it("reads a time only when it is stated as how long the check takes, never a past or future moment", () => {
    const t = (q: string, title = "Check the build") => glance(card(title, `Open https://home.example.test. ${q}`))!.time;
    expect(t("The build went live 2 hours ago.")).toBeNull();
    expect(t("It is live about 2 hours ago.")).toBeNull();
    expect(t("Deadline is in 3 days, or 5 minutes after the sync.")).toBeNull();
    expect(t("It takes about 10 minutes.")).toBe("About 10 min");
    expect(t("Published (about 2 hours) ago.")).toBeNull();
    expect(t("The previous playtest took only 5 minutes. This one is longer.")).toBeNull();
    expect(t("The playtest takes about 1 hour 30 minutes.")).toBeNull();
    expect(t("It takes 5 minutes, or 8 minutes with the export.")).toBeNull();
    expect(t("It takes 10 minutes.")).toBe("10 min");
    expect(t("The previous playtest lasted about 5 minutes. This one is longer.")).toBeNull();
    expect(t("It is about 5 minutes of play.")).toBeNull();
    expect(t("This check takes about 10 minutes.", "Check the build (2 min)")).toBeNull();
    expect(t("It takes 2 minutes.", "Check the build (2 min)")).toBe("2 min");
    expect(t("This check takes 5 minutes, or 8 minutes with the export.", "Check the build (2 min)")).toBeNull();
    expect(t("This check takes about 1 hour 30 minutes.", "Check the build (2 min)")).toBeNull();
    expect(t("Nothing about how long.", "Check the build (2 min)")).toBe("2 min");
    expect(t("Anything.", "Check the build (2 min)")).toBe("2 min");
  });
  it("takes the judging sentence only where it names an open check, and ignores a settled whether", () => {
    expect(glance(card("Check the build", "Open https://play.example.test. We already settled whether to ship. Check whether the controls feel good."))!.judging).toEqual(["whether the controls feel good"]);
    expect(glance(card("Check the build", "Open https://play.example.test. We already settled whether to ship."))!.judging).toEqual([]);
  });
  it("does not cut a judging question at an abbreviation", () => {
    expect(glance(card("Check the build", "Open https://x.example.test. 1) Can you find the Dr. Smith sign? 2) Can you read it, e.g. from the street?"))!.judging).toEqual(["Can you find the Dr. Smith sign?", "Can you read it, e.g. from the street?"]);
  });
  it("does not put a backtick or other closer into a link", () => {
    expect(glance(card("Check the page", "Open `https://play.example.test` and play a round."))!.link!.href).toBe("https://play.example.test");
  });
  it("gives no time for a negated or bounded duration", () => {
    const t = (q: string) => glance(card("Check the build", `Open https://home.example.test. ${q}`))!.time;
    expect(t("This no longer takes 5 minutes.")).toBeNull();
    expect(t("It doesn't take 5 minutes.")).toBeNull();
    expect(t("It takes more than 5 minutes.")).toBeNull();
    expect(t("It takes 1 hour and a half.")).toBeNull();
    expect(t("It takes 1.5 hours.")).toBeNull();
    expect(t("It takes 1½ hours.")).toBeNull();
  });
  it("keeps the closing parenthesis of a link that has one, and drops one that closes the prose", () => {
    expect(glance(card("Check the page", "Open https://en.example.test/wiki/Prototype_(game) now."))!.link!.href).toBe("https://en.example.test/wiki/Prototype_(game)");
    expect(glance(card("Check the page", "Open it (see https://en.example.test/page) now."))!.link!.href).toBe("https://en.example.test/page");
  });
  it("leaves a policy decision alone even when its title says check and it links background", () => {
    expect(glance(card("Which CI check should block merging?", "Choose the merge policy. Background: https://example.test/policy"))).toBeNull();
  });
  it("needs the title to ask for the check, not mention one", () => {
    expect(glance(card("Which CI check should block merging?", "Open https://example.test/policy for background, then choose the merge policy."))).toBeNull();
    expect(glance(card("Prototype G1: play the slice now?", "Open https://play.example.test and play a round."))).not.toBeNull();
  });
  it("is not a check card just because the title has a duration", () => {
    expect(glance(card("Release in 2 hours", "Pick the rollout day. See https://example.test/notes."))).toBeNull();
    expect(glance(card("Rotate the key within 5 minutes", "Which key? https://example.test/k"))).toBeNull();
    expect(glance(card("Which check took 5 minutes?", "Open https://example.test/k"))).toBeNull();
  });
});

describe("the At a glance block on a card", () => {
  const html = (a: OwedAsk) => renderToStaticMarkup(<AskCard ask={a} onPick={(() => {}) as never} onOpen={() => {}} nowMs={Date.parse("2026-10-09T08:00:00Z")} />);
  it("comes before the long question and before the pick buttons, so it is on screen without scrolling", () => {
    for (const a of [PHONE, PLAY, SHEET, WALK]) {
      const h = html(a);
      const at = h.indexOf("data-glance");
      expect(at).toBeGreaterThan(-1);
      expect(at).toBeLessThan(h.indexOf("data-question") === -1 ? h.indexOf("whitespace-pre-wrap") : h.indexOf("data-question"));
      expect(at).toBeLessThan(h.indexOf("data-option="));
    }
  });
  it("has a tap-height open button for a web link, with a safe target, and the stated time", () => {
    const h = html(PHONE);
    const a = h.match(/<a [^>]*data-glance-link[^>]*>/)![0];
    expect(a).toContain('href="https://home.example.test"');
    expect(a).toContain('rel="noopener noreferrer"');
    expect(a).toContain('target="_blank"');
    expect(a).toContain("min-h-11");
    expect(h).toMatch(/data-glance-time[^>]*>About 2 min/);
  });
  it("shows a file as a copyable path, not a link", () => {
    const h = html(SHEET);
    expect(h).not.toContain("data-glance-link");
    expect(h).toMatch(/data-glance-path[^>]*>\/srv\/example\/sheet\/street-sheet\.md/);
  });
  it("says so when the card states no time, instead of guessing", () => {
    expect(html(PLAY)).toMatch(/data-glance-time[^>]*>Time not stated/);
  });
  it("lists what is judged as a numbered list", () => {
    const h = html(PLAY);
    expect(h).toContain("data-glance-judging");
    expect(h.match(/<li[^>]*data-glance-item/g)).toHaveLength(4);
  });
  it("lists every answer with what it does: a ruling-only pick sends nothing", () => {
    const h = html(PLAY);
    expect(h.match(/<li[^>]*data-glance-answer/g)).toHaveLength(3);
    expect(h).toMatch(/data-glance-answers[\s\S]*Merge PR #9 first, then I play/);
    expect(h).toMatch(/records your pick only/i);
  });
  it("spells out what an answer that acts will do", () => {
    const a = card("Check the deploy (5 min)", "Look at https://x.example.test: 1) does it load? 2) is it fast?", [{ id: "go", label: "Ship it", kind: "instruction", instruction: "Deploy the build to production now." }, RULING("no", "Not yet")]);
    const h = html(a);
    expect(h).toMatch(/data-glance-answer[^>]*>[\s\S]*Ship it[\s\S]*Tells thr-a: Deploy the build/);
  });
  it("shows the whole instruction an acting answer sends, not its first sentence", () => {
    const a = card("Check the deploy (5 min)", "Look at https://x.example.test: 1) does it load? 2) is it fast?", [{ id: "go", label: "Ship it", kind: "instruction", instruction: "Run the tests. Then deploy the build to production now." }, RULING("no", "Not yet")]);
    const h = html(a);
    const g = h.slice(h.indexOf("data-glance-answers"), h.indexOf("</section>"));
    expect(g).toContain("Run the tests. Then deploy the build to production now.");
  });
  it("keeps each answer's own line when the options are mixed, and says it once only when all are ruling-only", () => {
    const mixed = html(card("Check the deploy (5 min)", "Look at https://x.example.test: 1) does it load? 2) is it fast?", [{ id: "go", label: "Ship it", kind: "instruction", instruction: "Deploy it." }, RULING("no", "Not yet")]));
    const block = mixed.slice(mixed.indexOf("data-glance-answers"), mixed.indexOf("</section>"));
    expect(block).toMatch(/Not yet[\s\S]*Records your pick only; nothing is sent\./);
    expect(block).not.toContain("data-glance-records");
    expect(html(PLAY)).toContain("data-glance-records");
  });
  it("keeps the full text, the commands and the pick buttons below it", () => {
    const h = html(WALK);
    expect(h).toContain("bash /tmp/x.sh ");
    expect(h).toContain("data-command-text");
    expect(h).toContain("data-option=\"a\"");
    expect(h).toContain("nobody has checked yet whether");
  });
  it("adds nothing to an ordinary decision card", () => {
    expect(html(PLAIN)).not.toContain("data-glance");
  });
});

describe("the phone inbox row", () => {
  it("carries the time of a check card, so the list says how long each one takes", () => {
    const data = { owed: [PHONE, PLAIN], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} };
    const rows = inboxRows(data, null, Date.parse("2026-10-09T08:00:00Z")).waiting;
    expect(rows.find((r) => r.ask?.id === PHONE.id)!.time).toBe("About 2 min");
    expect(rows.find((r) => r.ask?.id === PLAIN.id)!.time).toBeUndefined();
  });
});
