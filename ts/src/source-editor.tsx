import { useTheme } from "./theme";
import {
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type Ref,
} from "react";
import type * as Monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import { useEditorSettings } from "./settings";

export type SourceEditorHandle = { focus: () => void };

// File previews and editable settings share the same lazy Monaco surface.
export function SourceEditor({
  value,
  path,
  ariaLabel,
  readOnly = false,
  active = true,
  ref,
  onChange,
  onSave,
  onTabFocusChange,
  view,
  onDispose,
}: {
  value: string;
  path: string;
  ariaLabel: string;
  readOnly?: boolean;
  active?: boolean;
  ref?: Ref<SourceEditorHandle>;
  onChange?: (value: string) => void;
  onSave?: (value: string) => void;
  onTabFocusChange?: (enabled: boolean) => void;
  view?: Monaco.editor.ICodeEditorViewState | null;
  onDispose?: (view: Monaco.editor.ICodeEditorViewState | null) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const editor = useRef<Monaco.editor.IStandaloneCodeEditor | undefined>(
    undefined,
  );
  const model = useRef<Monaco.editor.ITextModel | undefined>(undefined);
  const configure = useRef<
    | ((
        settings: ReturnType<typeof useEditorSettings>,
        theme: "light" | "dark",
      ) => void)
    | undefined
  >(undefined);
  const [activated, setActivated] = useState(active);
  const [error, setError] = useState("");
  const settings = useEditorSettings();
  const theme = useTheme();
  const latest = useRef({
    value,
    ariaLabel,
    active,
    onChange,
    onSave,
    onTabFocusChange,
    view,
    onDispose,
    settings,
    theme,
  });
  latest.current = {
    value,
    ariaLabel,
    active,
    onChange,
    onSave,
    onTabFocusChange,
    view,
    onDispose,
    settings,
    theme,
  };
  const pendingFocus = useRef(false);
  const updating = useRef(false);
  useImperativeHandle(
    ref,
    () => ({
      focus: () => {
        if (editor.current) editor.current.focus();
        else pendingFocus.current = true;
      },
    }),
    [],
  );
  useEffect(() => {
    if (active) {
      setActivated(true);
      editor.current?.layout();
    } else pendingFocus.current = false;
  }, [active]);
  useEffect(() => {
    configure.current?.(settings, theme);
  }, [
    settings.indentSize,
    settings.insertSpaces,
    settings.tabSize,
    settings.colorPalette,
    theme,
  ]);
  useEffect(() => {
    editor.current?.updateOptions({ ariaLabel });
  }, [ariaLabel]);
  useEffect(() => {
    const m = model.current;
    const e = editor.current;
    if (!m || !e || m.getValue() === value) return;
    const position = e.saveViewState();
    updating.current = true;
    m.setValue(value);
    updating.current = false;
    e.restoreViewState(position);
  }, [value]);
  useEffect(() => {
    if (!activated) return;
    let canceled = false;
    let e: Monaco.editor.IStandaloneCodeEditor | undefined;
    let m: Monaco.editor.ITextModel | undefined;
    const subscriptions: Monaco.IDisposable[] = [];
    import("./code-editor")
      .then(({ monaco, language, configureEditor, editorOptions }) => {
        if (canceled) return;
        m = monaco.editor.createModel(latest.current.value, language(path));
        e = monaco.editor.create(host.current!, {
          ...editorOptions,
          model: m,
          readOnly,
          domReadOnly: readOnly,
          ariaLabel: latest.current.ariaLabel,
          theme: `cxz-${latest.current.settings.colorPalette}-${latest.current.theme}`,
        });
        model.current = m;
        editor.current = e;
        configure.current = (settings, theme) =>
          configureEditor(e!, m!, settings, theme);
        configure.current(latest.current.settings, latest.current.theme);
        if (latest.current.view) e.restoreViewState(latest.current.view);
        subscriptions.push(
          m.onDidChangeContent(() => {
            if (!updating.current) latest.current.onChange?.(m!.getValue());
          }),
        );
        if (!readOnly) {
          const control = /Mac|iPhone|iPad/.test(navigator.platform)
            ? monaco.KeyMod.WinCtrl
            : monaco.KeyMod.CtrlCmd;
          e.addCommand(control | monaco.KeyCode.Enter, () =>
            latest.current.onSave?.(m!.getValue()),
          );
          e.addCommand(control | monaco.KeyCode.KeyM, () => {
            e!.updateOptions({
              tabFocusMode: !e!.getOption(
                monaco.editor.EditorOption.tabFocusMode,
              ),
            });
          });
          subscriptions.push(
            e.onDidChangeConfiguration((event) => {
              if (event.hasChanged(monaco.editor.EditorOption.tabFocusMode))
                latest.current.onTabFocusChange?.(
                  e!.getOption(monaco.editor.EditorOption.tabFocusMode),
                );
            }),
          );
        }
        if (pendingFocus.current && latest.current.active) e.focus();
        pendingFocus.current = false;
      })
      .catch((error) => {
        if (!canceled) setError(String(error));
      });
    return () => {
      canceled = true;
      if (e) latest.current.onDispose?.(e.saveViewState());
      subscriptions.forEach((subscription) => subscription.dispose());
      configure.current = undefined;
      editor.current = undefined;
      model.current = undefined;
      e?.dispose();
      m?.dispose();
    };
  }, [activated, path, readOnly]);
  return (
    <div className="code-editor" ref={host}>
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
