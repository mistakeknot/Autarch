/**
 * Registers a panel with one optional host field. A host that does not know the field may refuse the whole registration
 * (the pinned SDK's collector throws `unknown field`), which would lose the panel, so a refusal retries without the field.
 */
export function registerWithOptionalField<T extends object>(register: (panel: T) => void, panel: T, field: Record<string, unknown>): "with" | "without" {
  try {
    register({ ...panel, ...field });
    return "with";
  } catch {
    register(panel);
    return "without";
  }
}
