export const LENSES = ["attention", "allocation", "dependencies", "neglect"] as const;
export type Lens = (typeof LENSES)[number];

/** A pace-layer by ecosystem frame; the real map is not built yet. */
export function MapPlaceholder({ lens, onLens }: { lens: Lens; onLens: (l: Lens) => void }) {
  return (
    <div className="p-4 text-sm">
      <p className="text-xs uppercase text-muted-foreground">Map (placeholder)</p>
      <div role="tablist" className="mt-2 flex gap-2">
        {LENSES.map((l) => (
          <button key={l} role="tab" type="button" aria-selected={l === lens} className={l === lens ? "font-medium underline" : ""} onClick={() => onLens(l)}>{l}</button>
        ))}
      </div>
      <p className="mt-3 text-muted-foreground">Pace layers by ecosystem, viewed through the {lens} lens. Nothing is plotted yet.</p>
    </div>
  );
}
