// gc (plan 2.9): keeps the installed build, the previous build and the newest N; never deletes backups.
import { lstatSync, realpathSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { listBuildDirs } from "./context.mjs";

/**
 * @returns {{keep:string[], remove:string[], removed:string[]}}
 */
export function gcBuilds({ buildsRoot, pluginId, keepN, installedRoot, previousRoot, apply }) {
  const all = listBuildDirs(buildsRoot, pluginId);
  const keep = new Set([installedRoot, previousRoot].filter(Boolean));
  for (const d of all.slice(-keepN)) keep.add(d);
  const base = join(buildsRoot, pluginId);
  const remove = [];
  for (const d of all) {
    if (keep.has(d)) continue;
    if (dirname(d) !== base || lstatSync(d).isSymbolicLink()) continue;
    remove.push(d);
  }
  const removed = [];
  if (apply) {
    for (const d of remove) {
      if (!realpathSync(d).startsWith(`${realpathSync(base)}/`)) continue;
      rmSync(d, { recursive: true, force: true });
      removed.push(d);
    }
  }
  return { keep: [...keep], remove, removed };
}
