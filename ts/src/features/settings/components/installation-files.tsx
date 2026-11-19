import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  ActionMenu,
  Button,
  Dialog,
  DialogBody,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Switch,
  ValueMenu,
} from "@lesomnus/nox";
import type { Connection } from "#src/shared/api/connection.ts";
import { SourceEditor } from "#src/shared/editor/source-editor.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { Icon } from "#src/shared/icons/icon.tsx";
import styles from "./installation-files.module.css";
import tableStyles from "#src/shared/components/resource-table.module.css";

type Kind = "compose" | "template";
type WireFile = { data: string | null; executable?: boolean };
type Source = {
  revision: string;
  files: Record<string, WireFile>;
  builtin?: boolean;
};
type DraftFile = WireFile & {
  text?: string;
  edited?: boolean;
  crlf?: boolean;
  bom?: boolean;
};
type ApplyResult = { revision: string; published: boolean; error?: string };

const title = (kind: Kind) =>
  kind === "compose"
    ? t("Compose override")
    : t("Default devcontainer template");

async function request<T>(
  c: Connection,
  kind: Kind,
  init?: RequestInit,
): Promise<T> {
  const response = await fetch(
    `${c.baseUrl.replace(/\/$/, "")}/configuration/${kind}`,
    { cache: "no-store", ...init },
  );
  if (!response.ok) throw new Error((await response.text()).trim());
  if (!response.headers.get("Content-Type")?.includes("application/json"))
    throw new Error(
      t("This gateway does not support installation file editing."),
    );
  return response.json();
}

function unpack(files: Source["files"]) {
  const decoder = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true });
  const result = new Map<string, DraftFile>();
  for (const [name, file] of Object.entries(files).sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    const bytes = Uint8Array.from(atob(file.data ?? ""), (c) =>
      c.charCodeAt(0),
    );
    try {
      const text = bytes.includes(0) ? undefined : decoder.decode(bytes);
      result.set(name, {
        ...file,
        text,
        crlf: text?.match(/\r?\n/)?.[0] === "\r\n",
        bom: text?.startsWith("\uFEFF"),
      });
    } catch {
      result.set(name, file);
    }
  }
  return result;
}

function pack(file: DraftFile): WireFile {
  if (file.text === undefined || !file.edited)
    return { data: file.data ?? "", executable: file.executable };
  const text = file.text.replace(/\r?\n/g, file.crlf ? "\r\n" : "\n");
  const bom = file.bom && !text.startsWith("\uFEFF") ? "\uFEFF" : "";
  const bytes = new TextEncoder().encode(bom + text);
  let raw = "";
  for (let offset = 0; offset < bytes.length; offset += 8192)
    raw += String.fromCharCode(...bytes.subarray(offset, offset + 8192));
  return { data: btoa(raw), executable: file.executable };
}

export function InstallationFileMenu({
  c,
  name,
  active,
}: {
  c: Connection;
  name: string;
  active: boolean;
}) {
  const [kind, setKind] = useState<Kind>();
  return (
    <>
      <ActionMenu
        label={t("Controls for {name}", { name })}
        triggerClassName={tableStyles.rowActionTrigger}
        motion={{ from: "top-right", effect: "reveal" }}
        disabled={!active || c.inProcess}
        disabledReason={t(
          "Installation file editing requires a host connection.",
        )}
        groups={[
          {
            label: t("Installation files"),
            items: (["compose", "template"] as const).map((kind) => ({
              label: title(kind),
              icon: (
                <Icon
                  name={kind === "compose" ? "document" : "folder"}
                  aria-hidden="true"
                />
              ),
              run: () => setKind(kind),
            })),
          },
        ]}
      />
      {kind && (
        <InstallationEditor
          c={c}
          kind={kind}
          close={() => setKind(undefined)}
        />
      )}
    </>
  );
}

