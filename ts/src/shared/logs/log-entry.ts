export const segmentBytes = 2 * 1024 * 1024;
export const retainedBytes = 32 * 1024 * 1024;
export const pageSize = 100;
export type LogLevel = "info" | "warn" | "error";
export type LogContext = {
  connection?: string;
  project?: string;
  session?: string;
  request?: string;
  method?: string;
};
export type LogInput = LogContext & {
  level: LogLevel;
  category: "client" | "rpc";
  message: string;
  details?: Record<string, string | number | boolean | undefined>;
};
export type LogEntry = LogInput & {
  id: string;
  time: number;
  tab: string;
  version: string;
};
export type LogQuery = {
  level?: string;
  category?: string;
  connection?: string;
  project?: string;
  session?: string;
  search?: string;
  offset?: number;
};
export type LogPage = {
  entries: LogEntry[];
  total: number;
  bytes: number;
  capacity: number;
  storage: "opfs" | "memory";
};

// Only diagnostic strings reach disk. Requests/responses, headers and arbitrary
// objects are never serialized. Redaction is also applied to error stacks.
export function diagnosticText(value: string, limit = 2048) {
  return value
    .slice(0, 32768)
    .replace(/\b(?:https?|wss?):\/\/[^\s<>"']+/gi, (text) => {
      try {
        const url = new URL(text);
        return url.origin + url.pathname;
      } catch {
        return "[URL]";
      }
    })
    .replace(/\bBearer\s+[^\s,;"']+/gi, "Bearer [redacted]")
    .replace(
      /\b(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|sk-[A-Za-z0-9_-]+)/g,
      "[redacted]",
    )
    .replace(
      /((?:token|password|secret|authorization|cookie|api[_-]?key)["']?\s*[=:]\s*["']?)[^\s,;"'}]+/gi,
      "$1[redacted]",
    )
    .slice(0, limit);
}

export function logEntry(
  input: LogInput,
  tab: string,
  version: string,
): LogEntry {
  const result: LogEntry = {
    id: crypto.randomUUID(),
    time: Date.now(),
    tab,
    version,
    level: input.level,
    category: input.category,
    message: diagnosticText(input.message),
  };
  for (const key of [
    "connection",
    "project",
    "session",
    "request",
    "method",
  ] as const) {
    if (input[key]) result[key] = diagnosticText(input[key], 512);
  }
  if (input.details) {
    result.details = {};
    for (const [key, value] of Object.entries(input.details).slice(0, 16)) {
      if (/token|password|secret|authorization|cookie|api[_-]?key/i.test(key))
        continue;
      if (typeof value === "string")
        result.details[key] = diagnosticText(
          value,
          key === "stack" ? 8192 : 2048,
        );
      else if (
        typeof value === "boolean" ||
        (typeof value === "number" && Number.isFinite(value))
      )
        result.details[key] = value;
    }
  }
  return result;
}

export function matchesLog(entry: LogEntry, query: LogQuery) {
  for (const key of [
    "level",
    "category",
    "connection",
    "project",
    "session",
  ] as const) {
    if (query[key] && entry[key] !== query[key]) return false;
  }
  const search = query.search?.trim().toLowerCase();
  return !search || JSON.stringify(entry).toLowerCase().includes(search);
}

export const encodeLine = (entry: LogEntry) => JSON.stringify(entry) + "\n";
export function decodeLines(text: string): LogEntry[] {
  return text.split("\n").flatMap((line) => {
    try {
      const entry = JSON.parse(line) as LogEntry;
      return typeof entry.id === "string" &&
        Number.isFinite(entry.time) &&
        typeof entry.message === "string" &&
        ["info", "warn", "error"].includes(entry.level)
        ? [entry]
        : [];
    } catch {
      return [];
    } // A torn final write must not hide earlier records.
  });
}
