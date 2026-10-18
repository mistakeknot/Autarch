import { createRoot } from "react-dom/client";
import { AsksPanel } from "../../../../../integrations/bb-plugin-autarch/ui/asks";
import { CatchupPanel } from "../../../../../integrations/bb-plugin-autarch/ui/catchup";
import { NoticeBanner } from "../../../../../integrations/bb-plugin-autarch/ui/notices";
import { HomeTabs } from "../../../../../integrations/bb-plugin-autarch/ui/tabs";
import { WaitingStrip } from "../../../../../integrations/bb-plugin-autarch/ui/waiting";
import { asksData, catchupItems, waiting } from "./fixtures";

const NOW = Date.parse("2026-10-08T00:00:00Z");
const H = "px-2 pt-2 text-xs font-semibold uppercase text-muted-foreground sm:px-4 sm:pt-3";
createRoot(document.getElementById("root")!).render(
  <div>
    <HomeTabs classic={false} waiting={waiting.total} onOpen={() => {}} onToggleClassic={() => {}} onTodos={() => {}} />
    <WaitingStrip waiting={waiting as never} onJump={() => {}} />
    <div className="p-2 sm:p-4"><NoticeBanner notices={waiting.noticeItems} suspended onAcknowledge={() => {}} /></div>
    <section className="border-b border-border">
      <h2 className={H}>Needs you now (4)</h2>
      <AsksPanel data={asksData as never} nowMs={NOW} hideHeld onPick={() => {}} onOpen={() => {}} />
    </section>
    <section className="border-b border-border">
      <h2 className={H}>Since you left (3)</h2>
      <CatchupPanel items={catchupItems as never} expanded={new Set()} visible={new Set(["failed:41", "note:7"])} result={null} onToggle={() => {}} onOverride={() => {}} onVisibility={() => {}} onMarkAll={() => {}} onMarkOne={() => {}} />
    </section>
  </div>,
);
