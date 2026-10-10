import { expect, it } from "vitest";
import {
  defaultEditorSettings,
  parseSettings,
  resolveEditorSettings,
} from "./editor-settings";
import { SettingsStore } from "./settings-store";

function fixture(raw: string | null = null) {
  let file = raw;
  const storage = {
    getItem: () => file,
    setItem: (_key: string, value: string) => {
      file = value;
    },
  };
  return { store: new SettingsStore(storage), storage, file: () => file };
}
it("inherits each missing session field independently, including explicit false", () => {
  const document = parseSettings(
    '{"editor.indentSize":4,"editor.insertSpaces":true,"editor.tabSize":8,"editor.colorPalette":"warm","session.editor.insertSpaces":false,"session.editor.tabSize":2}',
  );
  expect(resolveEditorSettings(document)).toEqual({
    indentSize: 4,
    insertSpaces: true,
    tabSize: 8,
    colorPalette: "warm",
    fontFamily: defaultEditorSettings.fontFamily,
  });
  expect(resolveEditorSettings(document, "session")).toEqual({
    indentSize: 4,
    insertSpaces: false,
    tabSize: 2,
    colorPalette: "warm",
    fontFamily: defaultEditorSettings.fontFamily,
  });
  expect(resolveEditorSettings({})).toEqual(defaultEditorSettings);
});
it("validates known settings and retains unknown future keys in a flat JSON object", () => {
  for (const raw of [
    "[]",
    "null",
    "{",
    '{"editor.tabSize":0}',
    '{"session.editor.indentSize":17}',
    '{"editor.tabSize":1.5}',
    '{"editor.insertSpaces":"false"}',
    '{"editor.colorPalette":"unknown"}',
    '{"editor.fontFamily":42}',
    '{"session.editor.fontFamily":"  "}',
  ])
    expect(() => parseSettings(raw)).toThrow();
  const { store, file } = fixture(
    '{"future":{"nested":[1,true]},"editor.tabSize":4}',
  );
  store.set("session.editor.tabSize", 2);
  store.set("editor.indentSize", 6);
  store.set("session.editor.tabSize", undefined);
  expect(JSON.parse(file()!)).toEqual({
    future: { nested: [1, true] },
    "editor.tabSize": 4,
    "editor.indentSize": 6,
  });
  expect(
    resolveEditorSettings(store.snapshot().document, "session").tabSize,
  ).toBe(4);
});
it("inherits global fonts and removes a session override without writing defaults", () => {
  const { store, file } = fixture();
  store.set("editor.fontFamily", '"D2Coding", monospace');
  expect(
    resolveEditorSettings(store.snapshot().document, "session").fontFamily,
  ).toBe('"D2Coding", monospace');
  store.set("session.editor.fontFamily", "monospace");
  expect(resolveEditorSettings(store.snapshot().document).fontFamily).toBe(
    '"D2Coding", monospace',
  );
  expect(
    resolveEditorSettings(store.snapshot().document, "session").fontFamily,
  ).toBe("monospace");
  store.set("session.editor.fontFamily", undefined);
  expect(
    resolveEditorSettings(store.snapshot().document, "session").fontFamily,
  ).toBe('"D2Coding", monospace');
  expect(JSON.parse(file()!)).toEqual({
    "editor.fontFamily": '"D2Coding", monospace',
  });
});
it("inherits a complete Google selection and replaces its provider with a local session font", () => {
  const { store, file } = fixture();
  store.set("editor.fontFamily", { provider: "google", family: "Roboto Mono" });
  expect(
    resolveEditorSettings(store.snapshot().document, "session"),
  ).toMatchObject({
    fontFamily: '"Roboto Mono", monospace',
    googleFont: "Roboto Mono",
  });
  store.set("session.editor.fontFamily", "monospace");
  const session = resolveEditorSettings(store.snapshot().document, "session");
  expect(session.fontFamily).toBe("monospace");
  expect(session.googleFont).toBeUndefined();
  store.set("session.editor.fontFamily", undefined);
  expect(
    resolveEditorSettings(store.snapshot().document, "session").googleFont,
  ).toBe("Roboto Mono");
  expect(JSON.parse(file()!)).toEqual({
    "editor.fontFamily": { provider: "google", family: "Roboto Mono" },
  });
  for (const font of [
    null,
    {},
    { provider: "other", family: "Roboto Mono" },
    { provider: "google", family: "" },
    { provider: "google", family: 'Bad"; font' },
    { provider: "google", family: " Bad" },
  ]) {
    expect(() =>
      parseSettings(JSON.stringify({ "editor.fontFamily": font })),
    ).toThrow();
  }
});
it("does not write defaults on read, and notifications follow a successful whole-file save", () => {
  const { store, file } = fixture();
  expect(file()).toBeNull();
  let observed = "";
  const unsubscribe = store.subscribe(() => {
    observed = file()!;
  });
  const raw = '{ "editor.tabSize": 8 }';
  store.save(raw);
  expect(observed).toBe(raw);
  expect(store.snapshot().raw).toBe(raw);
  unsubscribe();
});
it("rejects malformed updates and quota failures without losing the previous file or applying unsaved values", () => {
  const { store, storage, file } = fixture('{"editor.tabSize":4}');
  const before = store.snapshot();
  expect(() => store.save('{"editor.tabSize":0}')).toThrow();
  expect(store.snapshot()).toBe(before);
  storage.setItem = () => {
    throw new Error("quota");
  };
  expect(() => store.set("editor.tabSize", 8)).toThrow("quota");
  expect(file()).toBe('{"editor.tabSize":4}');
  expect(store.snapshot()).toBe(before);
});
it("retains invalid file text for recovery instead of overwriting it from form controls", () => {
  const { store, file } = fixture('{"broken"');
  expect(store.snapshot().valid).toBe(false);
  expect(store.snapshot().raw).toBe('{"broken"');
  expect(() => store.set("editor.tabSize", 4)).toThrow();
  expect(file()).toBe('{"broken"');
  store.save("{}");
  expect(store.snapshot().valid).toBe(true);
});
it("detects stale file saves and merges form changes against the latest stored file", () => {
  const { store, storage, file } = fixture('{"editor.tabSize":4}');
  const before = store.snapshot().raw;
  storage.setItem("settings", '{"editor.tabSize":8,"future":true}');
  expect(() => store.save("{}", before)).toThrow("changed");
  expect(store.snapshot().document["editor.tabSize"]).toBe(8);
  store.set("session.editor.tabSize", 2);
  expect(JSON.parse(file()!)).toEqual({
    "editor.tabSize": 8,
    future: true,
    "session.editor.tabSize": 2,
  });
});
it("reflects external replacement/removal and ignores prototype-shaped unknown settings", () => {
  const { store } = fixture();
  store.sync('{"__proto__":{"editor.tabSize":16},"editor.tabSize":8}');
  expect(resolveEditorSettings(store.snapshot().document).tabSize).toBe(8);
  expect(Object.hasOwn(store.snapshot().document, "__proto__")).toBe(true);
  store.sync(null);
  expect(resolveEditorSettings(store.snapshot().document)).toEqual(
    defaultEditorSettings,
  );
});
it("survives unavailable browser storage and bounds oversized files", () => {
  const store = new SettingsStore({
    getItem: () => {
      throw new Error("denied");
    },
    setItem: () => {
      throw new Error("denied");
    },
  });
  expect(store.snapshot().valid).toBe(false);
  expect(resolveEditorSettings(store.snapshot().document)).toEqual(
    defaultEditorSettings,
  );
  expect(() => parseSettings(" ".repeat(1024 * 1024 + 1))).toThrow("1 MiB");
});

it("accepts persisted theme preferences and rejects unsupported values without overwriting the file", () => {
  for (const theme of ["light", "dark", "system"])
    expect(
      parseSettings(JSON.stringify({ "ui.theme": theme }))["ui.theme"],
    ).toBe(theme);
  const { store, file } = fixture('{"ui.theme":"dark"}');
  for (const theme of ["auto", "LIGHT", null, 1, true])
    expect(() => store.set("ui.theme", theme)).toThrow("Unsupported theme");
  expect(file()).toBe('{"ui.theme":"dark"}');
  store.set("ui.theme", undefined);
  expect(store.snapshot().document["ui.theme"]).toBeUndefined();
});
