import { describe, expect, it } from "vitest";
import { parseCard, toV1 } from "../cards";
import { parseAsk, parseExistingAsk, normalizedJson, revision } from "../model";
import { readAsk } from "../askbody";

const declaration = () => ({ schema: 1, target_thread: "thr_target", reversible: false, subject: { kind: "none" }, ruling: { bead: null, under: [] } });
const wire = () => ({ schema: "home-ask/v2", project: "sample", project_root: "/sample", question: "Which colour?", options: [{ id: "blue", label: "Blue", kind: "instruction", instruction: "Use blue 🟦.", reversible: true, on_pick: declaration() }, { id: "wait", label: "Wait", kind: "ruling-only" }] });
const card = (ask: unknown) => parseCard(`Which colour?\nBlocks: thread:thr_target\nRequest: sample sha256:0123456789abcdef\n\n\`\`\`home-ask\n${JSON.stringify(ask)}\n\`\`\``);

describe("on_pick card declaration", () => {
  it("reads persisted declarations only with trusted card source", () => {
    const body = JSON.stringify(toV1(card(wire()), "thr_asker"));
    expect(readAsk(body)).toBeNull();
    expect(readAsk(body, {}, "home")).toBeNull();
    expect(readAsk(body, {}, "card")?.options?.[0].on_pick).toEqual(declaration());
  });
  it("preserves false reversibility independently from delegation and exact Unicode text", () => {
    const a = toV1(card(wire()), "thr_asker");
    expect(JSON.parse(normalizedJson(a)).options[0].on_pick).toEqual(declaration());
    expect(a.options![0].reversible).toBe(true);
    expect(a.options![0].instruction).toBe("Use blue 🟦.");
  });
  it("refuses both public and retained v1 filing", () => {
    const a = toV1(card(wire()), "thr_asker");
    expect(() => parseAsk(a)).toThrow();
    expect(() => parseExistingAsk(a)).toThrow();
  });
  it("changes revision when declaration behavior changes", () => {
    const first = toV1(card(wire()), "thr_asker");
    const next = wire(); next.options[0].on_pick!.reversible = true;
    expect(revision(toV1(card(next), "thr_asker"))).not.toBe(revision(first));
  });
  for (const [name, value] of Object.entries({ null: null, missing: {}, unknown: { ...declaration(), extra: true }, version: { ...declaration(), schema: 2 } })) {
    it(`rejects strict raw declaration: ${name}`, () => {
      const ask = wire(); Object.assign(ask.options[0], { on_pick: value });
      expect(() => card(ask)).toThrow();
    });
  }
  for (const text of ["Use e\u0301.", "Use \ud800.", "deploy", "ＭＥＲＧＥ", "force—push"]) {
    it(`rejects altered or privileged relay text ${JSON.stringify(text)}`, () => {
      const ask = wire(); ask.options[0].instruction = text;
      expect(() => card(ask)).toThrow();
    });
  }
});
