import { useTheme } from "#src/shared/theme/theme.tsx";
import { t, translateKnown } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useQuery } from "@lesomnus/payday/react";
import { ProjectService } from "#gen/cxz/project_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { Button } from "@lesomnus/cxz-ui";
import { openTerminal, type TerminalLink } from "./terminal-link";
import { enableTerminalWebGL } from "./terminal-renderer";
import { terminalTheme } from "./terminal-theme";
import { installTerminalSelectionCopy } from "./terminal-selection-copy";
import { useSettings } from "#src/shared/settings/settings.ts";
import { resolveTerminalSettings } from "#src/shared/settings/terminal-settings.ts";

export function terminalShortcut(
  event: Pick<
    KeyboardEvent,
    | "ctrlKey"
    | "altKey"
    | "metaKey"
    | "shiftKey"
    | "code"
    | "key"
    | "isComposing"
  >,
) {
  return (
    event.ctrlKey &&
    !event.altKey &&
    !event.metaKey &&
    !event.shiftKey &&
    !event.isComposing &&
    (event.code === "Backquote" || event.key === "`")
  );
}

export function WorkspaceTerminal({
  c,
  projectId,
  visible,
  hide,
}: {
  c: Connection;
  projectId: Uint8Array;
  visible: boolean;
  hide: () => void;
}) {
  useLocale();
  const project = useQuery(ProjectService.method.get, {
    ref: { key: { case: "id", value: projectId } },
    select: { all: true },
  });
  const theme = useTheme();
  const { snapshot } = useSettings();
  const preferences = useRef(resolveTerminalSettings(snapshot.document));
  preferences.current = resolveTerminalSettings(snapshot.document);
  const [clipboardNotice, setClipboardNotice] = useState<
    "copied" | "failed" | ""
  >("");
  const noticeTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );
  const currentTheme = useRef(theme);
  currentTheme.current = theme;
  const screen = useRef<HTMLDivElement>(null);
  const link = useRef<TerminalLink | null>(null);
  const terminal = useRef<import("@xterm/xterm").Terminal | null>(null);
  useEffect(() => {
    if (terminal.current) terminal.current.options.theme = terminalTheme(theme);
  }, [theme]);
  const fit = useRef<() => void>(() => {});
  const [generation, reconnect] = useState(0);
  const [state, setState] = useState("Connecting…");
  const [ended, setEnded] = useState(false);
  const runtimeId = project.data?.runtimeId;
  const showing = useRef(visible);
  showing.current = visible;
  useEffect(() => {
    if (!runtimeId || !screen.current) return;
    let disposed = false;
    let cleanup = () => {};
    setState("Connecting…");
    setEnded(false);
    setClipboardNotice("");
    void (async () => {
      const [{ Terminal }, { FitAddon }] = await Promise.all([
        import("@xterm/xterm"),
        import("@xterm/addon-fit"),
        import("@xterm/xterm/css/xterm.css"),
      ]);
      if (disposed) return;
      const term = new Terminal({
        cursorBlink: true,
        fontSize: 12,
        fontFamily: "ui-monospace, SFMono-Regular, Consolas, monospace",
        scrollback: 2000,
        allowProposedApi: false,
        theme: terminalTheme(currentTheme.current),
      });
      const fitter = new FitAddon();
      term.loadAddon(fitter);
      term.open(screen.current!);
      const disposeRenderer = enableTerminalWebGL(term);
      const disposeCopy = installTerminalSelectionCopy(
        term,
        screen.current!,
        () => preferences.current.copyOnSelect,
        (result) => {
          clearTimeout(noticeTimer.current);
          setClipboardNotice(result);
          noticeTimer.current = setTimeout(() => setClipboardNotice(""), 2200);
        },
      );
      // Install cleanup before opening the transport, including partial setup.
      cleanup = () => {
        clearTimeout(noticeTimer.current);
        disposeCopy();
        disposeRenderer();
        term.dispose();
      };
      const resize = () => {
        if (!screen.current?.clientWidth || !screen.current?.clientHeight)
          return;
        const size = fitter.proposeDimensions();
        if (size)
          term.resize(
            Math.max(1, Math.min(500, size.cols)),
            Math.max(1, Math.min(100, size.rows)),
          );
      };
      resize();
      terminal.current = term;
      fit.current = resize;
      const transport = openTerminal(
        c,
        runtimeId,
        term.cols,
        term.rows,
        (bytes, consumed) => {
          if (!disposed) term.write(bytes, consumed);
          else consumed();
        },
        (frame) => {
          if (disposed) return;
          if (frame.ready) {
            setState(
              c.inProcess
                ? "Simulated · /workspace"
                : project.data?.status?.remoteWorkspace || "Connected",
            );
            resize();
            if (showing.current) term.focus();
          }
          if (frame.exited || frame.error) {
            setEnded(true);
            setState(frame.error || "Shell exited");
          }
        },
      );
      link.current = transport;
      const data = term.onData((value) => {
        try {
          transport.input(value);
        } catch (error) {
          transport.close();
          setState(String(error));
          setEnded(true);
        }
      });
      const sizes = term.onResize(({ cols, rows }) =>
        transport.resize(cols, rows),
      );
      const observer = new ResizeObserver(resize);
      observer.observe(screen.current!);
      cleanup = () => {
        clearTimeout(noticeTimer.current);
        disposeCopy();
        observer.disconnect();
        data.dispose();
        sizes.dispose();
        transport.close();
        disposeRenderer();
        term.dispose();
        terminal.current = null;
        link.current = null;
      };
    })().catch((error) => {
      if (!disposed) {
        setState(String(error));
        setEnded(true);
      }
    });
    return () => {
      disposed = true;
      cleanup();
    };
  }, [c, runtimeId, generation]);
  useLayoutEffect(() => {
    if (!visible) return;
    fit.current();
    terminal.current?.focus();
  }, [visible]);
  return (
    <section
      className="workspace-terminal"
      aria-label={t("Workspace terminal")}
      hidden={!visible}
    >
      <header>
        <strong>{t("Terminal")}</strong>
        <small role={project.error ? "alert" : "status"}>
          {project.error ? String(project.error) : translateKnown(state)}
        </small>
        <span
          className="terminal-clipboard-feedback"
          role="status"
          aria-live="polite"
          title={
            clipboardNotice === "failed"
              ? t(
                  "Clipboard access failed. Use the terminal context menu to copy.",
                )
              : undefined
          }
        >
          {clipboardNotice === "copied"
            ? t("Copied")
            : clipboardNotice === "failed"
              ? t("Copy failed")
              : ""}
        </span>
        {ended && (
          <Button onClick={() => reconnect((value) => value + 1)}>
            {t("Reconnect")}
          </Button>
        )}
        <Button
          className="toolbar-button"
          aria-label={t("Hide terminal")}
          title="Ctrl+`"
          onClick={hide}
        >
          ⌄
        </Button>
      </header>
      <div className="terminal-screen" ref={screen} />
    </section>
  );
}
