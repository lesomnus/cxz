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
  type EditorScope,
} from "./editor-settings";
import { SourceEditor, type SourceEditorHandle } from "./source-editor";

const fields = {
  indentSize: {
    label: "들여쓰기 칸 수",
    description: "Tab으로 넣거나 Shift+Tab으로 지울 공백 수",
    choices: Array.from({ length: 16 }, (_, i) => [
      String(i + 1),
      `${i + 1}칸`,
    ]),
  },
  insertSpaces: {
    label: "Tab 입력 방식",
    description: "공백을 넣을지 실제 Tab 문자를 넣을지 선택",
    choices: [
      ["true", "공백"],
      ["false", "Tab 문자"],
    ],
  },
  tabSize: {
    label: "Tab 문자 표시 폭",
    description: "실제 Tab 문자가 정렬되는 칸 수",
    choices: Array.from({ length: 16 }, (_, i) => [
      String(i + 1),
      `${i + 1}칸`,
    ]),
  },
  colorPalette: {
    label: "색상 팔레트",
    description: "코드의 syntax highlight 색상",
    choices: Object.entries(palettes).map(([key, palette]) => [
      key,
      palette.label,
    ]),
  },
};

export function SettingsPage({
  fileOpen,
  setFileOpen,
}: {
  fileOpen: boolean;
  setFileOpen: (open: boolean) => void;
}) {
  const { store, snapshot } = useSettings();
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
      {message && <p role="status">{message}</p>}
    </>
  );
  function update(key: string, value: unknown) {
    try {
      store.set(key, value);
      setError("");
      setMessage("저장되었습니다.");
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
  function save(raw = draft) {
    try {
      store.save(raw, baseline.current);
      baseline.current = raw;
      setDraft(raw);
      setError("");
      setMessage("저장되었습니다.");
    } catch (error) {
      setError(String(error));
    }
  }
  function group(scope: EditorScope) {
    const resolved = resolveEditorSettings(snapshot.document, scope);
    const prefix = scope === "session" ? "session.editor." : "editor.";
    const title = scope === "session" ? "세션 대화 에디터" : "전역 에디터";
    return (
      <section className="settings-group" aria-label={title}>
        <h2>{title}</h2>
        <p className="muted">
          {scope === "session"
            ? "값을 지정하지 않은 항목은 전역 에디터 설정을 상속합니다. Question 답변에도 적용됩니다."
            : "브라우저의 파일 뷰와 에디터가 공유하는 기본 설정입니다."}
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
                  <strong>{fields[key].label}</strong>
                  <small>{fields[key].description}</small>
                  <code>{name}</code>
                </span>
                <select
                  aria-label={`${title} ${fields[key].label}`}
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
                    {scope === "session" ? "전역 설정 상속" : "기본값"} ·{" "}
                    {label}
                  </option>
                  {fields[key].choices.map(([value, label]) => (
                    <option value={value} key={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </label>
            );
          })}
        </fieldset>
        <small>적용 중인 미리보기</small>
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
          aria-label="에디터 설정"
          hidden={!wide && fileOpen}
        >
          <div className="settings-editor-body">
            <header>
              <div>
                <h1>에디터</h1>
                <small>이 브라우저의 에디터 설정은 즉시 저장됩니다.</small>
              </div>
              {!wide && (
                <Button
                  aria-controls="settings-file-editor"
                  onClick={() => setFileOpen(true)}
                >
                  settings.json 편집{dirty ? " · 수정됨" : ""}
                </Button>
              )}
            </header>
            {!jsonVisible && feedback}
            <div className="settings-editor-groups">
              {group("global")}
              {group("session")}
            </div>
          </div>
        </section>
        <aside
          id="settings-file-editor"
          className="settings-file-pane"
          aria-label="Settings file editor"
          ref={filePane}
          hidden={!jsonVisible}
        >
          <section className="settings-file">
            <header>
              <div>
                <h2>settings.json{dirty ? " · 수정됨" : ""}</h2>
                <small>이 브라우저의 설정 파일</small>
              </div>
              {!wide && (
                <Button onClick={() => setFileOpen(false)}>
                  에디터 설정으로 돌아가기
                </Button>
              )}
            </header>
            {jsonVisible && feedback}
            {stale && (
              <p role="alert">
                저장된 설정 파일이 변경되었습니다. 수정 중인 내용은
                유지했습니다. 다시 읽은 뒤 저장하세요.
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
                저장
              </Button>
              <Button onClick={reload}>저장된 파일 다시 읽기</Button>
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
                내보내기
              </Button>
              <Button onClick={() => upload.current!.click()}>가져오기</Button>
            </div>
            <footer>
              <span>JSON · Ctrl+Enter: 저장</span>
              <span>
                Ctrl+M: Tab 포커스 이동 {tabMovesFocus ? "켜짐" : "꺼짐"}
              </span>
            </footer>
            <input
              ref={upload}
              hidden
              type="file"
              accept=".json,application/json"
              aria-label="settings.json 가져오기"
              onChange={async (event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (!file) return;
                try {
                  if (file.size > MAX_SETTINGS_BYTES)
                    throw new Error(
                      "설정 파일은 1 MiB까지 저장할 수 있습니다.",
                    );
                  const raw = await file.text();
                  store.save(raw);
                  baseline.current = raw;
                  setDraft(raw);
                  setError("");
                  setMessage("가져온 설정을 저장했습니다.");
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
