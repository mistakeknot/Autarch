/** One read at a time (bead mk-t7656). A call made while a read is in flight queues exactly one more read after it, so
 * replies land in order, the newest state always lands, and a read slower than the poll never starves the page. */
export function singleFlight(run: () => Promise<unknown>): () => void {
  let busy = false;
  let again = false;
  const go = (): void => {
    if (busy) {
      again = true;
      return;
    }
    busy = true;
    again = false;
    run()
      .catch(() => {})
      .finally(() => {
        busy = false;
        if (again) go();
      });
  };
  return go;
}
