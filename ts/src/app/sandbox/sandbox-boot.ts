/**
 * Booting the sandbox has to finish or say why.
 *
 * `start` resolves when the WASM module is compiled and the Go instance has
 * published its entry point, before an RPC has proved the service is usable.
 * The caller's attempt includes that readiness check. Nothing bounds it:
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

/**
 * bootDeadline reads the deadline from the page's own URL, so a test that
 * forces a stall does not also have to wait out a deadline chosen for people.
 *
 * What the stall tests verify is the sequence -- deadline, retry without the
 * cache, report the second failure -- and that is the same at 300ms as at
 * fifteen seconds. The stall itself is already forced rather than waited for;
 * this is the other half of the same arrangement.
 *
 * Bounded to a second so the parameter cannot be used to make a person's
 * sandbox fail to start, and ignored unless it parses.
 */
export function bootDeadline(search = location.search): number {
  const raw = new URLSearchParams(search).get("boot-deadline");
  if (raw === null) return BootDeadlineMs;
  const ms = Number(raw);
  if (!Number.isFinite(ms) || ms < 1 || ms > 1000) return BootDeadlineMs;
  return ms;
}

export class BootTimeout extends Error {
  constructor(ms: number) {
    super(
      `the sandbox did not start within ${Math.round(ms / 1000)}s (module loading or service readiness stalled)`,
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
  (cached: boolean, signal: AbortSignal): Promise<T>;
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
  const run = async (cached: boolean) => {
    const controller = new AbortController();
    try {
      return await deadline(attempt(cached, controller.signal), ms, opts.timer);
    } catch (e) {
      // Close a stalled worker and cancel its readiness call before retrying.
      // A late start must also observe this signal and close its own worker.
      controller.abort(e);
      throw e;
    }
  };
  try {
    return await run(true);
  } catch (e) {
    if (!(e instanceof BootTimeout)) throw e;
    opts.onRetry?.(e);
    return await run(false);
  }
}
