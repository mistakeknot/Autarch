/** Shared, bounded card identity for confirmation buttons and bring-back notices. */
export function formatCardLabel(key: string | null | undefined, taskId: string, title: string | null | undefined, subject?: string | null): string {
  return `${key || taskId}: ${(title || subject || "Untitled card").slice(0, 120)}`;
}
