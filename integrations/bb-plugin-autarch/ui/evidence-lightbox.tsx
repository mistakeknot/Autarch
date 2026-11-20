import { useEffect, useId, useRef, useState } from "react";
import { ActionButton } from "./buttons.js";

export type LightboxImage = { id: string; caption: string; url: string };

/** Native modality keeps scoped plugin styles while providing top-layer focus containment and inert background. */
export function EvidenceLightbox({ images, current, onChange, onClose }: { images: LightboxImage[]; current: string; onChange: (id: string) => void; onClose: () => void }) {
 const dialog = useRef<HTMLDialogElement>(null), close = useRef<HTMLButtonElement>(null), scroller = useRef<HTMLDivElement>(null);
 const origin = useRef(document.activeElement as HTMLElement | null);
 const title = useId();
 const [natural, setNatural] = useState(false);
 const start = useRef<{ x: number; y: number } | null>(null);
 const index = images.findIndex(i => i.id === current), image = images[index];
 useEffect(() => {
  const node = dialog.current;
  node?.showModal(); close.current?.focus();
  return () => { node?.close(); origin.current?.focus(); };
 }, []);
 useEffect(() => { setNatural(false); if (scroller.current) { scroller.current.scrollTop = 0; scroller.current.scrollLeft = 0; } }, [current]);
 const move = (delta: number) => { const next = images[(index + delta + images.length) % images.length]; if (next) onChange(next.id); };
 if (!image) return null;
 return <dialog ref={dialog} aria-labelledby={title} className="m-auto h-[calc(100dvh-2rem)] max-h-dvh w-[calc(100vw-2rem)] max-w-full border border-border bg-popover p-0 text-popover-foreground backdrop:bg-background/80" onCancel={e => { e.preventDefault(); onClose(); }} onClick={e => { if (e.target === e.currentTarget) onClose(); }} onKeyDown={e => {
  if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onClose(); }
  else if (e.key === "ArrowLeft" || e.key === "ArrowRight") { e.preventDefault(); e.stopPropagation(); move(e.key === "ArrowLeft" ? -1 : 1); }
 }}>
  <div className="flex h-full min-h-0 flex-col" data-evidence-lightbox>
   <header className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border p-2">
    <h2 id={title} className="m-0 min-w-0 flex-1 text-sm font-medium [overflow-wrap:anywhere]">{image.caption}</h2>
    <span className="text-xs text-muted-foreground" role="status">{index + 1} / {images.length}</span>
    <ActionButton className="min-h-11 sm:min-h-11" aria-pressed={natural} onClick={() => setNatural(v => !v)}>{natural ? "Fit to viewport" : "Natural size"}</ActionButton>
    <button ref={close} type="button" className="min-h-11 rounded-md px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label="Close image preview" onClick={onClose}>Close</button>
   </header>
   <div ref={scroller} className={`min-h-0 flex-1 overflow-auto p-2 ${natural ? "" : "flex items-center justify-center"}`} data-evidence-viewport style={{ touchAction: natural ? "auto" : "pan-y" }} onClick={e => { if (e.target === e.currentTarget) onClose(); }} onTouchStart={e => { const t = e.touches[0]; start.current = t ? { x:t.clientX,y:t.clientY } : null; }} onTouchEnd={e => {
    const t=e.changedTouches[0], from=start.current; start.current=null;
    // Natural-size gestures pan the image instead of changing it.
    if (!natural && t && from && Math.abs(t.clientX-from.x)>48 && Math.abs(t.clientX-from.x)>Math.abs(t.clientY-from.y)*1.5) move(t.clientX<from.x?1:-1);
   }}>
    <img draggable={false} src={image.url} alt={image.caption} onClick={() => setNatural(v => !v)} className={natural ? "block max-w-none cursor-zoom-out" : "block max-h-full max-w-full cursor-zoom-in object-contain"} data-evidence-full-image />
   </div>
   <footer className="flex shrink-0 items-center justify-between gap-2 border-t border-border p-2">
    <ActionButton className="min-h-11 sm:min-h-11" disabled={images.length<2} onClick={() => move(-1)} aria-label="Previous image">Previous</ActionButton>
    <span className="text-xs text-muted-foreground">{natural ? "Natural size · scroll to inspect" : "Fit · click image to enlarge"}</span>
    <ActionButton className="min-h-11 sm:min-h-11" disabled={images.length<2} onClick={() => move(1)} aria-label="Next image">Next</ActionButton>
   </footer>
  </div>
 </dialog>;
}
