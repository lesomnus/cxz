import { Code, ConnectError } from "@connectrpc/connect";

/**
 * A stream that stays open and says nothing is the failure neither end
 * notices: the reader is waiting, the writer thinks it has nothing to write,
 * and nothing ends or throws. Reading it with a deadline is what turns that
 * silence into an error, which is what every caller here already knows how to
 * handle -- it reconnects. See cxz#188 and cxz#189.
 *
 * The abandoned read stays attached to the race, so its own later rejection is
 * already handled. Dropping its value costs nothing: the only reason to be
 * here is that there was no value.
 */
export function readBy<T>(
  read: Promise<T>,
  ms: number,
  what: string,
): Promise<T> {
  let timer: ReturnType<typeof setTimeout>;
  return Promise.race([
    read,
    new Promise<never>((_, reject) => {
      timer = setTimeout(
        () => reject(new ConnectError(what, Code.Unavailable)),
        ms,
      );
    }),
  ]).finally(() => clearTimeout(timer));
}

/**
 * Releasing an iterator is best effort and deliberately not awaited: one
 * suspended on a read that never completes -- the case a deadline reports --
 * would never process the return, and awaiting it would swallow the error
 * being thrown. The request itself goes when its signal is aborted.
 */
export function release(stream: { return?: () => Promise<unknown> }) {
  void stream.return?.().catch(() => {});
}
