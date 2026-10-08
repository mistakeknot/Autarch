// One look for every Home action button: a visible outline, hover and keyboard-focus states, a distinct
// selected state, a marked recommended option, and a different treatment for destructive choices. Colours come from
// the host theme tokens (Tokyo Night in the shipped theme); nothing here hard-codes a palette.
import type { ButtonHTMLAttributes } from "react";

export type ButtonTone = "default" | "recommended" | "destructive" | "quiet";

const BASE =
  "inline-flex min-h-8 cursor-pointer items-center justify-center gap-1 rounded-md border px-3 py-1 text-sm font-medium transition-colors " +
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 " +
  "disabled:cursor-not-allowed disabled:opacity-50";

const TONE: Record<ButtonTone, string> = {
  default: "border-input bg-transparent text-foreground hover:bg-state-hover",
  recommended: "border-primary bg-transparent text-primary hover:bg-state-hover",
  destructive: "border-destructive bg-transparent text-destructive hover:bg-destructive/10",
  quiet: "border-transparent bg-transparent text-muted-foreground underline-offset-2 hover:bg-state-hover hover:text-foreground hover:underline",
};
const SELECTED: Record<ButtonTone, string> = {
  default: "border-foreground bg-state-active text-foreground",
  recommended: "border-primary bg-primary text-primary-foreground",
  destructive: "border-destructive bg-destructive text-destructive-foreground",
  quiet: "border-input bg-state-active text-foreground",
};

/** The class string for a button; also used by tests to assert the look without a browser. */
export function buttonClass(tone: ButtonTone = "default", selected = false): string {
  return `${BASE} ${selected ? SELECTED[tone] : TONE[tone]}`;
}

export function ActionButton({ tone = "default", selected = false, className = "", ...rest }: ButtonHTMLAttributes<HTMLButtonElement> & { tone?: ButtonTone; selected?: boolean }) {
  return <button type="button" aria-pressed={selected ? true : undefined} data-tone={tone} data-selected={selected ? "true" : undefined} className={`${buttonClass(tone, selected)} ${className}`.trim()} {...rest} />;
}

/** The small marker next to the recommended option. */
export function RecommendedMark() {
  return <span className="rounded-sm border border-primary px-1 text-xs font-medium text-primary" data-recommended-mark>recommended</span>;
}
