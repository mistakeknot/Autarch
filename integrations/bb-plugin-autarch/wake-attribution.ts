/** Closed v1 host wake taxonomy. Attribution never comes from message text. */
export type WakeCause = "card_pick" | "merge" | "review" | "timer" | "message" | "workflow" | "retry" | "restart" | "interaction" | "unknown";
// Local send contract: field names checked against Aleph packages/domain/src/wake.ts.
// The pinned npm SDK does not export the host's ThreadSendArgs; this is not an augmentation.
// Older hosts may silently drop attribution.
export interface WakeAttribution {
  wakeCause: WakeCause;
  wakeRef?: string;
}
