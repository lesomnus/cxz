import {
  t,
  translateKnown,
  languages,
  localeStore,
  resolveLocale,
  type Locale,
} from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Button } from "@lesomnus/cxz-ui";
import {
  useSettings,
  useEditorSettings,
} from "#src/shared/settings/settings.ts";
import {
  defaultEditorSettings,
  editorKeys,
  MAX_SETTINGS_BYTES,
  palettes,
  paletteVariables,
  resolveEditorSettings,
  parseSettings,
  type EditorScope,
  type FontFamilySetting,
} from "#src/shared/settings/editor-settings.ts";
import { SettingField } from "@lesomnus/cxz-ui";
import { ValueMenu } from "@lesomnus/cxz-ui";
import { FontFamilyControl } from "#src/features/settings/components/font-family-control.tsx";
import { SettingSlider } from "@lesomnus/cxz-ui";
import { SegmentedControl } from "@lesomnus/cxz-ui";
import { Switch } from "@lesomnus/cxz-ui";
import { resolveTerminalSettings } from "#src/shared/settings/terminal-settings.ts";
import { useTheme, resolveTheme } from "#src/shared/theme/theme.tsx";
import {
  SourceEditor,
  type SourceEditorHandle,
} from "#src/shared/editor/source-editor.tsx";

const fields = {
  fontFamily: {
    label: "Font family",
    description: "Font used for editor text and line numbers",
  },
  indentSize: {
    label: "Indentation size",
    description: "Spaces inserted with Tab or removed with Shift+Tab",
  },
  insertSpaces: {
    label: "Tab input",
    description: "Choose spaces or a real tab character",
    choices: [
      ["true", "Spaces"],
      ["false", "Tab character"],
    ],
  },
  tabSize: {
    label: "Tab display width",
    description: "Columns used to display a real tab character",
  },
  colorPalette: {
    label: "Color palette",
    description: "Syntax highlighting colors",
    choices: Object.entries(palettes).map(([key, palette]) => [
      key,
      palette.label,
    ]),
  },
};

