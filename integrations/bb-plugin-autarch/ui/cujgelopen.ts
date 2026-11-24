import type { CujgelLink } from "../cujgel.js";

// Set only after the installed plugin ID is confirmed. No availability probe.
export const CUJGEL_PLUGIN_ID: string | null = null;

export type CujgelOpenTarget = { status: "available"; href: string } | { status: "missing"; href: null };
const ID = /^[A-Za-z0-9][A-Za-z0-9_-]{0,127}(?![\s\S])/;

/** The route is derived solely from snapshot IDs and the configured plugin ID. */
export function openTarget(link: Pick<CujgelLink, "project" | "comparison_id">, pluginId: string | null = CUJGEL_PLUGIN_ID): CujgelOpenTarget {
  if (!pluginId || !ID.test(link.project) || !ID.test(link.comparison_id)) return { status: "missing", href: null };
  return { status: "available", href: `/plugins/${encodeURIComponent(pluginId)}/journeys/${encodeURIComponent(link.project)}/${encodeURIComponent(link.comparison_id)}` };
}
