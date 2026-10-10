import { useLayoutEffect, useSyncExternalStore, type ReactNode } from "react";
import { useSettings } from "#src/shared/settings/settings.ts";

export type ThemePreference = "light" | "dark" | "system";
export type Theme = "light" | "dark";
export function resolveTheme(value: unknown): ThemePreference {
  return value === "light" || value === "system" ? value : "dark";
}
let media: MediaQueryList | undefined;
let systemDark = false;
const listeners = new Set<() => void>();
function systemTheme() {
  if (!media) {
    media = window.matchMedia("(prefers-color-scheme: dark)");
    systemDark = media.matches;
    media.addEventListener("change", (event) => {
      // Publish the event's snapshot before notifying any React subscribers.
      systemDark = event.matches;
      for (const listener of listeners) listener();
    });
  }
  return media;
}
function subscribe(callback: () => void) {
  systemTheme();
  listeners.add(callback);
  return () => listeners.delete(callback);
}
function systemSnapshot() {
  systemTheme();
  return systemDark;
}
export function useTheme(): Theme {
  const { snapshot } = useSettings();
  const dark = useSyncExternalStore(subscribe, systemSnapshot);
  const preference = resolveTheme(snapshot.document["ui.theme"]);
  return preference === "system" ? (dark ? "dark" : "light") : preference;
}
export function ThemeProvider({ children }: { children: ReactNode }) {
  const theme = useTheme();
  useLayoutEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  return children;
}
