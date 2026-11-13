import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));
import { parseAsk } from "../model.js";
import { AskCard } from "../ui/asks.js";

const evidence = [{ caption: "Run", url: "https://example.test/run" }];
const input = { v: 1, project: "Example", project_root: "/example", asker: "thread", thread: "thr_test", question: "Which?", options: [{id:"a",label:"A",kind:"ruling-only"},{id:"b",label:"B",kind:"ruling-only"}] };
it("accepts evidence in the strict parser", () => {
 expect(() => parseAsk({ ...input, evidence })).not.toThrow();
});
it("renders evidence on the decision card", () => {
 const ask = { id:"d", project:"Example",thread:"thr_test",subject:"Choice",asker:"thread",filed_at:"2026-01-01",revision:"r",ask:{...input,evidence} };
 const html = renderToStaticMarkup(<AskCard ask={ask as any} onPick={() => {}} onOpen={() => {}} />);
 expect(html).toContain("Evidence");
 expect(html).toContain('rel="noopener noreferrer"');
});
