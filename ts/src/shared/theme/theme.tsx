import {
  ThemeProvider as NoxThemeProvider,
  useTheme as useNoxTheme,
} from "@lesomnus/nox";
import { useMemo, type ReactNode } from "react";
import { settingsStore } from "#src/shared/settings/settings.ts";
import { createSettingsThemeStorage } from "./theme-storage";

export { resolveThemePreference as resolveTheme } from "@lesomnus/nox";
export type { Theme, ThemePreference } from "@lesomnus/nox";

export function useTheme() {
  return useNoxTheme().theme;
}
export function ThemeProvider({ children }: { children: ReactNode }) {
  const storage = useMemo(
    () => createSettingsThemeStorage(settingsStore()),
    [],
  );
  return <NoxThemeProvider storage={storage}>{children}</NoxThemeProvider>;
}
