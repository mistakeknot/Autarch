import { createRoot } from "react-dom/client";
import { AsksPanel } from "../../../../../integrations/bb-plugin-autarch/ui/asks";
import { HomeTabs } from "../../../../../integrations/bb-plugin-autarch/ui/tabs";
import { asksData } from "./fixtures";

const NOW = Date.parse("2026-10-08T00:00:00Z");
createRoot(document.getElementById("root")!).render(
  <div>
    <HomeTabs classic={false} onOpen={() => {}} onToggleClassic={() => {}} onTodos={() => {}} />
    <section className="border-b border-border">
      <h2 className="px-2 pt-2 text-xs font-semibold uppercase text-muted-foreground sm:px-4 sm:pt-3">Needs you now</h2>
      <AsksPanel data={asksData as never} nowMs={NOW} hideHeld onPick={() => {}} onOpen={() => {}} />
    </section>
  </div>,
);
