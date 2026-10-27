import { describe, expect, it } from "vitest";
import { registerWithOptionalField } from "../ui/optionalslot";

describe("registerWithOptionalField", () => {
  const panel = { id: "p", title: "P" };
  const field = { extra: 1 };

  it("registers once with the field when the host accepts it", () => {
    const seen: object[] = [];
    expect(registerWithOptionalField((p) => seen.push(p), panel, field)).toBe("with");
    expect(seen).toEqual([{ id: "p", title: "P", extra: 1 }]);
  });

  it("registers again without the field when the host refuses it, so the panel is not lost", () => {
    const seen: object[] = [];
    const strict = (p: Record<string, unknown>) => {
      if ("extra" in p) throw new Error('navPanel: unknown field "extra"');
      seen.push(p);
    };
    expect(registerWithOptionalField(strict, panel, field)).toBe("without");
    expect(seen).toEqual([panel]);
  });

  it("still throws when the host refuses the panel without the field too", () => {
    expect(() => registerWithOptionalField(() => { throw new Error("no"); }, panel, field)).toThrow("no");
  });
});
