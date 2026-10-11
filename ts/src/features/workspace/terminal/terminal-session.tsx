import { useTheme } from "#src/shared/theme/theme.tsx";
import {
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type Ref,
} from "react";
import { createPortal } from "react-dom";
import type { Connection } from "#src/shared/api/connection.ts";
import { openTerminal, type TerminalLink } from "./terminal-link";
import { enableTerminalWebGL } from "./terminal-renderer";
import { terminalTheme } from "./terminal-theme";
import { installTerminalSelectionCopy } from "./terminal-selection-copy";
import { installTerminalKeyboard } from "./terminal-keyboard";
import { useSettings } from "#src/shared/settings/settings.ts";
import { resolveTerminalSettings } from "#src/shared/settings/terminal-settings.ts";
import styles from "./workspace-terminal.module.css";

export type TerminalSessionStatus = {
  state: string;
  ended: boolean;
  clipboardNotice: "copied" | "failed" | "";
};
export type TerminalSessionHandle = {
  focus: () => void;
  reconnect: () => void;
};

// Each shell stays in this flat keyed list; docking only reparents its portal host.
// React pane remounts and tab switches must never reopen the PTY or clear its buffer.
export function TerminalSession({
  c,
  runtimeId,
  id,
  groupId,
  active,
  focused,
  visible,
  placement,
  getHost,
  onStatus,
  onFocus,
  ref,
}: {
  c: Connection;
  runtimeId?: string;
  id: string;
  groupId: string;
  active: boolean;
  focused: boolean;
  visible: boolean;
  placement: unknown;
  getHost: (group: string) => HTMLElement | null;
  onStatus: (id: string, status: TerminalSessionStatus) => void;
  onFocus: (id: string) => void;
  ref?: Ref<TerminalSessionHandle>;
}) {
  const [mount] = useState(() => {
    const el = document.createElement("div");
    el.className = styles.surface;
    return el;
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
  const showing = useRef(visible);
  showing.current = visible && focused;
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
      const disposeKeyboard = installTerminalKeyboard(term);
      // Install cleanup before opening the transport, including partial setup.
      cleanup = () => {
        clearTimeout(noticeTimer.current);
        disposeKeyboard();
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
            setState("");
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
        disposeKeyboard();
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
    if (visible && active && focused) {
      fit.current();
      terminal.current?.focus();
    }
  }, [visible, active, focused]);
  useImperativeHandle(
    ref,
    () => ({
      focus: () => terminal.current?.focus(),
      reconnect: () => reconnect((value) => value + 1),
    }),
    [],
  );
  useEffect(
    () => onStatus(id, { state, ended, clipboardNotice }),
    [id, state, ended, clipboardNotice, onStatus],
  );
  useLayoutEffect(() => {
    const host = getHost(groupId);
    const restoreFocus = mount.contains(document.activeElement);
    if (host && mount.parentElement !== host) host.append(mount);
    mount.hidden = !active;
    if (visible && active) fit.current();
    if (restoreFocus && visible && active) terminal.current?.focus();
  }, [groupId, active, visible, placement, getHost, mount]);
  // Remove the portal host in the same effect phase that attaches it. StrictMode
  // replays passive cleanup after layout setup, which would detach a live shell.
  useLayoutEffect(() => () => mount.remove(), [mount]);
  return createPortal(
    <div
      className={`terminal-screen ${styles.screen}`}
      data-terminal-state={ended ? "ended" : state ? "connecting" : "ready"}
      ref={screen}
      onFocusCapture={() => onFocus(id)}
    />,
    mount,
  );
}