function InstallationEditor({
  c,
  kind,
  close,
}: {
  c: Connection;
  kind: Kind;
  close: () => void;
}) {
  const [open, setOpen] = useState(true);
  const [source, setSource] = useState<Source>();
  const [files, setFiles] = useState(() => new Map<string, DraftFile>());
  const [selected, select] = useState("");
  const [newName, setNewName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [unpublished, setUnpublished] = useState(false);
  const [restoreBuiltin, setRestoreBuiltin] = useState(false);
  const [reload, setReload] = useState(0);
  const requestRef = useRef<AbortController | null>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const activeFile = files.get(selected);
  const isTemplate = kind === "template";
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    request<Source>(c, kind, { signal: controller.signal })
      .then((current) => {
        if (controller.signal.aborted) return;
        const draft = unpack(current.files);
        setSource(current);
        setFiles(draft);
        select(
          draft.has("devcontainer.json")
            ? "devcontainer.json"
            : (draft.keys().next().value ?? ""),
        );
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setError(String(error));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [c, kind, reload]);
  useEffect(() => () => requestRef.current?.abort(), []);

  function change(name: string, file: DraftFile) {
    setFiles((current) => new Map(current).set(name, file));
  }
  function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = newName.trim();
    if (
      !name ||
      files.has(name) ||
      /[\\:\x00-\x1f]/u.test(name) ||
      name.split("/").some((part) => !part || part === "." || part === "..")
    ) {
      setError(t("Enter a new relative file path."));
      return;
    }
    change(name, { data: "", text: "" });
    select(name);
    setNewName("");
    setError("");
  }
  function remove() {
    const next = new Map(files);
    next.delete(selected);
    setFiles(next);
    select(next.keys().next().value ?? "");
  }
  async function apply() {
    if (!source || requestRef.current) return;
    const next = restoreBuiltin ? new Map<string, DraftFile>() : files;
    const controller = new AbortController();
    requestRef.current = controller;
    setBusy(true);
    setError("");
    try {
      const result = await request<ApplyResult>(c, kind, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          revision: source.revision,
          files: Object.fromEntries(
            [...next].map(([path, file]) => [path, pack(file)]),
          ),
        }),
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      if (result.published) {
        setOpen(false);
      } else {
        setSource({ ...source, revision: result.revision });
        setFiles(next);
        if (!next.has(selected)) select("");
        setUnpublished(true);
        setError(
          t("Files saved on the host. Manager publication failed.") +
            " " +
            (result.error ?? ""),
        );
      }
    } catch (error) {
      if (!controller.signal.aborted) setError(String(error));
    } finally {
      requestRef.current = null;
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <Dialog
      className={styles.dialog}
      open={open}
      onClose={() => setOpen(false)}
      onClosed={close}
      dismissible={!busy}
      initialFocus={closeRef}
    >
      <DialogHeader>
        <DialogTitle>{title(kind)}</DialogTitle>
      </DialogHeader>
      <DialogBody className={styles.body} aria-busy={loading || busy}>
        <small>{c.baseUrl}</small>
        {source?.builtin && (
          <small>{t("Editing a copy of the built-in template.")}</small>
        )}
        {loading && <div role="status">{t("Loading…")}</div>}
        {error && <ErrorNotice error={error} />}
        {!loading && !source && (
          <Button onClick={() => setReload((n) => n + 1)}>{t("Retry")}</Button>
        )}
        {restoreBuiltin && (
          <p>
            {t(
              "The built-in template will be restored when you apply changes.",
            )}
          </p>
        )}
        {isTemplate && source && !restoreBuiltin && (
          <>
            <div className={styles.actions}>
              {files.size > 0 && (
                <ValueMenu
                  label={t("Files")}
                  value={selected}
                  options={[...files.keys()].map((value) => ({
                    value,
                    label: value,
                  }))}
                  choose={select}
                  disabled={busy}
                />
              )}
              {activeFile && (
                <>
                  <Switch
                    label={t("Executable")}
                    checked={!!activeFile.executable}
                    onChange={(executable) =>
                      change(selected, { ...activeFile, executable })
                    }
                    disabled={busy}
                  />
                  <span>{t("Executable")}</span>
                  <Button
                    onClick={remove}
                    disabled={busy || selected === "devcontainer.json"}
                  >
                    {t("Delete")}
                  </Button>
                </>
              )}
            </div>
            <form className={styles.add} onSubmit={add}>
              <input
                aria-label={t("New file path")}
                placeholder="scripts/setup.sh"
                value={newName}
                maxLength={1024}
                onChange={(event) => setNewName(event.target.value)}
                disabled={busy}
              />
              <Button type="submit" disabled={busy || !newName.trim()}>
                {t("Add file")}
              </Button>
            </form>
          </>
        )}
        {source && activeFile && !restoreBuiltin && (
          <div className={styles.editor} inert={busy}>
            {activeFile.text === undefined ? (
              <p>
                {t(
                  "This binary file will be preserved when you apply changes.",
                )}
              </p>
            ) : (
              <SourceEditor
                key={selected}
                path={selected}
                ariaLabel={selected}
                value={activeFile.text}
                onChange={(text) =>
                  change(selected, { ...activeFile, text, edited: true })
                }
              />
            )}
          </div>
        )}
        {source && (
          <small>
            {t(
              "Changes take effect when project containers are created or recreated.",
            )}
          </small>
        )}
      </DialogBody>
      <DialogFooter>
        {isTemplate && source && !source.builtin && !unpublished && (
          <Button
            disabled={busy}
            onClick={() => setRestoreBuiltin(!restoreBuiltin)}
          >
            {restoreBuiltin ? t("Cancel") : t("Restore built-in template")}
          </Button>
        )}
        <Button
          disabled={
            busy || !source || (!files.size && !restoreBuiltin && !unpublished)
          }
          loading={busy}
          onClick={() => void apply()}
        >
          {unpublished ? t("Retry") : t("Apply changes")}
        </Button>
        <Button ref={closeRef} disabled={busy} onClick={() => setOpen(false)}>
          {t("Close")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
