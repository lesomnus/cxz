import { isGoogleFont } from "#src/shared/settings/editor-settings.ts";

const loads = new Map<string, Promise<void>>();
const status = new Map<string, "loading" | "ready" | "error">();
const listeners = new Set<() => void>();
export function subscribeGoogleFonts(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
function notify() {
  listeners.forEach((listener) => listener());
}
export function googleFontStatus(family?: string) {
  return family ? (status.get(family) ?? "idle") : "ready";
}
export function googleFontURL(family: string) {
  if (!isGoogleFont({ provider: "google", family }))
    throw new Error("Invalid Google Fonts family");
  const query = new URLSearchParams({
    // Request the default face so families without intermediate weights (for
    // example Space Mono) work too. Editors synthesize emphasis when needed.
    family,
    display: "swap",
  });
  return `https://fonts.googleapis.com/css2?${query}`;
}

// Keep successful stylesheets for the lifetime of the page. Browser HTTP cache
// owns the font bytes; settings store only provider/family metadata.
export function loadGoogleFont(family: string): Promise<void> {
  const existing = loads.get(family);
  if (existing) return existing;
  const href = googleFontURL(family);
  const promise = new Promise<void>((resolve, reject) => {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.crossOrigin = "anonymous";
    link.href = href;
    let settled = false;
    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      link.onload = link.onerror = null;
      if (error) {
        link.remove();
        reject(error);
      } else {
        status.set(family, "ready");
        notify();
        resolve();
      }
    };
    const timer = setTimeout(
      () => finish(new Error("Font download timed out")),
      12000,
    );
    link.onerror = () => finish(new Error("Font stylesheet download failed"));
    link.onload = () => {
      // A stylesheet alone is not ready: load representative Latin/Korean
      // glyphs before switching the editor's metrics. Other subsets remain lazy.
      document.fonts
        .load(`400 14px ${JSON.stringify(family)}`, "Aa0가")
        .then((faces) =>
          finish(faces.length ? undefined : new Error("Font family not found")),
        )
        .catch((error: unknown) => finish(new Error(String(error))));
    };
    document.head.append(link);
  });
  loads.set(family, promise);
  status.set(family, "loading");
  notify();
  // Failed requests must be retryable, including after going back online.
  void promise.catch(() => {
    if (loads.get(family) === promise) {
      loads.delete(family);
      status.set(family, "error");
      notify();
    }
  });
  return promise;
}
