/**
 * Booting the sandbox has to finish or say why.
 *
 * `start` resolves when the WASM module is compiled and the Go instance has
 * published its entry point. Nothing bounds the whole of that from the page:
 * the only deadline inside it covers the publish step, which is the last one, so
 * a stall anywhere earlier -- reading the module's body, writing the copy that
 * makes the next reload fast -- leaves the page on "Starting sandbox…" with no
 * error, no progress and nothing to retry. That is what a flaky browser test
 * reports as "element(s) not found" after 45 seconds, and what a person reports
 * as "the sandbox doesn't load".
 *
 * So: a deadline, and one retry without the module cache, which is the part of
 * the path that is optional. A second failure is reported rather than retried
 * forever -- an unbounded retry is the same hang with more logs.
 */

export const BootDeadlineMs = 15000;

export class BootTimeout extends Error {
  constructor(ms: number) {
    super(
      `the sandbox did not start within ${Math.round(ms / 1000)}s (no error and no progress from the module)`,
    );
    this.name = "BootTimeout";
  }
}

/**
 * deadline rejects with BootTimeout when work outlives ms. The work is not
 * cancelled, because it cannot be: the caller closes what it started. What this
 * guarantees is only that the caller stops waiting.
 */
export function deadline<T>(
  work: Promise<T>,
  ms: number,
  timer: Timer = globalThis,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const id = timer.setTimeout(() => reject(new BootTimeout(ms)), ms);
    work.then(
      (v) => {
        timer.clearTimeout(id);
        resolve(v);
      },
      (e) => {
        timer.clearTimeout(id);
        reject(e);
      },
    );
  });
}

// Only the two calls this needs, so a test can supply a clock and a page can
// pass itself. The handle is whatever the host returns -- a number in a browser,
// an object in Node -- and is only ever handed straight back.
export interface Timer {
  setTimeout(fn: () => void, ms: number): TimerHandle;
  clearTimeout(id: TimerHandle): void;
}

type TimerHandle = ReturnType<typeof setTimeout>;

export interface Attempt<T> {
  /** cached is false on the retry, where the module cache is left out. */
  (cached: boolean): Promise<T>;
}

/**
 * boot runs an attempt under a deadline and, if that attempt times out, runs one
 * more without the cache. Only a timeout is retried: an attempt that failed with
 * a reason has given one, and repeating it would replace the reason with a
 * second copy of it.
 */
export async function boot<T>(
  attempt: Attempt<T>,
  opts: { ms?: number; timer?: Timer; onRetry?: (e: BootTimeout) => void } = {},
): Promise<T> {
  const ms = opts.ms ?? BootDeadlineMs;
  try {
    return await deadline(attempt(true), ms, opts.timer);
  } catch (e) {
    if (!(e instanceof BootTimeout)) throw e;
    opts.onRetry?.(e);
    return await deadline(attempt(false), ms, opts.timer);
  }
}
