// The project filter on the Queue (bead mk-sufr9): a button that opens a searchable list of every project with open
// cards, each with a tick. mk ticks projects out and in; the card lists show the ticked ones. No motion: the list
// appears and goes at once, since mk opens it often and reads what is behind it.
import { useCallback, useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { ActionButton, buttonClass } from "./buttons.js";
import { hiddenText, loadExcluded, saveExcluded, toggleProject, type ProjectChoice, type StorageGetter } from "./projectfilterstore.js";

const deviceStorage: StorageGetter = () => (typeof localStorage === "undefined" ? null : localStorage);

/** The choice, kept on this device. */
export function useProjectFilter(): { excluded: ReadonlySet<string>; setExcluded: (next: ReadonlySet<string>) => void } {
  const [excluded, set] = useState<ReadonlySet<string>>(() => loadExcluded(deviceStorage));
  const setExcluded = useCallback((next: ReadonlySet<string>) => {
    set(next);
    saveExcluded(deviceStorage, next);
  }, []);
  return { excluded, setExcluded };
}

export function ProjectFilterBar({ choices, excluded, hidden, onChange }: {
  choices: readonly ProjectChoice[];
  excluded: ReadonlySet<string>;
  /** How many cards in the lists below the filter hides right now. */
  hidden: number;
  onChange: (next: ReadonlySet<string>) => void;
}) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  // The active row is held by project name, not position, so a list that changes under it cannot move it to another project.
  const [activeName, setActiveName] = useState<string | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const input = useRef<HTMLInputElement>(null);

  const needle = query.trim().toLowerCase();
  const shown = needle ? choices.filter((c) => c.name.toLowerCase().includes(needle)) : choices;
  const active = activeName === null ? -1 : shown.findIndex((c) => c.name === activeName);
  const optionId = (i: number) => `${id}-option-${i}`;

  const close = (refocus: boolean) => {
    setOpen(false);
    setQuery("");
    setActiveName(null);
    if (refocus) trigger.current?.focus();
  };
  const reset = () => onChange(new Set());
  // The button that was pressed leaves the page with the reset, so focus goes where mk can carry on.
  const resetInPopup = () => { reset(); input.current?.focus(); };
  const resetFromNote = () => { reset(); trigger.current?.focus(); };

  useEffect(() => {
    if (open) input.current?.focus();
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      if (root.current && e.target instanceof Node && !root.current.contains(e.target)) close(false);
    };
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, [open]);
  useEffect(() => {
    if (open && active >= 0) document.getElementById(`${id}-option-${active}`)?.scrollIntoView?.({ block: "nearest" });
  }, [open, active, id]);

  if (choices.length === 0) return null;

  const included = choices.filter((c) => !excluded.has(c.name)).length;
  const label = excluded.size === 0 ? "All projects" : `${included} of ${choices.length} projects`;

  const onListKey = (e: KeyboardEvent) => {
    const last = shown.length - 1;
    const go = (to: number) => { e.preventDefault(); const c = shown[to]; if (c) setActiveName(c.name); };
    if (e.key === "ArrowDown") go(active >= last ? 0 : active + 1);
    else if (e.key === "ArrowUp") go(active <= 0 ? last : active - 1);
    else if (e.key === "Home") go(0);
    else if (e.key === "End") go(last);
    else if (e.key === "Enter") {
      e.preventDefault();
      const c = shown[active];
      if (c) onChange(toggleProject(excluded, c.name));
    }
  };
  // Escape closes from anywhere in the popup, the reset button included.
  const onPopupKey = (e: KeyboardEvent) => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    close(true);
  };

  return (
    <div ref={root} className="relative flex flex-wrap items-center gap-x-3 px-2 sm:px-4" data-project-filter>
      <button
        ref={trigger}
        type="button"
        className={`${buttonClass("quiet")} min-w-0 max-w-full`}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? `${id}-list` : undefined}
        onClick={() => (open ? close(false) : setOpen(true))}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" && !open) {
            e.preventDefault();
            setOpen(true);
          }
        }}
        data-project-filter-trigger
      >
        <span aria-hidden>▾</span>
        <span>{label}</span>
      </button>
      {hidden > 0 ? (
        <p role="status" className="m-0 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground" data-project-filter-hidden>
          <span>{hiddenText(hidden)}</span>
          <ActionButton tone="quiet" onClick={resetFromNote}>All projects</ActionButton>
        </p>
      ) : null}
      {open ? (
        <div className="absolute left-2 right-2 top-full z-20 rounded-md border border-border bg-background p-2 shadow-md sm:left-4 sm:right-auto sm:w-80" data-project-filter-popup onKeyDown={onPopupKey}>
          <input
            ref={input}
            type="text"
            role="combobox"
            aria-expanded
            aria-controls={`${id}-list`}
            aria-autocomplete="list"
            aria-label="Find a project"
            aria-activedescendant={active >= 0 && active < shown.length ? optionId(active) : undefined}
            placeholder="Find a project"
            autoComplete="off"
            value={query}
            className="mb-2 min-h-11 w-full rounded-md border border-border bg-background px-2 text-sm sm:min-h-8"
            onChange={(e) => { setQuery(e.target.value); setActiveName(choices.find((c) => c.name.toLowerCase().includes(e.target.value.trim().toLowerCase()))?.name ?? null); }}
            onKeyDown={onListKey}
          />
          {excluded.size > 0 ? (
            <ActionButton tone="quiet" className="mb-1 w-full justify-start" onClick={resetInPopup} data-project-filter-all>All projects</ActionButton>
          ) : null}
          <ul id={`${id}-list`} role="listbox" aria-multiselectable="true" aria-label="Projects" className="m-0 max-h-72 list-none overflow-y-auto p-0">
            {shown.map((c, i) => {
              const on = !excluded.has(c.name);
              return (
                <li
                  key={c.name}
                  id={optionId(i)}
                  role="option"
                  aria-selected={on}
                  className={`flex min-h-11 cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-state-hover sm:min-h-8 ${i === active ? "bg-state-hover ring-2 ring-ring" : ""}`}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => { setActiveName(c.name); onChange(toggleProject(excluded, c.name)); }}
                >
                  <span aria-hidden className={`flex size-5 shrink-0 items-center justify-center rounded-sm border text-xs ${on ? "border-primary bg-primary text-primary-foreground" : "border-input"}`}>{on ? "✓" : ""}</span>
                  <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">{c.name}</span>
                  <span className="shrink-0 text-xs text-muted-foreground">{c.count}</span>
                </li>
              );
            })}
          </ul>
          {shown.length === 0 ? <p className="m-0 px-2 py-2 text-sm text-muted-foreground" data-project-filter-nomatch>{`No project matches "${query.trim()}".`}</p> : null}
        </div>
      ) : null}
    </div>
  );
}
