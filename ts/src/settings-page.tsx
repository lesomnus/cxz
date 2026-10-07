import {
  t,
  translateKnown,
  languages,
  localeStore,
  resolveLocale,
  type Locale,
} from "./i18n";
import { useLocale } from "./i18n-react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Button } from "./button";
import { useSettings } from "./settings";
import {
  defaultEditorSettings,
  editorKeys,
  MAX_SETTINGS_BYTES,
  palettes,
  paletteVariables,
  resolveEditorSettings,
  parseSettings,
  type EditorScope,
} from "./editor-settings";
import { SourceEditor, type SourceEditorHandle } from "./source-editor";

const fields = {
  indentSize: {
    label: "Indentation size",
    description: "Spaces inserted with Tab or removed with Shift+Tab",
    choices: Array.from({ length: 16 }, (_, i) => [
      String(i + 1),
      `${i + 1} columns`,
    ]),
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
    choices: Array.from({ length: 16 }, (_, i) => [
      String(i + 1),
      `${i + 1} columns`,
    ]),
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
  topic: "editor" | "language";
}) {
  const locale = useLocale();
  const { store, snapshot } = useSettings();
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
    const resolved = resolveEditorSettings(snapshot.document, scope);
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
            const label = fields[key].choices.find(
              ([value]) => value === String(inherited),
            )![1];
            return (
              <label className="setting-row" key={key}>
                <span>
                  <strong>{translateKnown(fields[key].label)}</strong>
                  <small>{translateKnown(fields[key].description)}</small>
                  <code>{name}</code>
                </span>
                <select
                  aria-label={`${title} ${translateKnown(fields[key].label)}`}
                  value={hasValue ? String(snapshot.document[name]) : ""}
                  onChange={(event) => {
                    const value = event.target.value;
                    update(
                      name,
                      value === ""
                        ? undefined
                        : key === "insertSpaces"
                          ? value === "true"
                          : key === "colorPalette"
                            ? value
                            : Number(value),
                    );
                  }}
                >
                  <option value="">
                    {scope === "session" ? t("Inherit global") : t("Default")} ·{" "}
                    {key === "indentSize" || key === "tabSize"
                      ? t("{count} columns", { count: Number(inherited) })
                      : translateKnown(label)}
                  </option>
                  {fields[key].choices.map(([value, label]) => (
                    <option value={value} key={value}>
                      {key === "indentSize" || key === "tabSize"
                        ? t("{count} columns", { count: Number(value) })
                        : translateKnown(label)}
                    </option>
                  ))}
                </select>
              </label>
            );
          })}
        </fieldset>
        <small>{t("Current preview")}</small>
        <pre
          className="settings-preview"
          style={{
            tabSize: resolved.tabSize,
            ...paletteVariables(resolved.colorPalette),
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
            topic === "editor" ? t("Editor settings") : t("Language settings")
          }
          hidden={!wide && fileOpen}
        >
          <div className="settings-editor-body">
            <header>
              <div>
                <h1>{topic === "editor" ? t("Editor") : t("Language")}</h1>
                <small>
                  {topic === "editor"
                    ? t("Editor changes are saved immediately in this browser.")
                    : t(
                        "Choose a language, then apply it to download its language pack.",
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
                <section className="settings-group">
                  <h2>{t("Display language")}</h2>
                  <p className="muted">
                    {t(
                      "Language changes apply to this browser. Conversation content and code are preserved.",
                    )}
                  </p>
                  <fieldset disabled={!snapshot.valid || applying}>
                    <label className="setting-row">
                      <span>
                        <strong>{t("Language")}</strong>
                        <code>ui.language</code>
                      </span>
                      <select
                        aria-label={t("Display language")}
                        value={language}
                        onChange={(event) =>
                          setLanguage(event.target.value as Locale)
                        }
                      >
                        {languages.map(({ id, name }) => (
                          <option key={id} value={id}>
                            {name}
                          </option>
                        ))}
                      </select>
                    </label>
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
                  </fieldset>
                </section>
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
