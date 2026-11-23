import { describe, expect, it } from "vitest";
import { parseCard } from "../cards.js";

export const metadata = { thread: "thr_23456789ab", names: ["FAKE_TEST_TOKEN"], path: "/fake-test/credentials.env", project: "fake-test-project" };
const base = 'Provide the requested credential.\nRequest: fake-test sha256:0123456789abcdef\n```home-ask\n' + JSON.stringify({ schema: "home-ask/v2", project: "fake-test-project", project_root: "/fake-test", question: "Provide the requested credential.", options: [{ id: "later", label: "Later" }, { id: "done", label: "Done" }] }) + '\n```\n';
export const description = (m: unknown) => base + '```home-secret\n' + JSON.stringify(m) + '\n```\n';
describe("home-secret metadata", () => {
  it("parses metadata without leaving the fence in the question", () => {
    const card = parseCard(description(metadata));
    expect(card.question).toBe("Provide the requested credential.");
    expect(card).toHaveProperty("secret", metadata);
  });
  for (const key of ["extra", "value", "values", "token", "password", "secret", "VALUE"]) {
    it(`rejects ${key} without echoing its content`, () => {
      expect(() => parseCard(description({ ...metadata, [key]: "fake-test-value-not-a-secret" }))).toThrow();
      try { parseCard(description({ ...metadata, [key]: "fake-test-value-not-a-secret" })); } catch (e) { expect(String(e)).not.toContain("fake-test-value-not-a-secret"); }
    });
  }
  for (const m of [{ ...metadata, thread: "bad" }, { ...metadata, names: ["TOKEN=value"] }, { ...metadata, path: "relative" }, { ...metadata, names: [] }, { ...metadata, names: ["__proto__"] }]) {
    it(`rejects invalid metadata ${JSON.stringify(m)}`, () => expect(() => parseCard(description(m))).toThrow());
  }
  it("rejects duplicate blocks", () => expect(() => parseCard(description(metadata) + '```home-secret\n' + JSON.stringify(metadata) + '\n```')).toThrow());
});
