import { createRoot } from "react-dom/client";
import { YourMovePanel } from "../../../../../integrations/bb-plugin-autarch/ui/yourmove";
import { AsksPanel } from "../../../../../integrations/bb-plugin-autarch/ui/asks";
import { BlocksPanel } from "../../../../../integrations/bb-plugin-autarch/ui/blocks";
import { asksData, movesData, unboundQueue } from "./fixtures";

const noop = async () => ({ ok: true as const });
const handlers = { onCheck: noop, onClaim: noop, onSkip: noop, onNote: noop };
const scene = location.hash.slice(1) || "overview";
const NOW = Date.parse("2026-10-07T00:00:00Z");
const asks = <AsksPanel data={asksData as never} nowMs={NOW} onPick={() => {}} onOpen={() => {}} onNote={noop} onDismiss={() => {}} />;
const page =
  scene === "unbound" ? <BlocksPanel data={unboundQueue as never} nowMs={NOW} onPick={() => {}} onOpen={() => {}} onNote={noop} /> : (
    <>
      <YourMovePanel data={movesData as never} handlers={handlers} />
      {asks}
    </>
  );
createRoot(document.getElementById("root")!).render(<div style={{ maxWidth: 1100, margin: "0 auto" }}>{page}</div>);
