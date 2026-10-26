// Posting a note's card comment exactly once (bead mk-1poq7). The handler checks for the note marker and then
// creates the comment; two concurrent requests with one note id (a retry while the first send is still pending)
// would both see no marker. One at a time per (task, note id) closes that window.
import { hasNoteMarker } from "./service.js";

export interface NoteComments {
  listComments(taskId: string): Promise<{ body: string }[]>;
  createComment(taskId: string, body: string): Promise<unknown>;
}

const tails = new Map<string, Promise<unknown>>();

/** Runs fn after every earlier call with the same key has settled; the map entry is dropped when the queue empties. */
export function serialPerKey<T>(key: string, fn: () => Promise<T>): Promise<T> {
  const run = (tails.get(key) ?? Promise.resolve()).then(fn, fn);
  const tail = run.catch(() => {});
  tails.set(key, tail);
  void tail.then(() => { if (tails.get(key) === tail) tails.delete(key); });
  return run;
}

/** Posts body on the task unless a comment with this note's marker is already there. */
export function postNoteOnce(tasks: NoteComments, taskId: string, noteId: string, body: string): Promise<void> {
  return serialPerKey(`${taskId}\u0000${noteId}`, async () => {
    const have = (await tasks.listComments(taskId)).some((c) => hasNoteMarker(c.body, noteId));
    if (!have) await tasks.createComment(taskId, body);
  });
}
