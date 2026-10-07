import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import type { Project } from "../gen/cxz/project_pb";
import type { ProjectPathEntry } from "../gen/cxz/project_svc_pb";
import { Connection, ref } from "./connection";
import { Button } from "./button";
import type * as Monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import { SourceEditor } from "./source-editor";

type File = {
  path: string;
  text: string;
  view?: Monaco.editor.ICodeEditorViewState | null;
};
type Snapshot = {
  files: File[];
  active: string;
  phase: "idle" | "connecting" | "connected";
  simulated: boolean;
  error: string;
  workspace: string;
};

export class EditorState {
  value: Snapshot = {
    files: [],
    active: "",
    phase: "idle",
    simulated: false,
    error: "",
    workspace: "",
  };
  listeners = new Set<() => void>();
  snapshot = () => this.value;
  subscribe = (callback: () => void) => {
    this.listeners.add(callback);
    return () => {
      this.listeners.delete(callback);
    };
  };
  update(patch: Partial<Snapshot>) {
    this.value = { ...this.value, ...patch };
    this.listeners.forEach((f) => f());
  }
}

export function WorkspaceEditor({
  c,
  project,
}: {
  c: Connection;
  project: Project;
}) {
  const id = project.runtimeId;
  const [state] = useState(() => {
    let state = c.editors.get(id);
    if (!state) {
      state = new EditorState();
      c.editors.set(id, state);
    }
    return state;
  });
  const value = useSyncExternalStore(state.subscribe, state.snapshot);
  const root = project.status?.remoteWorkspace || "";
  const [expanded, setExpanded] = useState(new Set<string>([root]));
  const [directories, setDirectories] = useState(
    new Map<string, ProjectPathEntry[]>(),
  );
  const [loading, setLoading] = useState(new Set<string>());
  const [browseError, setBrowseError] = useState("");
  const pending = useRef(new Map<string, AbortController>());
  const connectRequest = useRef<AbortController | null>(null);
  const [frame, setFrame] = useState(0);
  useEffect(
    () => () => {
      pending.current.forEach((c) => c.abort());
      connectRequest.current?.abort();
      if (state.value.phase === "connecting") state.update({ phase: "idle" });
    },
    [state],
  );

  async function list(path: string) {
    if (pending.current.has(path)) return;
    const controller = new AbortController();
    pending.current.set(path, controller);
    setLoading((old) => new Set(old).add(path));
    setBrowseError("");
    try {
      const entries: ProjectPathEntry[] = [];
      for await (const reply of c.projects.paths(
        { ref: ref(id), path },
        { signal: controller.signal },
      )) {
        entries.push(
          ...reply.entries.filter(
            (e) =>
              e.name !== "." && e.name !== ".." && !/[\/\x00]/.test(e.name),
          ),
        );
        if (entries.length > 2048)
          throw new Error("Directory preview is limited to 2048 entries.");
        if (reply.truncated) setBrowseError("Directory preview was truncated.");
      }
      entries.sort(
        (a, b) =>
          Number(b.directory) - Number(a.directory) ||
          a.name.localeCompare(b.name),
      );
      if (!controller.signal.aborted)
        setDirectories((old) => new Map(old).set(path, entries));
    } catch (e) {
      if (!controller.signal.aborted) setBrowseError(String(e));
    } finally {
      pending.current.delete(path);
      setLoading((old) => {
        const n = new Set(old);
        n.delete(path);
        return n;
      });
    }
  }
  useEffect(() => {
    if (root) void list(root);
  }, [root]);
  async function open(path: string) {
    if (state.value.files.some((f) => f.path === path)) {
      state.update({ active: path });
      return;
    }
    if (pending.current.has(path)) return;
    const controller = new AbortController();
    pending.current.set(path, controller);
    setBrowseError("");
    try {
      const chunks: Uint8Array[] = [];
      let size = 0;
      for await (const reply of c.projects.download(
        { ref: ref(id), path },
        { signal: controller.signal },
      )) {
        size += reply.data.length;
        if (size > 1048576 || reply.totalSize > 1048576n)
          throw new Error("File preview is limited to 1 MiB.");
        chunks.push(reply.data);
      }
      const bytes = new Uint8Array(size);
      let offset = 0;
      for (const chunk of chunks) {
        bytes.set(chunk, offset);
        offset += chunk.length;
      }
      if (bytes.includes(0))
        throw new Error("Binary files cannot be previewed.");
      const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
      if (!controller.signal.aborted)
        state.update({
          files: [...state.value.files.slice(-15), { path, text }],
          active: path,
        });
    } catch (e) {
      if (!controller.signal.aborted) setBrowseError(String(e));
    } finally {
      pending.current.delete(path);
    }
  }
  async function connect() {
    if (state.value.phase === "connecting") return;
    const controller = new AbortController();
    connectRequest.current = controller;
    state.update({ phase: "connecting", error: "" });
    try {
      const result = await c.projects.editor(
        { ref: ref(id) },
        { signal: controller.signal, timeoutMs: 180000 },
      );
      if (!controller.signal.aborted) {
        state.update({
          phase: "connected",
          simulated: result.simulated,
          workspace: result.workspace,
        });
        setFrame((n) => n + 1);
      }
    } catch (e) {
      if (!controller.signal.aborted)
        state.update({ phase: "idle", error: String(e) });
    } finally {
      if (connectRequest.current === controller) connectRequest.current = null;
    }
  }
  const active = value.files.find((f) => f.path === value.active);
  function tree(path: string, depth: number): React.ReactNode {
    return directories.get(path)?.map((entry) => {
      const target = path.replace(/\/$/, "") + "/" + entry.name;
      const isOpen = expanded.has(target);
      return (
        <div key={target}>
          <Button
            className={`file-entry ${value.active === target ? "active" : ""}`}
            style={{ paddingInlineStart: 8 + depth * 12 }}
            aria-expanded={entry.directory ? isOpen : undefined}
            onClick={() => {
              if (entry.directory) {
                setExpanded((old) => {
                  const n = new Set(old);
                  if (n.has(target)) n.delete(target);
                  else n.add(target);
                  return n;
                });
                if (!directories.has(target)) void list(target);
              } else void open(target);
            }}
          >
            <span className="file-entry-symbol" aria-hidden="true">
              {entry.directory ? (isOpen ? "⌄" : "›") : ""}
            </span>
            <span>{entry.name}</span>
          </Button>
          {entry.directory && isOpen && tree(target, depth + 1)}
          {loading.has(target) && (
            <small className="file-loading">Loading…</small>
          )}
        </div>
      );
    });
  }
  return (
    <aside className="workspace-editor" aria-label="Workspace editor">
      <header>
        <div>
          <strong>{project.name || project.alias}</strong>
          <small>
            {value.phase === "connecting"
              ? "Connecting…"
              : value.phase === "connected"
                ? value.simulated
                  ? "Simulated connection"
                  : "Connected"
                : "File preview"}{" "}
            · {value.workspace || root || "Workspace unavailable"}
          </small>
        </div>
        {value.phase === "connected" ? (
          <Button onClick={() => state.update({ phase: "idle", error: "" })}>
            Disconnect
          </Button>
        ) : (
          <Button
            disabled={value.phase === "connecting"}
            onClick={() => void connect()}
          >
            Connect
          </Button>
        )}
      </header>
      {value.error && (
        <p className="editor-error" role="alert">
          {value.error}
        </p>
      )}
      {value.phase === "connected" && !value.simulated ? (
        <iframe
          key={frame}
          title="VS Code workspace"
          src={`/editor/${encodeURIComponent(id)}/?folder=${encodeURIComponent(value.workspace)}`}
        />
      ) : (
        <div className="file-workbench">
          <nav className="file-explorer" aria-label="Workspace files">
            <div className="file-explorer-heading">
              <span>Files</span>
              <Button
                aria-label="Refresh files"
                onClick={() => {
                  setDirectories(new Map());
                  if (root) void list(root);
                }}
              >
                ↻
              </Button>
            </div>
            {tree(root, 0)}
            {loading.has(root) && <small>Loading files…</small>}
            {!root && <small>Workspace unavailable.</small>}
          </nav>
          <div className="file-content">
            <div className="file-tabs" role="tablist" aria-label="Open files">
              {value.files.map((file) => (
                <div className="file-tab" key={file.path}>
                  <Button
                    role="tab"
                    aria-selected={file.path === value.active}
                    title={file.path}
                    onClick={() => state.update({ active: file.path })}
                  >
                    {file.path.split("/").at(-1)}
                  </Button>
                  <Button
                    aria-label={`Close ${file.path.split("/").at(-1)}`}
                    onClick={() => {
                      const files = state.value.files.filter(
                        (f) => f.path !== file.path,
                      );
                      state.update({
                        files,
                        active:
                          state.value.active === file.path
                            ? (files.at(-1)?.path ?? "")
                            : state.value.active,
                      });
                    }}
                  >
                    ×
                  </Button>
                </div>
              ))}
            </div>
            {active ? (
              <FileEditor key={active.path} file={active} />
            ) : (
              <div className="file-empty">Select a file to preview.</div>
            )}
            {browseError && (
              <p className="editor-error" role="alert">
                {browseError}
              </p>
            )}
            <footer>
              <span>{active?.path.replace(root + "/", "") || ""}</span>
              <span>
                Read only
                {value.simulated && value.phase === "connected"
                  ? " · Simulated"
                  : ""}
              </span>
            </footer>
          </div>
        </div>
      )}
    </aside>
  );
}

function FileEditor({ file }: { file: File }) {
  return (
    <SourceEditor
      value={file.text}
      path={file.path}
      ariaLabel={`File preview: ${file.path}`}
      readOnly
      view={file.view}
      onDispose={(view) => {
        file.view = view;
      }}
    />
  );
}
