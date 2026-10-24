import { ResourceValue as ValueMenu } from "#src/shared/components/resource-value.tsx";
import { useEffect, useState } from "react";
import { Button } from "@lesomnus/nox";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { SourceEditor } from "@lesomnus/nox/editor";
import {
  LibraryScope,
  LibraryDocuments,
  type LibraryMemory,
  type LibraryDocument,
} from "#gen/cxz/session_svc_pb";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useEditorSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import { Markdown } from "#src/shared/content/markdown.tsx";
import styles from "#src/shared/components/resource-surface.module.css";

export function MemoryLibrary({
  c,
  session,
}: {
  c: Connection;
  session: string;
}) {
  const [trash, setTrash] = useState(false);
  const scope = trash ? LibraryScope.TRASH : LibraryScope.LIBRARY;
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("");
  const [memory, setMemory] = useState("");
  const [document, setDocument] = useState("");
  const [deleted, setDeleted] = useState(false);
  const [draft, setDraft] = useState("");
  const [revision, setRevision] = useState("");
  const [original, setOriginal] = useState("");
  const [preview, setPreview] = useState(false);
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
    (signal) =>
      c.sessions.libraryMemories(
        { ref: ref(session), scope, query: filter },
        { signal, timeoutMs: 30000 },
      ),
    [c, session, scope, filter],
  );
  const contents = useRemote(
    async (signal) =>
      trash && !memory
        ? undefined
        : c.sessions.libraryMemory(
            {
              ref: ref(session),
              id: memory,
              scope,
              documents: deleted
                ? LibraryDocuments.DELETED
                : LibraryDocuments.PRESENT,
            },
            { signal, timeoutMs: 30000 },
          ),
    [c, session, memory, scope, deleted],
  );
  const text = useRemote(
    async (signal) =>
      document && !deleted && !creatingDocument
        ? c.sessions.libraryDocument(
            { ref: ref(session), id: memory, document, scope },
            { signal, timeoutMs: 30000 },
          )
        : undefined,
    [c, session, memory, document, scope, deleted, creatingDocument],
  );
  const own =
    !trash &&
    !contents.data?.memory?.snapshot &&
    contents.data?.memory?.session === session;
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
  const picked = contents.data?.memory;
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
          setFilter(query);
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
        <Button
          aria-pressed={!trash}
          onClick={() =>
            changeSelection(() => {
              setTrash(false);
              setMemory("");
              setDocument("");
            })
          }
        >
          Library
        </Button>
        <Button
          aria-pressed={trash}
          onClick={() =>
            changeSelection(() => {
              setTrash(true);
              setMemory("");
              setDocument("");
            })
          }
        >
          Trash
        </Button>
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
      <section className={styles.section}>
        <ValueMenu
          label="Memory"
          value={memory}
          options={[
            { value: "", label: trash ? "Choose a memory" : "This session" },
            ...(list.data?.memories ?? []).map((m) => ({
              value: m.id,
              label: `${m.name || m.id}${m.snapshot ? " · Snapshot" : ""}`,
            })),
          ]}
          choose={(id) =>
            changeSelection(() => {
              setMemory(id);
              setDocument("");
            })
          }
        />
        {picked && (
          <small className={styles.muted}>
            {picked.agent} · {picked.snapshot ? "Snapshot" : "Working memory"} ·{" "}
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
                      await c.sessions.forkLibraryMemory(
                        {
                          ...memoryRef,
                          name,
                        },
                        { timeoutMs: 30000 },
                      );
                      setMemory("");
                      setDocument("");
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
                    () =>
                      c.sessions.deleteLibraryMemory(memoryRef, {
                        timeoutMs: 30000,
                      }),
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
                  void act(() =>
                    c.sessions.restoreLibraryMemory(memoryRef, {
                      timeoutMs: 30000,
                    }),
                  )
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
                    () =>
                      c.sessions.purgeLibraryMemory(memoryRef, {
                        timeoutMs: 30000,
                      }),
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
              {list.data?.memories.map((m) => (
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
                    await c.sessions.mergeLibraryMemories(
                      {
                        ref: ref(session),
                        ids: merging,
                        name,
                      },
                      { timeoutMs: 30000 },
                    );
                    setMerging([]);
                  });
                })
              }
            >
              Merge selected
            </Button>
          </details>
        )}
      </section>
      <section className={styles.section}>
        <div className={styles.tabs}>
          <Button
            aria-pressed={!deleted}
            onClick={() =>
              changeSelection(() => {
                setDeleted(false);
                setDocument("");
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
                setDocument("");
              })
            }
          >
            Deleted documents
          </Button>
        </div>
        {contents.data?.documents.map((doc) => (
          <div key={doc.name} className={styles.row}>
            <Button
              aria-pressed={document === doc.name}
              onClick={() => editDocument(doc)}
            >
              {doc.name}
            </Button>
            <small className={styles.muted}>{doc.size.toString()} bytes</small>
            {own && !deleted && (
              <Button
                disabled={busy}
                onClick={() =>
                  destructive(
                    "Delete document",
                    "Move this document to trash?",
                    () =>
                      c.sessions.deleteLibraryDocument(
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
                Delete
              </Button>
            )}
            {!trash && deleted && picked?.session === session && (
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
      </section>
      {document && !deleted && (
        <section className={styles.section}>
          <div className={styles.row}>
            <strong>
              {document}
              {dirty ? " · Unsaved" : ""}
            </strong>
            <Button
              disabled={busy || text.loading}
              onClick={() => changeSelection(text.reload)}
            >
              Reload document
            </Button>
            <Button aria-pressed={preview} onClick={() => setPreview(!preview)}>
              {preview ? "Source" : "Preview"}
            </Button>
            <Button
              disabled={!own || busy || text.loading || (!dirty && !!revision)}
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
                  setCreatingDocument(false);
                })
              }
            >
              Save
            </Button>
          </div>
          {!own && (
            <small className={styles.muted}>
              Read only. Fork this memory to edit it in this session.
            </small>
          )}
          {preview ? (
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
        </section>
      )}
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
