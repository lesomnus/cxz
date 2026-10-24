import { expect, it } from "vitest";
import { SettingsStore } from "#src/shared/settings/settings-store.ts";
import { createSettingsThemeStorage } from "./theme-storage";

it("keeps ui.theme in the existing settings document and preserves unrelated fields", () => {
  let file = JSON.stringify({ "ui.theme": "light", "editor.fontSize": 16 });
  const store = new SettingsStore({
    getItem: () => file,
    setItem: (_key, value) => {
      file = value;
    },
  });
  const adapter = createSettingsThemeStorage(store);
  let notifications = 0;
  const stop = adapter.subscribe!(() => ++notifications);
  expect(adapter.read()).toBe("light");
  adapter.write("system");
  expect(JSON.parse(file)).toMatchObject({
    "ui.theme": "system",
    "editor.fontSize": 16,
  });
  store.sync('{"ui.theme":"dark"}');
  expect(adapter.read()).toBe("dark");
  expect(notifications).toBe(2);
  stop();
});