export function SettingsPage({
  fileOpen,
  setFileOpen,
  topic,
}: {
  fileOpen: boolean;
  setFileOpen: (open: boolean) => void;
  topic: "editor" | "general";
}) {
  const locale = useLocale();
  const theme = useTheme();
  const { store, snapshot } = useSettings();
  const globalEditor = useEditorSettings();
  const sessionEditor = useEditorSettings("session");
  const savedLanguage = resolveLocale(snapshot.document["ui.language"]);
  const [language, setLanguage] = useState<Locale>(savedLanguage);
  const [applying, setApplying] = useState(false);
  useEffect(() => {
    setLanguage(savedLanguage);
  }, [savedLanguage]);
  const area = useRef<HTMLElement>(null);
  const filePane = useRef<HTMLElement>(null);
  const fileInput = useRef<SourceEditorHandle>(null);
  const [wide, setWide] = useState(false);
  const [draft, setDraft] = useState(snapshot.raw);
  const baseline = useRef(snapshot.raw);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [tabMovesFocus, setTabMovesFocus] = useState(false);
  const upload = useRef<HTMLInputElement>(null);
  const global = resolveEditorSettings(snapshot.document);
  useLayoutEffect(() => {
    function resize(width: number) {
      const next = width >= 1600;
      // Keep a focused JSON editor available when the side pane folds away.
      if (!next && filePane.current?.contains(document.activeElement))
        setFileOpen(true);
      setWide(next);
    }
    resize(area.current!.clientWidth);
    const observer = new ResizeObserver(([entry]) =>
      resize(entry.contentRect.width),
    );
    observer.observe(area.current!);
    return () => observer.disconnect();
  }, [setFileOpen]);
  useLayoutEffect(() => {
    if (fileOpen && !wide) fileInput.current?.focus();
  }, [fileOpen, wide]);
  useEffect(() => {
    if (draft === baseline.current) {
      baseline.current = snapshot.raw;
      setDraft(snapshot.raw);
    }
  }, [snapshot.raw]);
  const dirty = draft !== baseline.current;
  const stale = baseline.current !== snapshot.raw;
  const jsonVisible = wide || fileOpen;
  const feedback = (
    <>
      {(error || snapshot.error) && (
        <p role="alert">{error || snapshot.error}</p>
      )}
      {message && <p role="status">{translateKnown(message)}</p>}
    </>
  );
  function update(key: string, value: unknown) {
    try {
      store.set(key, value);
      setError("");
      setMessage("Saved.");
    } catch (error) {
      setError(String(error));
    }
  }
  function reload() {
    baseline.current = snapshot.raw;
    setDraft(snapshot.raw);
    setError("");
    setMessage("");
  }
  async function save(raw = draft) {
    const expected = baseline.current;
    try {
      await localeStore.prepare(
        resolveLocale(parseSettings(raw)["ui.language"]),
      );
      store.save(raw, expected);
      baseline.current = raw;
      setError("");
      setMessage("Saved.");
    } catch (error) {
      setError(String(error));
    }
  }
  async function applyLanguage() {
    setApplying(true);
    setError("");
    setMessage("");
    try {
      await localeStore.prepare(language);
      if (
        resolveLocale(store.snapshot().document["ui.language"]) !==
        savedLanguage
      )
        throw new Error(
          t("Settings have changed. Reload the saved file before editing."),
        );
      store.set("ui.language", language);
      setMessage("Language applied.");
    } catch (error) {
      setError(
        t(
          "Unable to apply the language. Your previous language and saved settings are unchanged.",
        ) +
          " " +
          String(error),
      );
    } finally {
      setApplying(false);
    }
  }
  function group(scope: EditorScope) {
    const resolved = scope === "session" ? sessionEditor : globalEditor;
    const prefix = scope === "session" ? "session.editor." : "editor.";
    const title =
      scope === "session" ? t("Session editor") : t("Global editor");
    return (
      <section className="settings-group" aria-label={title}>
        <h2>{title}</h2>
        <p className="muted">
          {scope === "session"
            ? t(
                "Unset values inherit global editor settings. These settings also apply to question answers.",
              )
            : t("Default settings shared by browser file views and editors.")}
        </p>
        <fieldset disabled={!snapshot.valid}>
          {editorKeys.map((key) => {
            const name = prefix + key;
            const hasValue = Object.hasOwn(snapshot.document, name);
            const inherited =
              scope === "session" ? global[key] : defaultEditorSettings[key];
            return (
              <SettingField
                key={key}
                title={translateKnown(fields[key].label)}
                settingId={name}
                summary={translateKnown(fields[key].description)}
                details={
                  key === "fontFamily"
                    ? t(
                        "Browser monospace follows your browser's fixed-width font preference. Custom fonts use local font families. Google Fonts are downloaded on demand and cached by your browser.",
                      )
                    : undefined
                }
              >
                {key === "fontFamily" ? (
                  <FontFamilyControl
                    label={`${title} ${translateKnown(fields[key].label)}`}
                    value={
                      hasValue
                        ? (snapshot.document[name] as FontFamilySetting)
                        : undefined
                    }
                    inherited={
                      scope === "session"
                        ? ((snapshot.document["editor.fontFamily"] as
                            | FontFamilySetting
                            | undefined) ?? defaultEditorSettings.fontFamily)
                        : defaultEditorSettings.fontFamily
                    }
                    disabled={!snapshot.valid}
                    onChange={(value) => update(name, value)}
                  />
                ) : key === "indentSize" || key === "tabSize" ? (
                  <SettingSlider
                    label={`${title} ${translateKnown(fields[key].label)}`}
                    value={
                      hasValue ? Number(snapshot.document[name]) : undefined
                    }
                    inherited={Number(inherited)}
                    disabled={!snapshot.valid}
                    onChange={(value) => update(name, value)}
                  />
                ) : key === "insertSpaces" ? (
                  <SegmentedControl
                    label={`${title} ${translateKnown(fields[key].label)}`}
                    value={hasValue ? String(snapshot.document[name]) : ""}
                    disabled={!snapshot.valid}
                    options={[
                      {
                        value: "",
                        label: `↺ ${translateKnown(inherited ? "Spaces" : "Tab character")}`,
                        muted: true,
                      },
                      ...fields[key].choices.map(([value, label]) => ({
                        value,
                        label: translateKnown(label),
                      })),
                    ]}
                    onChange={(value) =>
                      update(name, value === "" ? undefined : value === "true")
                    }
                  />
                ) : (
                  <ValueMenu
                    label={`${title} ${translateKnown(fields[key].label)}`}
                    value={hasValue ? String(snapshot.document[name]) : ""}
                    disabled={!snapshot.valid}
                    options={[
                      {
                        value: "",
                        label: translateKnown(
                          palettes[inherited as keyof typeof palettes].label,
                        ),
                        muted: true,
                      },
                      ...fields[key].choices.map(([value, label]) => ({
                        value,
                        label: translateKnown(label),
                      })),
                    ]}
                    choose={(value) => update(name, value || undefined)}
                  />
                )}
              </SettingField>
            );
          })}
        </fieldset>
        <small>{t("Current preview")}</small>
        <pre
          className="settings-preview"
          style={{
            fontFamily: resolved.fontFamily,
            tabSize: resolved.tabSize,
            ...paletteVariables(resolved.colorPalette, theme),
          }}
        >
          <code className="editor-code-line">
            <span className="hljs-comment">// {title}</span>
            {"\n"}
            <span className="hljs-keyword">function</span>{" "}
            <span className="hljs-title">example</span>
            {"() {\n"}
            {resolved.insertSpaces ? " ".repeat(resolved.indentSize) : "\t"}
            <span className="hljs-keyword">return</span>{" "}
            <span className="hljs-string">"hello"</span>
            {";\n}"}
          </code>
        </pre>
      </section>
    );
  }
  return (
    <main className="settings-page" ref={area} data-wide={wide}>
      <div className="settings-split">
        <section
          id="settings-editor"
          className="settings-form-pane"
          aria-label={
            topic === "editor" ? t("Editor settings") : t("General settings")
          }
          hidden={!wide && fileOpen}
        >
          <div className="settings-editor-body">
            <header>
              <div>
                <h1>{topic === "editor" ? t("Editor") : t("General")}</h1>
                <small>
                  {topic === "editor"
                    ? t("Editor changes are saved immediately in this browser.")
                    : t(
                        "Appearance, language, and terminal behavior for this browser.",
                      )}
                </small>
              </div>
              {!wide && (
                <Button
                  aria-controls="settings-file-editor"
                  onClick={() => setFileOpen(true)}
                >
                  {t("Edit settings.json")}
                  {dirty ? t(" · Modified") : ""}
                </Button>
              )}
            </header>
            {!jsonVisible && feedback}
            <div className="settings-editor-groups">
              {topic === "editor" ? (
                <>
                  {group("global")}
                  {group("session")}
                </>
              ) : (
                <>
                  <section className="settings-group">
                    <h2>{t("Display language")}</h2>
                    <fieldset disabled={!snapshot.valid || applying}>
                      <SettingField
                        title={t("Language")}
                        settingId="ui.language"
                        summary={t("Choose the interface language.")}
                        details={t(
                          "Language changes apply to this browser. Conversation content and code are preserved.",
                        )}
                      >
                        <div className="language-setting-control">
                          <ValueMenu
                            label={t("Display language")}
                            value={language}
                            muted={
                              !Object.hasOwn(
                                snapshot.document,
                                "ui.language",
                              ) && language === "en"
                            }
                            options={languages.map(({ id, name }) => ({
                              value: id,
                              label: name,
                            }))}
                            disabled={!snapshot.valid || applying}
                            choose={(value) => setLanguage(value as Locale)}
                          />
                          <Button
                            disabled={
                              language === savedLanguage &&
                              language === locale.locale &&
                              !locale.error
                            }
                            onClick={() => void applyLanguage()}
                          >
                            {applying ? t("Applying…") : t("Apply settings")}
                          </Button>
                        </div>
                      </SettingField>
                    </fieldset>
                  </section>
                  <section
                    className="settings-group"
                    aria-label={t("Theme settings")}
                  >
                    <h2>{t("Theme")}</h2>
                    <fieldset disabled={!snapshot.valid}>
                      <SettingField
                        title={t("Appearance")}
                        settingId="ui.theme"
                        summary={t(
                          "Choose Light, Dark, or follow your system appearance.",
                        )}
                      >
                        <SegmentedControl
                          label={t("Appearance")}
                          value={resolveTheme(snapshot.document["ui.theme"])}
                          options={[
                            { value: "light", label: t("Light") },
                            {
                              value: "dark",
                              label: t("Dark"),
                              muted: !Object.hasOwn(
                                snapshot.document,
                                "ui.theme",
                              ),
                            },
                            { value: "system", label: t("System") },
                          ]}
                          disabled={!snapshot.valid}
                          onChange={(value) => update("ui.theme", value)}
                        />
                      </SettingField>
                    </fieldset>
                  </section>
                  <section
                    className="settings-group"
                    aria-label={t("Terminal settings")}
                  >
                    <h2>{t("Terminal")}</h2>
                    <fieldset disabled={!snapshot.valid}>
                      <SettingField
                        title={t("Copy on selection")}
                        settingId="terminal.copyOnSelect"
                        summary={t(
                          "Automatically copy selected terminal text.",
                        )}
                        details={t(
                          "Dragging copies the final selection when you release the pointer. A notice confirms when clipboard access succeeds.",
                        )}
                      >
                        <Switch
                          label={t("Copy on selection")}
                          checked={
                            resolveTerminalSettings(snapshot.document)
                              .copyOnSelect
                          }
                          disabled={!snapshot.valid}
                          onChange={(checked) =>
                            update("terminal.copyOnSelect", checked)
                          }
                        />
                      </SettingField>
                    </fieldset>
                  </section>
                  <section
                    className="settings-group"
                    aria-label={t("Notifications")}
                  >
                    <h2>{t("Notifications")}</h2>
                    <SettingField
                      title={t("Notification sounds")}
                      settingId="notifications.sound"
                      summary={t(
                        "Play a sound when a response finishes or a new question arrives.",
                      )}
                      details={t(
                        "Sounds become available after interacting with the page. Tab indicators do not request notification permission.",
                      )}
                    >
                      <Switch
                        label={t("Notification sounds")}
                        checked={
                          snapshot.document["notifications.sound"] !== false
                        }
                        disabled={!snapshot.valid}
                        onChange={(checked) =>
                          update("notifications.sound", checked)
                        }
                      />
                    </SettingField>
                  </section>
                </>
              )}
            </div>
          </div>
        </section>
        <aside
          id="settings-file-editor"
          className="settings-file-pane"
          aria-label={t("Settings file editor")}
          ref={filePane}
          hidden={!jsonVisible}
        >
          <section className="settings-file">
            <header>
              <div>
                <h2>settings.json{dirty ? t(" · Modified") : ""}</h2>
                <small>{t("Settings file for this browser")}</small>
              </div>
              {!wide && (
                <Button onClick={() => setFileOpen(false)}>
                  {t("Back to settings")}
                </Button>
              )}
            </header>
            {jsonVisible && feedback}
            {stale && (
              <p role="alert">
                {t(
                  "The saved settings file has changed. Your draft was preserved. Reload before saving.",
                )}
              </p>
            )}
            <SourceEditor
              ref={fileInput}
              value={draft}
              path="settings.json"
              ariaLabel="settings.json"
              active={jsonVisible}
              onChange={(value) => {
                setDraft(value);
                setMessage("");
              }}
              onSave={save}
              onTabFocusChange={setTabMovesFocus}
            />
            <div className="settings-file-actions">
              <Button disabled={!dirty || stale} onClick={() => save()}>
                {t("Save")}
              </Button>
              <Button onClick={reload}>{t("Reload saved file")}</Button>
              <Button
                onClick={() => {
                  const url = URL.createObjectURL(
                    new Blob([snapshot.raw], { type: "application/json" }),
                  );
                  const link = document.createElement("a");
                  link.href = url;
                  link.download = "settings.json";
                  link.click();
                  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
                }}
              >
                {t("Export")}
              </Button>
              <Button onClick={() => upload.current!.click()}>
                {t("Import")}
              </Button>
            </div>
            <footer>
              <span>{t("JSON · Ctrl+Enter: Save")}</span>
              <span>
                {t("Ctrl+M: Tab moves focus")}{" "}
                {tabMovesFocus ? t("on") : t("off")}
              </span>
            </footer>
            <input
              ref={upload}
              hidden
              type="file"
              accept=".json,application/json"
              aria-label={t("settings.json Import")}
              onChange={async (event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (!file) return;
                try {
                  if (file.size > MAX_SETTINGS_BYTES)
                    throw new Error(t("Settings files are limited to 1 MiB."));
                  const raw = await file.text();
                  await localeStore.prepare(
                    resolveLocale(parseSettings(raw)["ui.language"]),
                  );
                  store.save(raw);
                  baseline.current = raw;
                  setDraft(raw);
                  setError("");
                  setMessage("Imported settings saved.");
                } catch (error) {
                  setError(String(error));
                }
              }}
            />
          </section>
        </aside>
      </div>
    </main>
  );
}
