export interface Flight {
  /** Start a read, or queue one more if a read is in flight. */
  go: () => void;
  /** Stop for good (unmount): no queued or new read starts, and a reply still in flight is ignored. */
  stop: () => void;
}

/** One read at a time (bead mk-t7656). A call made while a read is in flight queues exactly one more read after it, so
 * replies land in order, the newest state always lands, and a read slower than the poll never starves the page. A read
 * that has not settled after `timeoutMs` is given up: its reply is ignored (`live()` turns false) and the next read runs. */
export function singleFlight(run: (live: () => boolean) => Promise<unknown>, timeoutMs = 30_000): Flight {
  let busy = false;
  let again = false;
  let stopped = false;
  let gen = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const go = (): void => {
    if (stopped) return;
    if (busy) {
      again = true;
      return;
    }
    busy = true;
    again = false;
    const g = ++gen;
    const settle = (): void => {
      if (g !== gen) return; // already given up, or stopped
      clearTimeout(timer);
      gen++;
      busy = false;
      if (again) go();
    };
    timer = setTimeout(settle, timeoutMs);
    let p: Promise<unknown>;
    try {
      p = run(() => !stopped && g === gen);
    } catch {
      p = Promise.resolve();
    }
    p.catch(() => {}).finally(settle);
  };
  const stop = (): void => {
    stopped = true;
    gen++;
    clearTimeout(timer);
  };
  return { go, stop };
}
