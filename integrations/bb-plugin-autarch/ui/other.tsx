// The Other box on every card: mk's own words. "Ask / note" posts a comment and wakes the owner and never rules;
// "Answer with this" records the words as a custom pick. The text is plain text, trimmed, capped at 20000 characters.
// The server enforces the same cap; the counter here is a courtesy. Images (paste or attach) travel with the text;
// the server checks type, size and count again, and an image alone (no text) is not sent.
import { useRef, useState } from "react";
import { ActionButton } from "./buttons.js";

export const OTHER_MAX = 20000;

export const OTHER_IMG_MAX_COUNT = 4;
export const OTHER_IMG_MAX_BYTES = 5 * 1024 * 1024;
export const OTHER_IMG_MAX_TOTAL = 12 * 1024 * 1024;
const IMG_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];

/** One image as the server takes it. */
export type OtherImage = { mime: string; data_base64: string };
type Staged = OtherImage & { id: number; name: string; bytes: number; url: string };

function toBase64(buf: ArrayBuffer): string {
  const u = new Uint8Array(buf);
  let bin = "";
  for (let i = 0; i < u.length; i += 0x8000) bin += String.fromCharCode(...u.subarray(i, i + 0x8000));
  return btoa(bin);
}

/** A read or a send that never settles must not leave the buttons disabled for good (bead mk-1poq7). */
export const IMG_READ_TIMEOUT_MS = 15_000;
export const SEND_TIMEOUT_MS = 60_000;

/** Settles with p, or with `onTimeout()` after ms, whichever comes first; the timer never outlives the race. */
export function settleWithin<T>(p: Promise<T>, ms: number, onTimeout: () => T): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => resolve(onTimeout()), ms);
    p.then(
      (v) => { clearTimeout(timer); resolve(v); },
      (e) => { clearTimeout(timer); reject(e); },
    );
  });
}

export type OtherOutcome = { ok: boolean; error?: string };

/** The trimmed text, or null when it is empty or over the cap (the buttons stay disabled). */
export function otherText(raw: string): string | null {
  const t = raw.trim();
  const n = Array.from(t).length; // code points, as the service counts them
  return n === 0 || n > OTHER_MAX ? null : t;
}

/** What Ask / note and the answer button do; an AskCard folds this into its "How this works" disclosure. */
export function otherHelpText(withAnswer: boolean): string {
  return `Ask / note sends a comment and wakes the owner; it never rules or closes. Images go to the owner with your words; an image alone is not sent.${withAnswer ? " The answer button records your words as your ruling." : ""}`;
}

