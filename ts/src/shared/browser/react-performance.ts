// React 19.2 dev Performance Tracks serialize changed props and retain measures
// in the browser's timeline. Bound that diagnostic data without disabling React
// warnings or modifying application-owned Performance entries.
const MAX_MEASURES = 128;
const MAX_PROPERTIES = 32;
const MAX_TEXT = 256;
const installed = new WeakMap<Performance, () => void>();

type Track = {
  track?: string;
  trackGroup?: string;
  properties?: unknown[][];
};
const isReactTrack = (track?: Track) =>
  track?.track === "Components ⚛" || track?.trackGroup === "Scheduler ⚛";
const shortText = (value: unknown) =>
  typeof value === "string"
    ? value.length > MAX_TEXT
      ? `${value.slice(0, MAX_TEXT)}…`
      : value
    : typeof value === "number" || typeof value === "boolean"
      ? String(value)
      : "…";

export function installReactPerformanceBudget(
  target: Performance = performance,
) {
  const existing = installed.get(target);
  if (existing) return existing;
  const original = target.measure;
  const recent: string[] = [];
  const clear = (name: string) => {
    // A third party may use the same label. Keep their entries intact.
    if (
      target
        .getEntriesByName(name, "measure")
        .every((entry) =>
          isReactTrack((entry as PerformanceMeasure).detail?.devtools),
        )
    )
      target.clearMeasures(name);
  };
  const measure: Performance["measure"] = (name, options, endMark) => {
    if (
      !options ||
      typeof options !== "object" ||
      !isReactTrack(options.detail?.devtools)
    )
      return original.call(target, name, options, endMark);
    const track = options.detail.devtools as Track;
    const bounded = track.properties
      ? {
          ...options,
          detail: {
            ...options.detail,
            devtools: {
              ...track,
              properties: track.properties
                .slice(0, MAX_PROPERTIES)
                .map((pair) => pair.slice(0, 2).map(shortText)),
            },
          },
        }
      : options;
    let entry: PerformanceMeasure;
    try {
      entry = original.call(target, name, bounded);
    } catch (error) {
      if (!(error instanceof DOMException) || error.name !== "DataCloneError")
        throw error;
      // Diagnostic cloning must not abort a UI commit. Drop retained React
      // entries and retry this timing without the failed props payload.
      for (const label of new Set(recent)) clear(label);
      recent.length = 0;
      entry = original.call(target, name, {
        start: options.start,
        end: options.end,
        duration: options.duration,
        detail: {
          devtools: { track: track.track, trackGroup: track.trackGroup },
        },
      });
    }
    recent.push(name);
    if (recent.length > MAX_MEASURES) clear(recent.shift()!);
    return entry;
  };
  target.measure = measure;
  const dispose = () => {
    if (target.measure === measure) target.measure = original;
    for (const name of new Set(recent)) clear(name);
    installed.delete(target);
  };
  installed.set(target, dispose);
  return dispose;
}
