const prefix = "cxz.recent-searches.v1:";
export const searchHistoryLifetime = 7 * 24 * 60 * 60 * 1000;
const limit = 50;

export type SearchMatch = "substring" | "regex" | "fuzzy";
export type SearchFilters = {
  query: string;
  project: string;
  projectName: string;
  match: SearchMatch;
  ignoreCase: boolean;
  since: string;
  until: string;
};
export type RecentSearch = SearchFilters & {
  id: string;
  time: number;
  hits: number;
  hasMore: boolean;
  partial: boolean;
};

function validDate(value: unknown) {
  return (
    typeof value === "string" &&
    (!value ||
      (/^\d{4}-\d{2}-\d{2}$/.test(value) &&
        Number.isFinite(Date.parse(value + "T00:00:00Z"))))
  );
}

function valid(value: unknown, now: number): value is RecentSearch {
  if (!value || typeof value !== "object") return false;
  const row = value as Partial<RecentSearch>;
  return (
    typeof row.id === "string" &&
    typeof row.time === "number" &&
    row.time > now - searchHistoryLifetime &&
    row.time <= now &&
    typeof row.query === "string" &&
    row.query.length <= 4096 &&
    !!row.query.trim() &&
    typeof row.project === "string" &&
    row.project.length <= 256 &&
    typeof row.projectName === "string" &&
    row.projectName.length <= 512 &&
    (row.match === "substring" ||
      row.match === "regex" ||
      row.match === "fuzzy") &&
    typeof row.ignoreCase === "boolean" &&
    validDate(row.since) &&
    validDate(row.until) &&
    typeof row.hits === "number" &&
    Number.isSafeInteger(row.hits) &&
    row.hits >= 0 &&
    typeof row.hasMore === "boolean" &&
    typeof row.partial === "boolean"
  );
}

function trim(rows: unknown, now: number): RecentSearch[] {
  return Array.isArray(rows)
    ? rows
        .filter((row) => valid(row, now))
        .sort((a, b) => b.time - a.time)
        .slice(0, limit)
    : [];
}

function persisted(storage: Storage, key: string, now: number) {
  const raw = storage.getItem(key);
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    parsed = null;
  }
  const rows = trim(parsed, now);
  if (!rows.length) storage.removeItem(key);
  else {
    const clean = JSON.stringify(rows);
    if (clean !== raw) storage.setItem(key, clean);
  }
  return rows;
}

function sameFilters(a: SearchFilters, b: SearchFilters) {
  return (
    a.query === b.query &&
    a.project === b.project &&
    a.match === b.match &&
    a.ignoreCase === b.ignoreCase &&
    a.since === b.since &&
    a.until === b.until
  );
}

export class SearchHistory {
  private rows: RecentSearch[] = [];
  constructor(
    private storage?: Storage,
    private readonly key?: string,
  ) {}

  list(now = Date.now()) {
    if (this.storage && this.key) {
      try {
        this.rows = persisted(this.storage, this.key, now);
      } catch {
        this.storage = undefined;
      }
    }
    this.rows = trim(this.rows, now);
    return this.rows;
  }

  remember(search: RecentSearch, more: boolean, now = Date.now()) {
    if (!valid(search, now)) return this.list(now);
    const rows = this.list(now);
    const previous = rows.find((row) => sameFilters(row, search));
    if (
      more
        ? previous?.id !== search.id
        : previous && previous.time > search.time
    )
      return rows;
    return this.save(
      trim([search, ...rows.filter((row) => !sameFilters(row, search))], now),
    );
  }

  clear() {
    return this.save([]);
  }

  private save(rows: RecentSearch[]) {
    this.rows = rows;
    if (this.storage && this.key) {
      try {
        if (rows.length) this.storage.setItem(this.key, JSON.stringify(rows));
        else this.storage.removeItem(this.key);
      } catch {
        this.storage = undefined;
      }
    }
    return rows;
  }
}

export function createSearchHistory(baseUrl: string, identity?: string) {
  if (identity) {
    try {
      const storage = localStorage;
      // Also retire histories from credentials that have since been rotated.
      const keys = Array.from({ length: storage.length }, (_, i) =>
        storage.key(i),
      );
      const now = Date.now();
      for (const key of keys) {
        if (key?.startsWith(prefix)) persisted(storage, key, now);
      }
      const scope = encodeURIComponent(new URL(baseUrl).href);
      return new SearchHistory(
        storage,
        `${prefix}${scope}:${encodeURIComponent(identity)}`,
      );
    } catch {
      // Search stays usable when Web Storage is disabled or full.
    }
  }
  return new SearchHistory();
}
