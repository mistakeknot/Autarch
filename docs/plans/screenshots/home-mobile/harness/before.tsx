import { createRoot } from "react-dom/client";
import { AsksPanel } from "/tmp/hm-before/integrations/bb-plugin-autarch/ui/asks";
import { asksData } from "./fixtures";

const NOW = Date.parse("2026-10-08T00:00:00Z");
createRoot(document.getElementById("root")!).render(
  <div>
    <nav className="flex gap-3 border-b border-border px-4 py-2 text-sm">
      {["Queue", "Vizier", "Map", "Settings"].map((t) => <button key={t} type="button" className="underline-offset-2 hover:underline">{t}</button>)}
      <button type="button" className="ml-auto text-muted-foreground">Classic tabs</button>
      <button type="button" className="text-muted-foreground">Todos</button>
    </nav>
    <section className="border-b border-border">
      <h2 className="px-4 pt-3 text-xs font-semibold uppercase text-muted-foreground">Needs you now</h2>
      <AsksPanel data={asksData as never} nowMs={NOW} hideHeld onPick={() => {}} onOpen={() => {}} />
    </section>
  </div>,
);
