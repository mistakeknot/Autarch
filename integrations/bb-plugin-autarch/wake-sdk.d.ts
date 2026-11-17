import "@get-bb/plugin-sdk";
import type { WakeCause } from "./wake-attribution.js";

// Aleph 0.49.0+aleph.0.10.1 supplies these fields through the host SDK runtime.
// The npm SDK types lag that runtime; older hosts silently drop attribution.
// Forwarding contract: https://github.com/mistakeknot/aleph/pull/230
declare module "@get-bb/plugin-sdk" {
  interface ThreadSendArgs {
    wakeCause: WakeCause;
    wakeRef?: string;
  }
}
