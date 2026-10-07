import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useQuery } from "@lesomnus/payday/react";
import { ProjectService } from "../gen/cxz/project_svc_pb";
import type { Connection } from "./connection";
import { Button } from "./button";
import { openTerminal, type TerminalLink } from "./terminal-link";

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
  const project = useQuery(ProjectService.method.get, {
    ref: { key: { case: "id", value: projectId } },
    select: { all: true },
  });
  const screen = useRef<HTMLDivElement>(null);
  const link = useRef<TerminalLink | null>(null);
  const terminal = useRef<import("@xterm/xterm").Terminal | null>(null);
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
        theme: {
          background: "#111111",
          foreground: "#ededed",
          cursor: "#ededed",
          selectionBackground: "#444444",
          black: "#111111",
          red: "#aaaaaa",
          green: "#bbbbbb",
          yellow: "#cccccc",
          blue: "#aaaaaa",
          magenta: "#bbbbbb",
          cyan: "#cccccc",
          white: "#dddddd",
          brightBlack: "#777777",
          brightRed: "#eeeeee",
          brightGreen: "#eeeeee",
          brightYellow: "#eeeeee",
          brightBlue: "#eeeeee",
          brightMagenta: "#eeeeee",
          brightCyan: "#eeeeee",
          brightWhite: "#ffffff",
        },
      });
      const fitter = new FitAddon();
      term.loadAddon(fitter);
      term.open(screen.current!);
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
        observer.disconnect();
        data.dispose();
        sizes.dispose();
        transport.close();
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
      aria-label="Workspace terminal"
      hidden={!visible}
    >
      <header>
        <strong>Terminal</strong>
        <small role={project.error ? "alert" : "status"}>
          {project.error ? String(project.error) : state}
        </small>
        {ended && (
          <Button onClick={() => reconnect((value) => value + 1)}>
            Reconnect
          </Button>
        )}
        <Button
          className="toolbar-button"
          aria-label="Hide terminal"
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
