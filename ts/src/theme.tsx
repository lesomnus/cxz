import { useLayoutEffect, useSyncExternalStore, type ReactNode } from "react";
import { useSettings } from "./settings";

export type ThemePreference = "light" | "dark" | "system";
export type Theme = "light" | "dark";
export function resolveTheme(value: unknown): ThemePreference {
  return value === "light" || value === "system" ? value : "dark";
}
let media: MediaQueryList | undefined;
function systemTheme() {
  return (media ??= window.matchMedia("(prefers-color-scheme: dark)"));
}
function subscribe(callback: () => void) {
  const query = systemTheme();
  query.addEventListener("change", callback);
  return () => query.removeEventListener("change", callback);
}
export function useTheme(): Theme {
  const { snapshot } = useSettings();
  const dark = useSyncExternalStore(subscribe, () => systemTheme().matches);
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