export function OtherBox({
  onNote,
  onAnswer,
  answerDisabledReason,
  label = "Other",
  showHelp = true,
}: {
  onNote: (text: string, images?: OtherImage[]) => Promise<OtherOutcome | void> | void;
  /** Absent: the card cannot be ruled here (no pick buttons), so only Ask / note is offered. */
  onAnswer?: ((text: string, images?: OtherImage[]) => Promise<OtherOutcome | void> | void) | undefined;
  /** When set, "Answer with this" is shown disabled with this reason. */
  answerDisabledReason?: string | undefined;
  label?: string;
  /** False when the caller shows the help text itself. */
  showHelp?: boolean;
}) {
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [imgs, setImgsState] = useState<Staged[]>([]);
  const imgsRef = useRef<Staged[]>([]);
  const setImgs = (v: Staged[]) => {
    imgsRef.current = v;
    setImgsState(v);
  };
  const [imgErr, setImgErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(0);
  const nextId = useRef(1);
  const chain = useRef<Promise<void>>(Promise.resolve());
  // One read at a time, each against the images as they are when it finishes, so a send, a removal or a second
  // paste cannot lose or resurrect an image. Send stays disabled while any read is pending.
  const addNow = async (files: File[]) => {
    setImgErr(null);
    for (const f of files) {
      const cur = imgsRef.current;
      if (!IMG_TYPES.includes(f.type)) { setImgErr(`${f.name || "That file"} is not a PNG, JPEG, GIF or WebP image.`); continue; }
      if (cur.length >= OTHER_IMG_MAX_COUNT) { setImgErr(`At most ${OTHER_IMG_MAX_COUNT} images.`); break; }
      if (f.size > OTHER_IMG_MAX_BYTES) { setImgErr(`${f.name || "That image"} is over ${OTHER_IMG_MAX_BYTES / (1024 * 1024)} MB.`); continue; }
      if (cur.reduce((a, x) => a + x.bytes, 0) + f.size > OTHER_IMG_MAX_TOTAL) { setImgErr(`Images total over ${OTHER_IMG_MAX_TOTAL / (1024 * 1024)} MB.`); break; }
      let item: Staged;
      try {
        const buf = await f.arrayBuffer();
        item = { id: nextId.current++, mime: f.type, data_base64: toBase64(buf), name: f.name || "pasted image", bytes: f.size, url: URL.createObjectURL(f) };
      } catch {
        setImgErr(`${f.name || "That image"} could not be read.`);
        continue;
      }
      const now = imgsRef.current;
      if (now.length >= OTHER_IMG_MAX_COUNT || now.reduce((a, x) => a + x.bytes, 0) + item.bytes > OTHER_IMG_MAX_TOTAL) {
        URL.revokeObjectURL(item.url);
        setImgErr(`At most ${OTHER_IMG_MAX_COUNT} images, ${OTHER_IMG_MAX_TOTAL / (1024 * 1024)} MB in all.`);
        break;
      }
      setImgs([...now, item]);
    }
  };
  const addFiles = (files: File[]) => {
    if (busy) return; // a send in flight owns the staged images; anything added now would be cleared unsent
    setLoading((n) => n + 1);
    chain.current = chain.current
      .then(() => settleWithin(addNow(files), IMG_READ_TIMEOUT_MS, () => setImgErr("An image took too long to read and was skipped. Try attaching it again.")))
      .catch(() => {})
      .finally(() => setLoading((n) => n - 1));
  };
  const dropImg = (id: number) => {
    const gone = imgsRef.current.find((x) => x.id === id);
    if (gone) URL.revokeObjectURL(gone.url);
    setImgs(imgsRef.current.filter((x) => x.id !== id));
  };
  const t = otherText(raw);
  const count = Array.from(raw.trim()).length;
  const over = count > OTHER_MAX;
  const run = async (f: (text: string, images?: OtherImage[]) => Promise<OtherOutcome | void> | void, okText: string, clear: boolean) => {
    if (t === null || busy || loading > 0) return;
    setBusy(true);
    setMsg(null);
    try {
      const sent = f(t, ...(imgs.length > 0 ? [imgs.map(({ mime, data_base64 }) => ({ mime, data_base64 }))] : []));
      const r = await settleWithin(Promise.resolve(sent), SEND_TIMEOUT_MS, (): OtherOutcome => ({ ok: false, error: "no reply yet; it may still go through. Press the button again to retry (a retry never posts twice)" }));
      if (r && r.ok === false) setMsg({ ok: false, text: r.error ?? "not sent" });
      else {
        setMsg({ ok: true, text: okText });
        if (clear) {
          setRaw("");
          imgs.forEach((x) => URL.revokeObjectURL(x.url));
          setImgs([]);
        }
      }
    } catch (e) {
      setMsg({ ok: false, text: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="mt-3 min-w-0" data-other>
      <label className="block text-xs font-medium text-muted-foreground">
        {label}
        <textarea
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          onPaste={(e) => {
            const files = Array.from(e.clipboardData?.files ?? []).filter((f) => f.type.startsWith("image/"));
            if (files.length > 0) {
              e.preventDefault();
              addFiles(files);
            }
          }}
          rows={3}
          className="mt-1 block w-full min-w-0 resize-y rounded-md border border-input bg-transparent p-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          placeholder="Your own words: a question, a note, or a different answer. Plain text; paste or attach images."
          data-other-text
        />
      </label>
      {imgs.length > 0 ? (
        <ul className="mt-1 flex flex-wrap gap-2" data-other-images>
          {imgs.map((x) => (
            <li key={x.id} className="relative">
              <img src={x.url} alt={x.name} className="h-16 w-16 rounded-md border border-input object-cover" />
              <button type="button" onClick={() => dropImg(x.id)} aria-label={`Remove ${x.name}`} className="absolute -right-1 -top-1 rounded-full border border-input bg-background px-1 text-xs leading-4" data-other-image-remove>×</button>
            </li>
          ))}
        </ul>
      ) : null}
      {imgErr ? <p role="alert" className="mt-1 text-xs font-medium text-destructive" data-other-image-error>{imgErr}</p> : null}
      <div className="mt-1 flex flex-wrap items-center gap-2">
        <label className="cursor-pointer text-xs text-muted-foreground underline">
          Attach image
          <input type="file" accept={IMG_TYPES.join(",")} multiple disabled={busy} className="sr-only" data-other-attach onChange={(e) => { const fs = Array.from(e.target.files ?? []); e.target.value = ""; addFiles(fs); }} />
        </label>
        <span className={`text-xs ${over ? "font-medium text-destructive" : "text-muted-foreground"}`} data-other-counter aria-live="polite">{`${count} / ${OTHER_MAX}`}</span>
        <ActionButton disabled={t === null || busy || loading > 0} onClick={() => void run(onNote, "Sent. The owner is woken; nothing was ruled or closed.", true)} data-other-note>Ask / note</ActionButton>
        {onAnswer || answerDisabledReason ? (
          <ActionButton
            disabled={t === null || busy || loading > 0 || !onAnswer || answerDisabledReason !== undefined}
            title={answerDisabledReason}
            onClick={() => onAnswer && void run(onAnswer, "Recorded as your answer.", false)}
            data-other-answer
          >
            Answer with this
          </ActionButton>
        ) : null}
      </div>
      {busy || loading > 0 ? <p className="mt-1 text-xs text-muted-foreground" data-other-wait>{busy ? "Sending..." : "Reading an image..."}</p> : null}
      {answerDisabledReason ? <p className="mt-1 text-xs text-muted-foreground" data-other-answer-reason>{answerDisabledReason}</p> : null}
      {showHelp ? <p className="mt-1 text-xs text-muted-foreground" data-other-help>{otherHelpText(!!onAnswer)}</p> : null}
      {msg ? <p role={msg.ok ? "status" : "alert"} className={`mt-1 text-xs ${msg.ok ? "text-muted-foreground" : "font-medium text-destructive"}`} data-other-result={msg.ok ? "ok" : "error"}>{msg.text}</p> : null}
    </div>
  );
}
