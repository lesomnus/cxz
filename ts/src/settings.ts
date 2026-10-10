import { useSyncExternalStore } from "react";
import { resolveEditorSettings, type EditorScope } from "./editor-settings";
import { SettingsStore } from "./settings-store";
import { useFontFamily } from "./use-font-family";

let instance: SettingsStore | undefined;
export function settingsStore() {
  if (!instance) {
    // Access to localStorage itself can be denied in restricted browser modes.
    instance = new SettingsStore({
      getItem: (key) => window.localStorage.getItem(key),
      setItem: (key, value) => window.localStorage.setItem(key, value),
    });
    window.addEventListener("storage", (event) => {
      if (
        (event.key === "settings" || event.key === null) &&
        event.storageArea === window.localStorage
      )
        instance!.sync(event.key === null ? null : event.newValue);
    });
  }
  return instance;
}
export function useSettings() {
  const store = settingsStore();
  return {
    store,
    snapshot: useSyncExternalStore(store.subscribe, store.snapshot),
  };
}
export function useEditorSettings(scope: EditorScope = "global") {
  const { snapshot } = useSettings();
  const settings = resolveEditorSettings(snapshot.document, scope);
  const font = useFontFamily(settings.fontFamily, settings.googleFont);
  return { ...settings, fontFamily: font.fontFamily };
}
