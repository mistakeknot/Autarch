import type { HoldEnd } from "../holdend.js";
import { holdEndText } from "./holdend.js";

/** The line under a hold's reason that says when it ends: "until Nov 14", "until PR 114 merges", or "no end set" for an old hold. Wraps, never clips. */
export function HoldEndLine({ held, as: Tag = "span", className = "block text-xs [overflow-wrap:anywhere]" }: { held: { end?: HoldEnd | null }; as?: "span" | "p"; className?: string }) {
  return <Tag className={className} data-hold-end>{holdEndText(held, Date.now())}</Tag>;
}
