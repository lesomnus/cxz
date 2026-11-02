import { useEffect, useState, useRef } from "react";
import { Button, Section, SegmentedControl } from "@lesomnus/nox";
import { SourceEditor } from "@lesomnus/nox/editor";
import type { Memory } from "#gen/cxz/memory_pb";
import { MemoryScope, MemoryReadFormat } from "#gen/cxz/memory_svc_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectRef } from "#src/shared/api/project-id.ts";
import { publicUUID } from "#src/shared/logs/rpc-diagnostics.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useEditorSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import { RelativeTime } from "#src/shared/components/date-time.tsx";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { Markdown } from "#src/shared/content/markdown.tsx";
import styles from "./memory-library.module.css";

const ownerID = (id: Uint8Array) =>
  publicUUID({ key: { case: "id", value: id } }) ?? "";
const key = (id: Uint8Array) =>
  Array.from(id, (b) => b.toString(16).padStart(2, "0")).join("");
const labelsText = (labels: Record<string, string>) =>
  Object.entries(labels)
    .map(([k, v]) => `${k}=${v}`)
    .join(", ");
function parseLabels(value: string) {
  const labels: Record<string, string> = {};
  for (const part of value.split(",")) {
    if (!part.trim()) continue;
    const at = part.indexOf("=");
    if (at < 1)
      throw new Error("Labels must use key=value, separated by commas.");
    labels[part.slice(0, at).trim()] = part.slice(at + 1).trim();
  }
  return labels;
}
export function MemoryLibrary({
  c,
  project,
  sessions = [],
  registerGuard,
}: {
  c: Connection;
  project: string;
  sessions?: Session[];
  registerGuard?: (guard: ((run: () => void) => void) | undefined) => void;
}) {
  const [trash, setTrash] = useState(false),
    [query, setQuery] = useState(""),
    [filter, setFilter] = useState(""),
    [labelQuery, setLabelQuery] = useState(""),
    [labelFilter, setLabelFilter] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<Uint8Array>(),
    [creating, setCreating] = useState(false),
    [draft, setDraft] = useState(""),
    [original, setOriginal] = useState(""),
    [name, setName] = useState(""),
    [labels, setLabels] = useState(""),
    [owner, setOwner] = useState(""),
    [preview, setPreview] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [history, setHistory] = useState(false),
    [compare, setCompare] = useState<bigint>();
  const [confirmation, setConfirmation] = useState<{
    title: string;
    text: string;
    run: () => Promise<unknown>;
  }>();
  const createKey = useRef(crypto.randomUUID()),
    theme = useTheme(),
    settings = useEditorSettings();
  const scope = trash ? MemoryScope.TRASH : MemoryScope.ACTIVE;
  const listKey = JSON.stringify([project, scope, filter, labelFilter]);
  const list = useRemote(
    async (signal) => {
      const items: Memory[] = [];
      let after = "";
      do {
        const p = await c.memories.list(
          {
            project: projectRef(project),
            scope,
            query: filter,
            filters: Object.keys(labelFilter).length
              ? [{ labels: labelFilter }]
              : [],
            size: 500,
            after,
          },
          { signal, timeoutMs: 30000 },
        );
        items.push(...p.items);
        if (p.next === after && after)
          throw new Error("Invalid memory page cursor");
        after = p.next;
      } while (after);
      return { items, key: listKey };
    },
    [c, project, scope, filter, labelFilter],
  );
  const selectionSource = useRef<typeof list.data>(undefined);
  const memories = list.data?.key === listKey ? list.data.items : [];
  const memoryRef = (id = selected!) => ({
    key: { case: "id" as const, value: id },
    project: projectRef(project),
  });
  const selectedKey = selected ? key(selected) : "";
  const text = useRemote(
    async (signal) =>
      selected && !creating
        ? {
            ...(await c.memories.read(
              { ref: memoryRef(), format: MemoryReadFormat.FULL },
              { signal, timeoutMs: 30000 },
            )),
            key: selectedKey,
          }
        : undefined,
    [c, project, selectedKey, creating],
  );
  const read = text.data?.key === selectedKey ? text.data : undefined;
  const picked = read?.memory;
  useEffect(() => {
    if (
      !creating &&
      !selected &&
      !list.loading &&
      list.data?.key === listKey &&
      list.data !== selectionSource.current
    )
      setSelected(memories[0]?.id);
  }, [creating, selectedKey, list.loading, list.data, listKey]);
  useEffect(() => {
    if (read) {
      setDraft(read.content);
      setOriginal(read.content);
      setName(read.memory?.name ?? "");
      setLabels(labelsText(read.memory?.labels ?? {}));
      setCompare(undefined);
    }
  }, [text.data]);
  const dirty = creating
    ? !!(draft || name || labels)
    : !!read &&
      (draft !== original ||
        name !== picked?.name ||
        labels !== labelsText(picked?.labels ?? {}));
  function choose(run: () => void) {
    if (busy) return;
    if (dirty)
      setConfirmation({
        title: "Discard unsaved changes",
        text: "This memory has unsaved changes. Discard them?",
        run: async () => {
          run();
        },
      });
    else run();
  }
  useEffect(() => {
    registerGuard?.(choose);
    return () => registerGuard?.(undefined);
  }, [dirty, busy, registerGuard]);
  async function act(run: () => Promise<unknown>) {
    if (busy) return false;
    setBusy(true);
    setError("");
    try {
      await run();
      list.reload();
      return true;
    } catch (e) {
      setError(String(e));
      return false;
    } finally {
      setBusy(false);
    }
  }
  function reset() {
    selectionSource.current = list.data;
    setSelected(undefined);
    setCreating(false);
    setDraft("");
    setOriginal("");
    setName("");
    setLabels("");
    setHistory(false);
    setCompare(undefined);
    setError("");
  }
  function newMemory(content = "", title = "", tags = "") {
    choose(() => {
      reset();
      createKey.current = crypto.randomUUID();
      setCreating(true);
      setDraft(content);
      setName(title);
      setLabels(tags);
      setPreview(false);
      setOwner(sessions[0] ? ownerID(sessions[0].id) : "");
    });
  }
  const versions = useRemote(
    async (signal) =>
      history && selected
        ? c.memories.versions(
            { ref: memoryRef(), size: 100 },
            { signal, timeoutMs: 30000 },
          )
        : undefined,
    [c, project, selectedKey, history, read?.revision],
  );
  const difference = useRemote(
    async (signal) =>
      compare !== undefined && selected
        ? c.memories.read(
            {
              ref: memoryRef(),
              baseRevision: compare,
              format: MemoryReadFormat.DIFF,
            },
            { signal, timeoutMs: 30000 },
          )
        : undefined,
    [c, project, selectedKey, compare, read?.revision],
  );
  async function save() {
    await act(async () => {
      const tagMap = parseLabels(labels);
      let saved: Memory;
      if (creating) {
        saved = await c.memories.add({
          project: projectRef(project),
          name,
          content: draft,
          labels: tagMap,
          ownerSession: owner,
          clientId: createKey.current,
        });
      } else {
        if (!picked) throw new Error("Read this memory before saving.");
        saved = await c.memories.patch({
          ref: memoryRef(),
          name: name !== picked.name ? name : undefined,
          content: draft !== original ? draft : undefined,
          labels: Object.keys(tagMap).length ? tagMap : undefined,
          clearLabels: !Object.keys(tagMap).length,
          dateUpdated: picked.dateUpdated,
          baseRevision: read!.revision,
        });
      }
      setOriginal(draft);
      setSelected(saved.id);
      setCreating(false);
      text.reload();
      versions.reload();
    });
  }
  function remove() {
    if (!picked) return;
    setConfirmation({
      title: "Delete memory",
      text: "Move this memory to trash? It can be restored until its deletion deadline.",
      run: async () => {
        await c.memories.erase(memoryRef());
        reset();
      },
    });
  }
  return (
    <div className={styles.content}>
      <form
        className={styles.row}
        onSubmit={(e) => {
          e.preventDefault();
          try {
            const tags = parseLabels(labelQuery);
            choose(() => {
              reset();
              setFilter(query);
              setLabelFilter(tags);
            });
          } catch (e) {
            setError(String(e));
          }
        }}
      >
        <input
          className={styles.input}
          aria-label="Search memory"
          placeholder="Search names and text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <input
          className={styles.input}
          aria-label="Filter labels"
          placeholder="Labels: topic=design, status=current"
          value={labelQuery}
          onChange={(e) => setLabelQuery(e.target.value)}
        />
        <Button type="submit" disabled={busy}>
          Search
        </Button>
      </form>
      <div className={styles.tabs}>
        <SegmentedControl
          label="Memory scope"
          size="compact"
          value={trash ? "trash" : "active"}
          disabled={busy}
          options={[
            { value: "active", label: "Memories" },
            { value: "trash", label: "Trash" },
          ]}
          onChange={(v) =>
            choose(() => {
              reset();
              setTrash(v === "trash");
            })
          }
        />
        <Button
          disabled={busy}
          onClick={() => {
            list.reload();
          }}
        >
          Refresh
        </Button>
        {!trash && (
          <Button disabled={busy} onClick={() => newMemory()}>
            New memory
          </Button>
        )}
      </div>
      {(error || list.error || text.error) && (
        <p role="alert">{error || list.error || text.error}</p>
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
                {filter || Object.keys(labelFilter).length
                  ? "No memories match your filters."
                  : trash
                    ? "Trash is empty."
                    : "No memories yet. Create a document to start."}
              </p>
            )}
            {list.error && (
              <p className={styles.empty}>
                Memories could not be loaded. Refresh to try again.
              </p>
            )}
            {memories.map((m) => {
              const owning = sessions.find(
                (s) => ownerID(s.id) === m.ownerSession,
              );
              return (
                <Button
                  key={key(m.id)}
                  className={styles.memoryItem}
                  disabled={busy}
                  aria-pressed={!creating && selectedKey === key(m.id)}
                  onClick={() =>
                    choose(() => {
                      reset();
                      setSelected(m.id);
                      setPreview(true);
                    })
                  }
                >
                  <span className={styles.memoryTitle}>
                    <span title={m.name}>{m.name}</span>
                  </span>
                  <small>
                    {owning?.alias || owning?.name || "Project memory"} · r
                    {m.revision.toString()}
                    {m.dateUpdated && (
                      <>
                        {" "}
                        · <RelativeTime value={m.dateUpdated} />
                      </>
                    )}
                  </small>
                  {Object.keys(m.labels).length > 0 && (
                    <small>{labelsText(m.labels)}</small>
                  )}
                </Button>
              );
            })}
          </nav>
        </Section>
        <div className={styles.detail}>
          {creating || selected ? (
            <Section
              className={styles.section}
              aria-label="Memory document"
              aria-busy={text.loading && !creating}
            >
              <header className={styles.heading}>
                <strong>
                  {creating ? "New memory" : picked?.name || "Memory"}
                  {dirty ? " · Unsaved" : ""}
                </strong>
                {picked && (
                  <span className={styles.muted}>
                    Revision {picked.revision.toString()}
                  </span>
                )}
              </header>
              {text.loading && !creating ? (
                <p className={styles.empty}>Loading memory…</p>
              ) : text.error && !creating ? (
                <Button onClick={text.reload}>Retry memory</Button>
              ) : (
                <>
                  {trash ? (
                    <>
                      <p className={styles.muted}>
                        In trash until{" "}
                        {picked?.dateDeleted
                          ? new Date(
                              Number(picked.dateDeleted.seconds) * 1000,
                            ).toLocaleString()
                          : ""}
                      </p>
                      <div className={styles.row}>
                        <Button
                          disabled={busy || !picked}
                          onClick={() =>
                            void act(async () => {
                              await c.memories.restore({
                                ref: memoryRef(),
                                dateUpdated: picked!.dateUpdated,
                              });
                              reset();
                            })
                          }
                        >
                          Restore
                        </Button>
                        <Button
                          disabled={busy || !picked}
                          onClick={() =>
                            setConfirmation({
                              title: "Purge memory",
                              text: "Permanently delete this memory and all its versions?",
                              run: async () => {
                                await c.memories.purge({
                                  ref: memoryRef(),
                                  dateUpdated: picked!.dateUpdated,
                                });
                                reset();
                              },
                            })
                          }
                        >
                          Purge
                        </Button>
                      </div>
                    </>
                  ) : (
                    <>
                      <label className={styles.field}>
                        Name
                        <input
                          className={styles.input}
                          aria-label="Memory name"
                          value={name}
                          onChange={(e) => setName(e.target.value)}
                          disabled={busy}
                        />
                      </label>
                      <label className={styles.field}>
                        Labels
                        <input
                          className={styles.input}
                          aria-label="Memory labels"
                          placeholder="topic=design"
                          value={labels}
                          onChange={(e) => setLabels(e.target.value)}
                          disabled={busy}
                        />
                      </label>
                      {creating && sessions.length > 0 && (
                        <label className={styles.field}>
                          Owner session
                          <select
                            className={styles.input}
                            aria-label="Owner session"
                            value={owner}
                            onChange={(e) => setOwner(e.target.value)}
                          >
                            <option value="">No owner session</option>
                            {sessions.map((s) => (
                              <option key={s.runtimeId} value={ownerID(s.id)}>
                                {s.alias || s.name || s.runtimeId}
                              </option>
                            ))}
                          </select>
                        </label>
                      )}
                      <div className={styles.row}>
                        <Button
                          disabled={
                            busy || !name.trim() || (!creating && !dirty)
                          }
                          loading={busy}
                          onClick={() => void save()}
                        >
                          Save
                        </Button>
                        <Button
                          aria-pressed={preview}
                          onClick={() => setPreview(!preview)}
                        >
                          {preview ? "Source" : "Preview"}
                        </Button>
                        {!creating && (
                          <>
                            <Button
                              disabled={busy}
                              onClick={() => choose(text.reload)}
                            >
                              Reload
                            </Button>
                            <Button
                              disabled={busy}
                              onClick={() =>
                                newMemory(draft, `${name} copy`, labels)
                              }
                            >
                              Copy
                            </Button>
                            <Button disabled={busy} onClick={remove}>
                              Delete
                            </Button>
                          </>
                        )}
                      </div>
                    </>
                  )}
                  {preview || trash ? (
                    <Markdown text={draft} />
                  ) : (
                    <div className={styles.editor}>
                      <SourceEditor
                        path={`memory-${selectedKey || "new"}.md`}
                        ariaLabel="Memory content"
                        value={draft}
                        readOnly={busy}
                        onChange={setDraft}
                        theme={theme}
                        settings={settings}
                      />
                    </div>
                  )}
                  {!creating && (
                    <details onToggle={(e) => setHistory(e.currentTarget.open)}>
                      <summary>Version history</summary>
                      {versions.error && <p role="alert">{versions.error}</p>}
                      <div className={styles.row}>
                        {versions.data?.versions.map((v) => (
                          <Button
                            key={v.revision.toString()}
                            aria-pressed={compare === v.revision}
                            onClick={() => setCompare(v.revision)}
                          >
                            r{v.revision.toString()}
                          </Button>
                        ))}
                      </div>
                      {difference.error && (
                        <p role="alert">{difference.error}</p>
                      )}
                      {difference.data && compare !== undefined && (
                        <>
                          <pre className={styles.diff}>
                            {difference.data.diff || "No body changes."}
                          </pre>
                          {!trash && (
                            <Button
                              disabled={busy}
                              onClick={() =>
                                choose(() => {
                                  void act(async () => {
                                    const past = await c.memories.read({
                                      ref: memoryRef(),
                                      revision: compare,
                                      format: MemoryReadFormat.FULL,
                                    });
                                    setDraft(past.content);
                                    setPreview(false);
                                    setCompare(undefined);
                                  });
                                })
                              }
                            >
                              Use this version
                            </Button>
                          )}
                        </>
                      )}
                    </details>
                  )}
                </>
              )}
            </Section>
          ) : (
            <Section className={styles.placeholder} aria-label="Memory preview">
              <strong>Your project's shared knowledge</strong>
              <p>
                Select a memory to read or edit it. Each memory is one Markdown
                document; labels organize related notes.
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
