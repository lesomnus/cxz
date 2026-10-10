// Acknowledge bytes only after xterm has parsed them, batching small writes.
// The timer flushes a final partial batch so an exiting shell can drain.
export function createTerminalAcknowledger(send: (bytes: number) => void) {
  let pending = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let disposed = false;
  const flush = () => {
    clearTimeout(timer);
    timer = undefined;
    if (!pending || disposed) return;
    const bytes = pending;
    pending = 0;
    send(bytes);
  };
  return {
    parsed(bytes: number) {
      if (disposed) return;
      pending += bytes;
      if (pending >= 16 * 1024) flush();
      else timer ??= setTimeout(flush, 5);
    },
    dispose() {
      disposed = true;
      pending = 0;
      clearTimeout(timer);
      timer = undefined;
    },
  };
}
