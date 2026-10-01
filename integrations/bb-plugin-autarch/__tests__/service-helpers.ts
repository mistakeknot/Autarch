import Database from "better-sqlite3";
import { chmodSync, lstatSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { Delegation } from "../delegation.js";
import { Service, type ProjectInfo, type ServiceDeps } from "../service.js";
import { Store } from "../store.js";
import { tmpDir } from "./helpers.js";

export interface Env {
  dir: string;
  file: string;
  roots: Record<string, string>;
  projects: ProjectInfo[];
  clock: { t: number };
  down: { value: boolean };
  ids: { n: number };
  now: () => string;
  open: (over?: Partial<ServiceDeps>) => Service;
  addProject: (name: string) => string;
  cleanup: () => void;
}

export const T0 = Date.parse("2026-09-26T14:03:00.000Z");

/** A temp database file, project roots that "serve" resolves, a controllable clock and id source. */
export function makeEnv(projectNames = ["Autarch"], idPrefix = "dec"): Env {
  const t = tmpDir();
  const clock = { t: T0 };
  const down = { value: false };
  const ids = { n: 0 };
  const projects: ProjectInfo[] = [];
  const roots: Record<string, string> = {};
  const now = () => new Date(clock.t).toISOString();
  const addProject = (name: string) => {
    const root = join(t.dir, `root-${name}`);
    mkdirSync(root);
    const st = lstatSync(root);
    projects.push({ name, root, dev: st.dev, ino: st.ino });
    roots[name] = root;
    return root;
  };
  projectNames.forEach(addProject);
  const file = join(t.dir, "data.db");
  const open = (over: Partial<ServiceDeps> = {}) => {
    const store = new Store(new Database(file), { now });
    return new Service({
      store,
      now,
      newId: () => `${idPrefix}-${++ids.n}`,
      projects: async () => {
        if (down.value) throw new Error("serve is down");
        return projects;
      },
      env: {},
      ...over,
    });
  };
  const cleanup = () => {
    for (const r of Object.values(roots)) {
      try {
        chmodSync(r, 0o755);
      } catch {
        /* gone */
      }
    }
    t.cleanup();
  };
  return { dir: t.dir, file, roots, projects, clock, down, ids, now, open, addProject, cleanup };
}

export const OPTIONS = [
  { id: "project", label: "Collapse per project", kind: "instruction", reversible: true, instruction: "On branch feat/x, group per project, run npm test, commit locally, and report." },
  { id: "day", label: "Collapse per day", kind: "ruling-only" },
  { id: "ask", label: "Show me both first", kind: "needs-context" },
];

export function ask(env: Env, over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    v: 1,
    kind: "decide",
    project: "Autarch",
    project_root: env.roots.Autarch,
    asker: "thread",
    thread: "thr-a",
    question: "Collapse routine catch-up items per project or per day?",
    subject: "autarch/catch-up: collapse order",
    options: OPTIONS,
    ...over,
  };
}

/**
 * A Delegation whose project list serve has already confirmed, as after a successful open check
 * (plan 1.3.6). A bare `new Delegation(svc)` refuses to rule until that check runs.
 */
export function verifiedDelegation(svc: Service, names: readonly string[] = ["Autarch", "Other"]): Delegation {
  const d = new Delegation(svc);
  d.applyProjectList(names);
  return d;
}
