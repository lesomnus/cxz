import type { SessionEvent } from "#gen/cxz/session_pb";

export type SnapshotSource<T extends object> = () => T;
export type EventSource = SnapshotSource<SessionEvent>;
const sources = new WeakMap<object, SnapshotSource<object>>();

// Keep protocol buffers outside React's changed-prop inspection. A stable
// reader preserves event identity and full payloads without enumerating bytes
// in development Performance Tracks whenever virtual rows are reused.
export function snapshotSource<T extends object>(value: T): SnapshotSource<T>;
export function snapshotSource<T extends object>(
  value: T | undefined,
): SnapshotSource<T> | undefined;
export function snapshotSource<T extends object>(
  value: T | undefined,
): SnapshotSource<T> | undefined {
  if (!value) return undefined;
  let source = sources.get(value);
  if (!source) {
    source = () => value;
    sources.set(value, source);
  }
  return source as SnapshotSource<T>;
}

export function eventSource(event: SessionEvent): EventSource {
  return snapshotSource(event);
}

export type EventListSource = () => SessionEvent[];

export function eventListSource(events: SessionEvent[]): EventListSource {
  return snapshotSource(events);
}
