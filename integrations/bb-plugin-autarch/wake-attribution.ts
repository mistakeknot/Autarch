/** Closed v1 host wake taxonomy. Attribution never comes from message text. */
export type WakeCause = "card_pick" | "merge" | "review" | "timer" | "message" | "workflow" | "retry" | "restart" | "interaction" | "unknown";
export interface WakeAttribution {
  wakeCause: WakeCause;
  wakeRef?: string;
}
