import { useEffect, useState } from "react";
import { Button, Section, SegmentedControl } from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { RelativeTime } from "#src/shared/components/date-time.tsx";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { SourceEditor } from "@lesomnus/nox/editor";
import {
  LibraryScope,
  LibraryDocuments,
  type LibraryDocument,
} from "#gen/cxz/session_svc_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useEditorSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import { Markdown } from "#src/shared/content/markdown.tsx";
import styles from "./memory-library.module.css";

export function MemoryLibrary({
  c,
  session,
  sessions = [],
  registerGuard,
}: {
  c: Connection;
  session: string;
  sessions?: Session[];
  registerGuard?: (guard: ((run: () => void) => void) | undefined) => void;
}) {
  const [trash, setTrash] = useState(false);
  const scope = trash ? LibraryScope.TRASH : LibraryScope.LIBRARY;
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("");
  const [selection, setMemory] = useState<string>();
  const [ownMemory, setOwnMemory] = useState<string>();
  const memory = selection ?? "";
  const newMemory = memory === "new";
  const listKey = JSON.stringify([session, scope, filter]);
  const [document, setDocument] = useState("");
  const [deleted, setDeleted] = useState(false);
  const [draft, setDraft] = useState("");
  const [revision, setRevision] = useState("");
  const [original, setOriginal] = useState("");
  const [preview, setPreview] = useState(true);
  const [name, setName] = useState("");
  const [newDocument, setNewDocument] = useState("");
  const [creatingDocument, setCreatingDocument] = useState(false);
  const [merging, setMerging] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [confirmation, setConfirmation] = useState<{
    title: string;
    text: string;
    run: () => Promise<unknown>;
  }>();
  const theme = useTheme();
  const settings = useEditorSettings();
  const list = useRemote(
    async (signal) => ({
      ...(await c.sessions.libraryMemories(
        { ref: ref(session), scope, query: filter },
        { signal, timeoutMs: 30000 },
      )),
      key: listKey,
    }),
    [c, session, scope, filter],
  );
  const contentsKey = JSON.stringify([session, memory, scope, deleted]);
  const contents = useRemote(
    async (signal) =>
      !memory || newMemory
        ? undefined
        : {
            ...(await c.sessions.libraryMemory(
              {
                ref: ref(session),
                id: memory,
                scope,
                documents: deleted
                  ? LibraryDocuments.DELETED
                  : LibraryDocuments.PRESENT,
              },
              { signal, timeoutMs: 30000 },
            )),
            key: contentsKey,
          },
    [c, session, memory, scope, deleted],
  );
  const text = useRemote(
    async (signal) =>
      memory && !newMemory && document && !deleted && !creatingDocument
        ? c.sessions.libraryDocument(
            { ref: ref(session), id: memory, document, scope },
            { signal, timeoutMs: 30000 },
          )
        : undefined,
    [c, session, memory, document, scope, deleted, creatingDocument],
  );
  const picked =
    contents.data?.key === contentsKey ? contents.data.memory : undefined;
  const documents =
    contents.data?.key === contentsKey ? contents.data.documents : [];
  const own =
    !trash && (newMemory || (!picked?.snapshot && picked?.session === session));
  const memories = list.data?.key === listKey ? list.data.memories : [];
  useEffect(() => {
    if (!trash && !filter && !list.loading && list.data?.key === listKey) {
      setOwnMemory(
        memories.find((m) => !m.snapshot && m.session === session)?.id,
      );
    }
    if (
      selection === undefined &&
      !list.loading &&
      list.data?.key === listKey
    ) {
      setMemory(
        memories.find((m) => !m.snapshot && m.size > 0n)?.id ??
          memories.find((m) => m.size > 0n)?.id ??
          memories[0]?.id ??
          "",
      );
    }
  }, [selection, list.loading, list.data, listKey]);
  useEffect(() => {
    if (text.data) {
      setDraft(text.data.content);
      setOriginal(text.data.content);
      setRevision(text.data.revision);
    } else {
      setDraft("");
      setOriginal("");
      setRevision("");
    }
  }, [text.data]);
  const dirty = draft !== original;
  function changeSelection(run: () => void) {
    if (busy) return;
    if (dirty)
      setConfirmation({
        title: "Discard unsaved changes",
        text: "The document has unsaved changes. Discard them before opening another document?",
        run: async () => {
          setDraft(original);
          run();
        },
      });
    else run();
  }
  useEffect(() => {
    registerGuard?.(changeSelection);
    return () => registerGuard?.(undefined);
  }, [dirty, original, busy, registerGuard]);
  function resetDocument() {
    setDocument("");
    setDraft("");
    setOriginal("");
    setRevision("");
    setCreatingDocument(false);
  }
  async function act(run: () => Promise<unknown>) {
    if (busy) return false;
    setBusy(true);
    setError("");
    try {
      await run();
      list.reload();
      contents.reload();
      return true;
    } catch (error) {
      setError(String(error));
      return false;
    } finally {
      setBusy(false);
    }
  }
  function destructive(
    title: string,
    text: string,
    run: () => Promise<unknown>,
  ) {
    setConfirmation({ title, text, run });
  }
  function editDocument(doc: LibraryDocument) {
    changeSelection(() => {
      setCreatingDocument(false);
      setDocument(doc.name);
      setNewDocument("");
    });
  }
  const memoryRef = { ref: ref(session), id: picked?.id ?? memory };
  return (
    <div className={styles.content}>
      <form
        className={styles.row}
        onSubmit={(event) => {
          event.preventDefault();
          changeSelection(() => {
            setMemory(undefined);
            resetDocument();
            setFilter(query);
          });
        }}
      >
        <input
          className={styles.input}
          aria-label="Search shared memory"
          placeholder="Search names and document text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <Button type="submit">Search</Button>
      </form>
      <div className={styles.tabs}>
        <SegmentedControl
          label="Memory scope"
          size="compact"
          disabled={busy}
          value={trash ? "trash" : "library"}
          options={[
            { value: "library", label: "Library" },
            { value: "trash", label: "Trash" },
          ]}
          onChange={(value) =>
            changeSelection(() => {
              setTrash(value === "trash");
              setMemory(undefined);
              resetDocument();
              setDeleted(false);
              setMerging([]);
            })
          }
        />
        <Button
          disabled={busy}
          onClick={() => {
            list.reload();
            contents.reload();
          }}
        >
          Refresh
        </Button>
        <Button
          disabled={busy || trash}
          onClick={() =>
            void act(() =>
              c.sessions.importLibraryMemories(
                { ref: ref(session) },
                { timeoutMs: 30000 },
              ),
            )
          }
        >
          Import files
        </Button>
      </div>
      {(error || list.error || contents.error || text.error) && (
        <p role="alert">
          {error || list.error || contents.error || text.error}
        </p>
      )}
      <div className={styles.columns}>
        <Section className={styles.library} aria-label="Project memories">
          <header className={styles.heading}>
            <strong>{trash ? "Trash" : "Memories"}</strong>
            <span className={styles.muted}>{memories.length}</span>
          </header>
          <nav
            className={styles.memoryList}
            aria-label="Memories"
            aria-busy={list.loading}
          >
            {list.loading && !list.data && (
              <p className={styles.empty}>Loading memories…</p>
            )}
            {!list.loading && !list.error && !memories.length && (
              <p className={styles.empty}>
                {filter
                  ? "No memories match your search."
                  : trash
                    ? "Trash is empty."
                    : "No shared memories yet. Create a document to start this session's memory."}
              </p>
            )}
            {list.error && (
              <p className={styles.empty}>
                Memories could not be loaded. Refresh to try again.
              </p>
            )}
            {memories.map((m) => {
              const owner = sessions.find((s) => s.runtimeId === m.session);
              const title =
                m.name && m.name !== "default"
                  ? m.name
                  : owner?.name || owner?.alias || m.id;
              return (
                <Button
                  key={m.id}
                  disabled={busy}
                  className={styles.memoryItem}
                  aria-pressed={memory === m.id}
                  onClick={() =>
                    changeSelection(() => {
                      setMemory(m.id);
                      resetDocument();
                      setName("");
                    })
                  }
                >
                  <span className={styles.memoryTitle}>
                    <AgentBrand agent={m.agent} />
                    <span title={title}>{title}</span>
                  </span>
                  <small>
                    {m.snapshot
                      ? "Snapshot"
                      : m.session === session
                        ? "This session"
                        : owner?.alias || "Working memory"}{" "}
                    · {formatSize(m.size)}
                    {m.updated && (
                      <>
                        {" "}
                        · <RelativeTime value={m.updated} />
                      </>
                    )}
                  </small>
                </Button>
              );
            })}
          </nav>
          {!trash && (
            <Button
              disabled={busy || list.loading || !!list.error}
              onClick={() =>
                changeSelection(() => {
                  setMemory(ownMemory ?? "new");
                  setQuery("");
                  setFilter("");
                  resetDocument();
                  setDeleted(false);
                })
              }
            >
              This session's memory
            </Button>
          )}
          <details className={styles.actions}>
            <summary>Memory actions</summary>
            {picked && (
              <small className={styles.muted}>
                {picked.agent} ·{" "}
                {picked.snapshot ? "Snapshot" : "Working memory"} ·{" "}
                {picked.size.toString()} bytes
              </small>
            )}
            <div className={styles.row}>
              <input
                className={styles.input}
                aria-label="Memory name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Name for rename, copy or merge"
              />
              <Button
                disabled={busy || !picked || !name.trim() || trash}
                onClick={() =>
                  void act(() =>
                    c.sessions.renameLibraryMemory(
                      {
                        ...memoryRef,
                        name: name.trim(),
                      },
                      { timeoutMs: 30000 },
                    ),
                  )
                }
              >
                Rename
              </Button>
            </div>
            <div className={styles.row}>
              {!trash && (
                <>
                  <Button
                    disabled={busy || !picked}
                    onClick={() =>
                      void act(() =>
                        c.sessions.snapshotLibraryMemory(
                          { ...memoryRef, name },
                          { timeoutMs: 30000 },
                        ),
                      )
                    }
                  >
                    Snapshot
                  </Button>
                  <Button
                    disabled={busy || !picked}
                    onClick={() =>
                      changeSelection(() => {
                        void act(async () => {
                          const forked = await c.sessions.forkLibraryMemory(
                            {
                              ...memoryRef,
                              name,
                            },
                            { timeoutMs: 30000 },
                          );
                          setMemory(forked.memory?.id ?? "");
                          resetDocument();
                        });
                      })
                    }
                  >
                    Fork into this session
                  </Button>
                  <Button
                    disabled={busy || !picked}
                    onClick={() =>
                      destructive(
                        "Delete memory",
                        "Move this memory to trash? You can restore it later.",
                        async () => {
                          await c.sessions.deleteLibraryMemory(memoryRef, {
                            timeoutMs: 30000,
                          });
                          setMemory("");
                          resetDocument();
                        },
                      )
                    }
                  >
                    Delete
                  </Button>
                </>
              )}
              {trash && (
                <>
                  <Button
                    disabled={busy || !picked}
                    onClick={() =>
                      void act(async () => {
                        await c.sessions.restoreLibraryMemory(memoryRef, {
                          timeoutMs: 30000,
                        });
                        setMemory("");
                        resetDocument();
                      })
                    }
                  >
                    Restore
                  </Button>
                  <Button
                    disabled={busy || !picked}
                    onClick={() =>
                      destructive(
                        "Purge memory",
                        "Permanently delete this memory? This cannot be undone.",
                        async () => {
                          await c.sessions.purgeLibraryMemory(memoryRef, {
                            timeoutMs: 30000,
                          });
                          setMemory("");
                          resetDocument();
                        },
                      )
                    }
                  >
                    Purge
                  </Button>
                </>
              )}
            </div>
            {!trash && (
              <details>
                <summary>Merge memories</summary>
                <div className={styles.list}>
                  {memories.map((m) => (
                    <label key={m.id}>
                      <input
                        type="checkbox"
                        checked={merging.includes(m.id)}
                        onChange={(e) =>
                          setMerging((old) =>
                            e.target.checked
                              ? [...old, m.id]
                              : old.filter((id) => id !== m.id),
                          )
                        }
                      />{" "}
                      {m.name || m.id}
                    </label>
                  ))}
                </div>
                <Button
                  disabled={busy || merging.length < 2 || merging.length > 8}
                  onClick={() =>
                    changeSelection(() => {
                      void act(async () => {
                        const merged = await c.sessions.mergeLibraryMemories(
                          {
                            ref: ref(session),
                            ids: merging,
                            name,
                          },
                          { timeoutMs: 30000 },
                        );
                        setMerging([]);
                        setMemory(merged.memory?.id ?? "");
                        resetDocument();
                      });
                    })
                  }
                >
                  Merge selected
                </Button>
              </details>
            )}
          </details>
        </Section>
        <div className={styles.detail}>
          <Section className={styles.section} aria-label="Memory documents">
            <header className={styles.heading}>
              <strong>
                {picked?.name ||
                  (newMemory ? "This session’s memory" : "Documents")}
              </strong>
              {picked && (
                <span className={styles.muted}>{formatSize(picked.size)}</span>
              )}
            </header>
            <div className={styles.tabs}>
              <Button
                aria-pressed={!deleted}
                onClick={() =>
                  changeSelection(() => {
                    setDeleted(false);
                    resetDocument();
                  })
                }
              >
                Documents
              </Button>
              <Button
                aria-pressed={deleted}
                onClick={() =>
                  changeSelection(() => {
                    setDeleted(true);
                    resetDocument();
                  })
                }
              >
                Deleted documents
              </Button>
            </div>
            {contents.loading && !!memory && !newMemory && (
              <p className={styles.empty}>Loading documents…</p>
            )}
            {!contents.loading &&
              !contents.error &&
              !documents.length &&
              (memory || !memories.length) && (
                <p className={styles.empty}>
                  {deleted
                    ? "No deleted documents."
                    : newMemory
                      ? "Add your first Markdown document below."
                      : picked
                        ? "This memory has no documents."
                        : "Choose a memory or start this session's memory."}
                </p>
              )}
            {contents.error && (
              <Button onClick={contents.reload}>Retry documents</Button>
            )}
            {documents.map((doc) => (
              <div key={doc.name} className={styles.row}>
                <Button
                  aria-pressed={document === doc.name}
                  disabled={busy}
                  onClick={() => editDocument(doc)}
                >
                  {doc.name}
                </Button>
                <small className={styles.muted}>
                  {doc.size.toString()} bytes
                </small>
                {own && !deleted && (
                  <Button
                    disabled={busy}
                    onClick={() =>
                      destructive(
                        "Delete document",
                        "Move this document to trash?",
                        async () => {
                          await c.sessions.deleteLibraryDocument(
                            {
                              ...memoryRef,
                              document: doc.name,
                              revision: doc.revision,
                            },
                            { timeoutMs: 30000 },
                          );
                          if (document === doc.name) resetDocument();
                        },
                      )
                    }
                  >
                    Delete
                  </Button>
                )}
                {own && deleted && (
                  <>
                    <Button
                      disabled={busy}
                      onClick={() =>
                        void act(() =>
                          c.sessions.restoreLibraryDocument(
                            {
                              ...memoryRef,
                              document: doc.name,
                              revision: doc.revision,
                            },
                            { timeoutMs: 30000 },
                          ),
                        )
                      }
                    >
                      Restore
                    </Button>
                    <Button
                      disabled={busy}
                      onClick={() =>
                        destructive(
                          "Purge document",
                          "Permanently delete this document?",
                          () =>
                            c.sessions.purgeLibraryDocument(
                              {
                                ...memoryRef,
                                document: doc.name,
                                revision: doc.revision,
                              },
                              { timeoutMs: 30000 },
                            ),
                        )
                      }
                    >
                      Purge
                    </Button>
                  </>
                )}
              </div>
            ))}
            {own && !deleted && (
              <div className={styles.row}>
                <input
                  className={styles.input}
                  aria-label="New document name"
                  placeholder="notes.md"
                  value={newDocument}
                  onChange={(e) => setNewDocument(e.target.value)}
                />
                <Button
                  disabled={!newDocument.trim() || busy}
                  onClick={() =>
                    changeSelection(() => {
                      setCreatingDocument(true);
                      setPreview(false);
                      setDocument(newDocument.trim());
                      setDraft("");
                      setOriginal("");
                      setRevision("");
                    })
                  }
                >
                  New document
                </Button>
              </div>
            )}
          </Section>
          {document && !deleted ? (
            <Section className={styles.section} aria-label="Memory document">
              <div className={styles.row}>
                <strong>
                  {document}
                  {dirty ? " · Unsaved" : ""}
                </strong>
                <Button
                  disabled={busy || text.loading || creatingDocument}
                  onClick={() => changeSelection(text.reload)}
                >
                  Reload document
                </Button>
                <Button
                  aria-pressed={preview}
                  onClick={() => setPreview(!preview)}
                >
                  {preview ? "Source" : "Preview"}
                </Button>
                <Button
                  disabled={
                    !own ||
                    busy ||
                    text.loading ||
                    !!text.error ||
                    (!dirty && !!revision)
                  }
                  onClick={() =>
                    void act(async () => {
                      const saved = await c.sessions.writeLibraryDocument(
                        {
                          ref: ref(session),
                          document,
                          content: draft,
                          revision,
                        },
                        { timeoutMs: 30000 },
                      );
                      setRevision(saved.revision);
                      setOriginal(saved.content);
                      setMemory(saved.memory?.id ?? memory);
                      setCreatingDocument(false);
                    })
                  }
                >
                  Save
                </Button>
              </div>
              {!own && (
                <small className={styles.muted}>
                  {trash
                    ? "Read only. Restore this memory before editing or forking it."
                    : "Read only. Fork this memory to edit it in this session."}
                </small>
              )}
              {text.error ? (
                <p className={styles.empty}>
                  The document could not be loaded. Reload to try again.
                </p>
              ) : text.loading ? (
                <p className={styles.empty}>Loading document…</p>
              ) : preview ? (
                <Markdown text={draft} />
              ) : (
                <div className={styles.editor}>
                  <SourceEditor
                    path={document}
                    ariaLabel="Memory document"
                    value={draft}
                    readOnly={!own || busy || text.loading}
                    onChange={setDraft}
                    theme={theme}
                    settings={settings}
                  />
                </div>
              )}
            </Section>
          ) : (
            <Section
              className={styles.placeholder}
              aria-label="Document preview"
            >
              <strong>Your project's shared knowledge</strong>
              <p>
                Choose a document to read it. Working memories can be edited by
                their owning session; fork another memory to make it your own.
              </p>
            </Section>
          )}
        </div>
      </div>
      {confirmation && (
        <ConfirmationDialog
          title={confirmation.title}
          confirmLabel="Confirm"
          close={() => setConfirmation(undefined)}
          execute={() => act(confirmation.run)}
        >
          <p>{confirmation.text}</p>
        </ConfirmationDialog>
      )}
    </div>
  );
}

function formatSize(bytes: bigint) {
  const size = Number(bytes);
  return size < 1024
    ? `${size} B`
    : size < 1024 * 1024
      ? `${(size / 1024).toFixed(1)} KB`
      : `${(size / 1024 / 1024).toFixed(1)} MB`;
}
