type Result = { data: unknown; expiresAt: number };
type Entry = { result?: Result };

export const ACCOUNT_INFO_TTL = 5 * 60_000;
export const USAGE_TTL = 60_000;
export const ACCOUNT_LIST_KEY = "account-list";
export const accountReadPrefix = (identity: string) =>
  `account/${encodeURIComponent(identity)}/`;

// Browser memory belongs to one authenticated Connection, with bounded LRU
// entries. Neither API credentials nor responses are written to browser storage.
export class ReadCache {
  private entries = new Map<string, Entry>();
  constructor(
    private limit = 256,
    private now = () => Date.now(),
  ) {}

  peek<T>(key: string) {
    const entry = this.entries.get(key);
    if (entry) {
      this.entries.delete(key);
      this.entries.set(key, entry);
    }
    const result = entry?.result;
    return (
      result && {
        data: result.data as T,
        fresh: result.expiresAt > this.now(),
      }
    );
  }

  async read<T>(
    key: string,
    ttl: number,
    load: (signal: AbortSignal) => Promise<T>,
    signal: AbortSignal,
  ): Promise<T> {
    signal.throwIfAborted();
    const previous = this.entries.get(key);
    this.entries.delete(key);
    if (previous?.result && previous.result.expiresAt > this.now()) {
      this.entries.set(key, previous);
      return previous.result.data as T;
    }
    const entry = { result: previous?.result };
    this.entries.set(key, entry);
    while (this.entries.size > this.limit)
      this.entries.delete(this.entries.keys().next().value!);
    const data = await load(signal);
    // A newer read, invalidation, disposal or view cancellation wins over a
    // late reply. Failed reads never become fresh cached results.
    if (!signal.aborted && this.entries.get(key) === entry)
      entry.result = { data, expiresAt: this.now() + ttl };
    return data;
  }

  invalidate(key: string) {
    this.entries.delete(key);
  }
  invalidatePrefix(prefix: string) {
    for (const key of this.entries.keys())
      if (key.startsWith(prefix)) this.entries.delete(key);
  }
  clear() {
    this.entries.clear();
  }
}

export type RemoteReadOptions = {
  cache?: { store: ReadCache; key: string; ttl: number };
  refresh?: number;
  enabled?: boolean;
  keepPreviousData?: boolean;
};
