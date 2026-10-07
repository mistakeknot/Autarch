import { createRoot } from "react-dom/client";
import { AsksPanel } from "/tmp/ym-before/integrations/bb-plugin-autarch/ui/asks";
import { BlocksPanel } from "/tmp/ym-before/integrations/bb-plugin-autarch/ui/blocks";
import { asksData, unboundQueue } from "./fixtures";

const scene = location.hash.slice(1) || "overview";
const NOW = Date.parse("2026-10-07T00:00:00Z");
const page =
  scene === "unbound" ? <BlocksPanel data={unboundQueue as never} nowMs={NOW} onPick={() => {}} onOpen={() => {}} /> : <AsksPanel data={asksData as never} nowMs={NOW} onPick={() => {}} onOpen={() => {}} />;
createRoot(document.getElementById("root")!).render(<div style={{ maxWidth: 1100, margin: "0 auto" }}>{page}</div>);
