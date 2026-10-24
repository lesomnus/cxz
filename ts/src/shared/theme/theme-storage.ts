import type { ThemeStorage } from "@lesomnus/nox";
import type { SettingsStore } from "#src/shared/settings/settings-store.ts";

// Keep the existing settings document and cross-tab synchronization authoritative.
export function createSettingsThemeStorage(
  store: Pick<SettingsStore, "snapshot" | "set" | "subscribe">,
): ThemeStorage {
  return {
    read: () => store.snapshot().document["ui.theme"],
    write: (preference) => store.set("ui.theme", preference),
    subscribe: store.subscribe,
  };
}
