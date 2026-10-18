// The same invented data rendered by the a6621fc tree (a worktree at /tmp/hu-before).
import { createRoot } from "react-dom/client";
import { AsksPanel } from "/tmp/hu-before/integrations/bb-plugin-autarch/ui/asks";
import { CatchupPanel } from "/tmp/hu-before/integrations/bb-plugin-autarch/ui/catchup";
import { HomeTabs } from "/tmp/hu-before/integrations/bb-plugin-autarch/ui/tabs";
import { asksData, catchupItems, waiting } from "./fixtures";

const NOW = Date.parse("2026-10-08T00:00:00Z");
const H = "px-2 pt-2 text-xs font-semibold uppercase text-muted-foreground sm:px-4 sm:pt-3";
// Before: the adoption notice was a pinned catch-up row at the bottom, and the sidebar badge added catch-up to the asks.
const items = [...catchupItems, { item: "pinned:vizier-adopted:9", kind: "delegated", at: "2026-10-07T23:00:00Z", text: waiting.noticeItems[0]!.text }];
createRoot(document.getElementById("root")!).render(
  <div>
    <HomeTabs classic={false} onOpen={() => {}} onToggleClassic={() => {}} onTodos={() => {}} />
    <section className="border-b border-border">
      <h2 className={H}>Needs you now</h2>
      <AsksPanel data={asksData as never} nowMs={NOW} hideHeld onPick={() => {}} onOpen={() => {}} />
    </section>
    <section className="border-b border-border">
      <h2 className={H}>Since you left</h2>
      <CatchupPanel items={items as never} expanded={new Set()} onToggle={() => {}} onOverride={() => {}} onVisibility={() => {}} onMarkAll={() => {}} />
    </section>
  </div>,
);
