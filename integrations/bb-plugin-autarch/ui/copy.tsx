// Copyable command blocks. The command is one line, shown in a monospace block that scrolls sideways and
// selects normally; the Copy button puts exactly that line on the clipboard: no prompt, no backticks, no trailing space.
import { useRef, useState } from "react";
import { ActionButton } from "./buttons.js";

/** What copyText needs from the browser; injectable so tests can use a fake clipboard and a fake document. */
export interface CopyEnv {
  clipboard?: { writeText(text: string): Promise<void> } | undefined;
  /** The select-all/execCommand fallback. Returns whether the browser accepted the copy. */
  legacyCopy?: ((text: string) => boolean) | undefined;
}

/** The legacy path: a hidden textarea holding the exact text, selected whole, copied with execCommand. */
export function legacyCopyViaDocument(doc: Document): (text: string) => boolean {
  return (text) => {
    const ta = doc.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "0";
    ta.style.left = "-9999px";
    ta.style.opacity = "0";
    doc.body.appendChild(ta);
    try {
      ta.focus();
      ta.select();
      ta.setSelectionRange(0, text.length);
      return doc.execCommand("copy");
    } catch {
      return false;
    } finally {
      doc.body.removeChild(ta);
    }
  };
}

export function browserCopyEnv(): CopyEnv {
  return {
    clipboard: typeof navigator !== "undefined" ? navigator.clipboard : undefined,
    legacyCopy: typeof document !== "undefined" ? legacyCopyViaDocument(document) : undefined,
  };
}

/** Copies the text exactly as given. Tries the async clipboard, then the legacy path. True when one accepted it. */
export async function copyText(text: string, env: CopyEnv = browserCopyEnv()): Promise<boolean> {
  if (env.clipboard) {
    try {
      await env.clipboard.writeText(text);
      return true;
    } catch {
      /* permission denied or insecure context: fall back */
    }
  }
  return env.legacyCopy ? env.legacyCopy(text) : false;
}

export function CommandBlock({ label, command, expectedSha, env, copiedMs = 1500 }: { label: string; command: string; expectedSha?: string | undefined; env?: CopyEnv; copiedMs?: number }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onCopy = async () => {
    const ok = await copyText(command, env);
    setState(ok ? "copied" : "failed");
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setState("idle"), copiedMs);
  };
  return (
    <div className="min-w-0" data-command={label}>
      <div className="mb-1 flex flex-wrap items-baseline gap-x-2 text-xs text-muted-foreground">
        <span className="font-medium">{label}</span>
        {expectedSha ? <span className="min-w-0 [overflow-wrap:anywhere]" data-expected-sha>{`expected sha256 ${expectedSha}`}</span> : null}
      </div>
      <div className="flex min-w-0 items-stretch gap-2">
        {/* The scrolling box holds only the command. Selection (drag or triple-click) covers exactly that text. */}
        <pre className="m-0 min-w-0 flex-1 overflow-x-auto whitespace-pre rounded-md border border-border bg-muted px-3 py-2 font-mono text-xs [overflow-wrap:normal]" style={{ overflowX: "auto", whiteSpace: "pre" }} data-command-box>
          <code data-command-text>{command}</code>
        </pre>
        <ActionButton tone={state === "failed" ? "destructive" : "default"} onClick={() => void onCopy()} aria-label={`Copy ${label} command`} data-copy-button data-copy-state={state}>
          {state === "copied" ? "Copied" : state === "failed" ? "Copy failed: select it" : "Copy"}
        </ActionButton>
      </div>
    </div>
  );
}
