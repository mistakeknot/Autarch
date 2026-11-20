import { useLayoutEffect, useId, useRef, useState } from "react";
import { ActionButton } from "./buttons.js";

export type LightboxImage = { id: string; caption: string; url: string };

/** Native modality keeps scoped plugin styles while providing top-layer focus containment and inert background. */
export function EvidenceLightbox({ images, current, onChange, onClose, trigger }: { trigger: HTMLButtonElement | null; images: LightboxImage[]; current: string; onChange: (id: string) => void; onClose: () => void }) {
 const dialog = useRef<HTMLDialogElement>(null), scroller = useRef<HTMLDivElement>(null);

 const title = useId();
 const [natural, setNatural] = useState(false);
 const start = useRef<{ x: number; y: number } | null>(null);
 const index = images.findIndex(i => i.id === current), image = images[index];
 useLayoutEffect(() => {
  const node = dialog.current;
  node?.showModal(); node?.querySelector<HTMLButtonElement>('[aria-label="Close image preview"]')?.focus();
  return () => { node?.close(); trigger?.focus(); };
 }, [trigger]);
 useLayoutEffect(() => { if (scroller.current) { scroller.current.scrollTop = 0; scroller.current.scrollLeft = 0; } }, [current]);
 const move = (delta: number) => { const next = images[(index + delta + images.length) % images.length]; if (next) { setNatural(false); onChange(next.id); } };
 useLayoutEffect(() => {
  const keys = (e: KeyboardEvent) => {
   if ((e.key === "ArrowLeft" || e.key === "ArrowRight") && (e.altKey || e.metaKey || e.ctrlKey)) return;
   // Controls rely on native key defaults: this capture runs before React root listeners.
   // All remaining keys stay contained even when modifiers are held.
   e.stopPropagation();
   if (e.key === "Escape") { e.preventDefault(); onClose(); }
   else if (e.key === "ArrowLeft" || e.key === "ArrowRight") { e.preventDefault(); move(e.key === "ArrowLeft" ? -1 : 1); }
  };
  document.addEventListener("keydown", keys, true);
  return () => document.removeEventListener("keydown", keys, true);
 });
 if (!image) return null;
 return <dialog ref={dialog} tabIndex={-1} aria-labelledby={title} className="m-auto h-[calc(100dvh-2rem)] max-h-dvh w-[calc(100vw-2rem)] max-w-full border border-border bg-popover p-0 text-popover-foreground backdrop:bg-background/80" onCancel={e => { e.preventDefault(); e.stopPropagation(); onClose(); }} onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
  <div className="flex h-full min-h-0 flex-col" data-evidence-lightbox>
   <header className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border p-2">
    <h2 id={title} className="m-0 min-w-0 flex-1 text-sm font-medium [overflow-wrap:anywhere]">{image.caption}</h2>
    <span className="text-xs text-muted-foreground" role="status">{index + 1} / {images.length}</span>
    <ActionButton className="min-h-11 sm:!min-h-11" onClick={() => setNatural(v => !v)}>{natural ? "Fit to viewport" : "Natural size"}</ActionButton>
    <ActionButton className="min-h-11 sm:!min-h-11" aria-label="Close image preview" onClick={onClose}>Close</ActionButton>
   </header>
   <div ref={scroller} className={`min-h-0 flex-1 overflow-auto overscroll-contain p-2 ${natural ? "" : "flex items-center justify-center"}`} data-evidence-viewport style={{ touchAction: natural ? "auto" : "pan-y pinch-zoom" }} onClick={e => { if (e.target === e.currentTarget) onClose(); }} onTouchStart={e => { const t = e.touches[0]; start.current = e.touches.length === 1 && t ? { x:t.clientX,y:t.clientY } : null; }} onTouchCancel={() => { start.current=null; }} onTouchMove={e => { if(e.touches.length!==1)start.current=null; }} onTouchEnd={e => {
    const t=e.changedTouches[0], from=start.current; start.current=null;
    // Natural-size and page-zoomed gestures pan instead of changing the image.
    const pageZoomed = (window.visualViewport?.scale ?? 1) > 1;
    if (!natural && !pageZoomed && t && from && Math.abs(t.clientX-from.x)>48 && Math.abs(t.clientX-from.x)>Math.abs(t.clientY-from.y)*1.5) move(t.clientX<from.x?1:-1);
   }}>
    <img draggable={false} src={image.url} alt={image.caption} onClick={() => { dialog.current?.focus(); setNatural(v => !v); }} className={natural ? "block max-w-none cursor-zoom-out" : "block max-h-full max-w-full cursor-zoom-in object-contain"} data-evidence-full-image />
   </div>
   <footer className="flex shrink-0 items-center justify-between gap-2 border-t border-border p-2">
    <ActionButton className="min-h-11 sm:!min-h-11" disabled={images.length<2} onClick={() => move(-1)} aria-label="Previous image">Previous</ActionButton>
    <span className="text-xs text-muted-foreground">{natural ? "Natural size · scroll to inspect" : "Fit · tap or click image to enlarge"}</span>
    <ActionButton className="min-h-11 sm:!min-h-11" disabled={images.length<2} onClick={() => move(1)} aria-label="Next image">Next</ActionButton>
   </footer>
  </div>
 </dialog>;
}
