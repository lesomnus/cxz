import { Icon } from "#src/shared/icons/icon.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useEffect, useLayoutEffect, useState, useRef } from "react";
import {
  ActionMenu,
  Button,
  Section,
  SegmentedControl,
  FormField,
} from "@lesomnus/nox";
import type { Memory } from "#gen/cxz/memory_pb";
import {
  MemoryService,
  MemoryScope,
  MemoryReadFormat,
} from "#gen/cxz/memory_svc_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectRef } from "#src/shared/api/project-id.ts";
import { memoryLocation, uuid } from "#src/shared/navigation/resource-route.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { publicUUID } from "#src/shared/logs/rpc-diagnostics.ts";
import { memoryListKey } from "#src/shared/api/read-cache.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useConnectionQuery } from "#src/shared/api/use-query.ts";
import { RelativeTime } from "#src/shared/components/date-time.tsx";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { MemoryDocument, type MemoryDocumentAction } from "./memory-document";
import styles from "./memory-library.module.css";

const ownerID = (id: Uint8Array) =>
  publicUUID({ key: { case: "id", value: id } }) ?? "";
const key = (id: Uint8Array) =>
  Array.from(id, (b) => b.toString(16).padStart(2, "0")).join("");
const labelsText = (labels: Record<string, string>) =>
  Object.entries(labels)
    .map(([k, v]) => `${k}=${v}`)
    .join(", ");
const visibleLabels = (labels: Record<string, string>) =>
  Object.fromEntries(
    Object.entries(labels).filter(([k]) => !k.startsWith("legacy.")),
  );
function DocumentIcon() {
  return <Icon name="document" width="14" height="14" aria-hidden="true" />;
}
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
  projectHandle = project,
  projectInfo,
  catalog = true,
  sessions = [],
  registerGuard,
  memoryId,
  onMemoryChange,
  onDirtyChange,
  onCreatingChange,
}: {
  c: Connection;
  project: string;
  projectHandle?: string;
  projectInfo?: Project;
  catalog?: boolean;
  sessions?: Session[];
  registerGuard?: (guard: ((run: () => void) => void) | undefined) => void;
  memoryId?: string;
  onMemoryChange?: (id?: string, replace?: boolean) => void;
  onDirtyChange?: (dirty: boolean) => void;
  onCreatingChange?: (creating: boolean) => void;
}) {
  const [trash, setTrash] = useState(false),
    [query, setQuery] = useState(""),
    [filter, setFilter] = useState(""),
    [labelQuery, setLabelQuery] = useState(""),
    [labelFilter, setLabelFilter] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<Uint8Array | undefined>(() =>
      memoryId ? uuid(memoryId) : undefined,
    ),
    [creating, setCreating] = useState(false),
    [draft, setDraft] = useState(""),
    [name, setName] = useState(""),
    [labels, setLabels] = useState(""),
    [owner, setOwner] = useState(""),
    [showFilters, setShowFilters] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [confirmation, setConfirmation] = useState<{
    title: string;
    text: string;
    run: () => Promise<unknown>;
  }>();
  const lastRouteID = useRef(memoryId);
  const invalidMemoryID = !!(onMemoryChange && memoryId && !uuid(memoryId));
  const mutationPending = useRef(false);
  const createKey = useRef(crypto.randomUUID());
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
    { enabled: catalog },
  );
  const selectionSource = useRef<typeof list.data>(undefined);
  const memories =
    list.data?.key === listKey
      ? [...list.data.items].sort((a, b) => {
          const updated =
            Number(b.dateUpdated?.seconds ?? 0n) -
              Number(a.dateUpdated?.seconds ?? 0n) ||
            (b.dateUpdated?.nanos ?? 0) - (a.dateUpdated?.nanos ?? 0);
          return (
            updated ||
            a.name.localeCompare(b.name) ||
            key(a.id).localeCompare(key(b.id))
          );
        })
      : [];
  const memoryRef = (id = selected!) => ({
    key: { case: "id" as const, value: id },
    project: projectRef(project),
  });
  const selectedKey = selected ? key(selected) : "";
  const text = useConnectionQuery(
    c.queries,
    MemoryService.method.read,
    selected && !creating
      ? { ref: memoryRef(), format: MemoryReadFormat.FULL }
      : undefined,
    { watch: false },
  );
  const read = text.data;
  const picked = read?.memory;
  const reading = !creating && (text.loading || (!read && !text.error));
  useEffect(() => {
    if (
      !onMemoryChange &&
      !creating &&
      !selected &&
      !invalidMemoryID &&
      !list.loading &&
      list.data?.key === listKey &&
      list.data !== selectionSource.current
    )
      selectMemory(memories[0]?.id, true);
  }, [
    creating,
    selectedKey,
    list.loading,
    list.data,
    listKey,
    invalidMemoryID,
  ]);
  useLayoutEffect(() => {
    if (read && !creating) {
      setDraft(read.content);
      setName(read.memory?.name ?? "");
      setLabels(labelsText(visibleLabels(read.memory?.labels ?? {})));
    }
  }, [read, creating]);
  const dirty = creating && !!(draft || name || labels);
  // Browser Back can run immediately after an edit commits.
  useLayoutEffect(() => {
    onDirtyChange?.(dirty);
    return () => onDirtyChange?.(false);
  }, [dirty, onDirtyChange]);
  useEffect(() => {
    if (!onMemoryChange || memoryId === lastRouteID.current) return;
    lastRouteID.current = memoryId;
    reset();
    setSelected(memoryId ? uuid(memoryId) : undefined);
    if (!memoryId) {
      selectionSource.current = undefined;
      setTrash(false);
    }
    setQuery("");
    setFilter("");
    setLabelQuery("");
    setLabelFilter({});
  }, [memoryId, onMemoryChange]);
  useEffect(() => {
    if (
      onMemoryChange &&
      memoryId &&
      picked &&
      ownerID(picked.id) === memoryId &&
      !creating
    )
      setTrash(!!picked.dateDeleted);
  }, [picked?.id, picked?.dateDeleted, creating, memoryId, onMemoryChange]);
  function selectMemory(id: Uint8Array | undefined, replace = false) {
    setSelected(id);
    const publicID = id ? ownerID(id) : undefined;
    lastRouteID.current = publicID;
    onMemoryChange?.(publicID, replace);
  }
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
    if (mutationPending.current) return false;
    mutationPending.current = true;
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
      mutationPending.current = false;
      setBusy(false);
    }
  }
  function reset(updateRoute = false) {
    selectionSource.current = list.data;
    setSelected(undefined);
    setCreating(false);
    setDraft("");
    setName("");
    setLabels("");
    setError("");
    if (updateRoute) selectMemory(undefined, true);
  }
  function newMemory(content = "", title = "", tags = "") {
    choose(() => {
      reset();
      createKey.current = crypto.randomUUID();
      onCreatingChange?.(true);
      // Keep the source route until saving so leaving can guard the new draft.
      setCreating(true);
      setDraft(content);
      setName(title);
      setLabels(tags);
      setOwner(sessions[0] ? ownerID(sessions[0].id) : "");
    });
  }
  async function save() {
    if (!creating) return;
    await act(async () => {
      const saved = await c.memories.add({
        project: projectRef(project),
        name,
        content: draft,
        labels: parseLabels(labels),
        ownerSession: owner,
        clientId: createKey.current,
      });
      c.reads.invalidate(memoryListKey(project));
      selectMemory(saved.id, true);
      setCreating(false);
    });
  }
  function remove() {
    if (!picked) return;
    setConfirmation({
      title: "Delete memory",
      text: "Move this memory to trash? It can be restored until its deletion deadline.",
      run: async () => {
        await c.memories.erase(memoryRef());
        c.reads.invalidate(memoryListKey(project));
        reset(true);
      },
    });
  }
  function ownerFor(memory: Memory) {
    const session = sessions.find((s) => ownerID(s.id) === memory.ownerSession);
    return {
      name:
        session?.alias ||
        session?.name ||
        (memory.ownerSession
          ? `Session ${memory.ownerSession.slice(0, 8)}`
          : "Project memory"),
      agent: session?.agent || memory.labels["legacy.agent"] || "",
    };
  }
  function cancelEdit() {
    choose(() => {
      reset(true);
      onCreatingChange?.(false);
      list.reload();
    });
  }
  async function renameMemory(value: string) {
    if (creating) {
      setName(value);
      return;
    }
    if (!picked || trash || mutationPending.current)
      throw new Error("This memory cannot be renamed right now.");
    mutationPending.current = true;
    setBusy(true);
    setError("");
    try {
      const saved = await c.memories.patch({
        ref: memoryRef(),
        name: value,
        dateUpdated: picked.dateUpdated,
      });
      c.reads.invalidate(memoryListKey(project));
      await text.refresh((reply) => reply.memory?.name === saved.name);
      list.reload();
    } finally {
      mutationPending.current = false;
      setBusy(false);
    }
  }
  const documentActions: MemoryDocumentAction[] = creating
    ? [
        {
          label: "Save",
          icon: "save",
          disabled: busy || !name.trim(),
          run: save,
        },
        { label: "Cancel", icon: "close", disabled: busy, run: cancelEdit },
      ]
    : [
        {
          label: "Copy",
          icon: "copy",
          disabled: busy || reading || !picked || trash,
          run: () => newMemory(draft, name + " copy", labels),
        },
        {
          label: "Reload",
          icon: "refresh",
          disabled: busy || reading || !picked,
          run: () =>
            act(async () => {
              c.reads.invalidate(memoryListKey(project));
              await text.refresh();
            }),
        },
        {
          label: "Delete",
          icon: "erase",
          danger: true,
          disabled: busy || reading || !picked || trash,
          run: remove,
        },
      ];
  const tags = picked ? Object.entries(visibleLabels(picked.labels)) : [];
  return (
    <div className={styles.content}>
      {(error || list.error || text.error) && (
        <ErrorNotice
          className={styles.error}
          error={error || list.error || text.error}
        />
      )}
      <div
        className={styles.columns}
        data-catalog={catalog}
        data-collection={
          catalog && onMemoryChange && !memoryId && !creating
            ? "true"
            : undefined
        }
      >
        {catalog && (
          <Section className={styles.library} aria-label="Project memories">
            <header className={styles.heading}>
              <span className={styles.listTitle}>
                Memories <span className={styles.count}>{memories.length}</span>
              </span>
              <div className={styles.iconActions}>
                <Button
                  variant="toolbar"
                  disabled={busy || list.loading}
                  aria-label="Refresh"
                  title="Refresh"
                  onClick={() =>
                    choose(() => {
                      c.reads.invalidate(memoryListKey(project));
                      list.reload();
                      text.reload();
                    })
                  }
                >
                  <Icon name="sync" width="14" height="14" aria-hidden="true" />
                </Button>
                {!trash && (
                  <Button
                    variant="toolbar"
                    disabled={busy}
                    aria-label="New memory"
                    title="New memory"
                    onClick={() => newMemory()}
                  >
                    <Icon
                      name="add"
                      width="14"
                      height="14"
                      aria-hidden="true"
                    />
                  </Button>
                )}
              </div>
            </header>
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
                  reset(true);
                  setTrash(v === "trash");
                })
              }
            />
            <form
              className={styles.search}
              onSubmit={(e) => {
                e.preventDefault();
                try {
                  const tags = parseLabels(labelQuery);
                  choose(() => {
                    reset(true);
                    setFilter(query);
                    setLabelFilter(tags);
                  });
                } catch (e) {
                  setError(String(e));
                }
              }}
            >
              <div className={styles.searchRow}>
                <input
                  className={styles.input}
                  aria-label="Search memory"
                  placeholder="Search documents…"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
                <Button
                  type="submit"
                  variant="toolbar"
                  disabled={busy}
                  aria-label="Search"
                  title="Search"
                >
                  <Icon
                    name="search"
                    width="14"
                    height="14"
                    aria-hidden="true"
                  />
                </Button>
                <Button
                  type="button"
                  variant="toolbar"
                  disabled={busy}
                  aria-label="Filter labels"
                  title="Filter labels"
                  aria-pressed={showFilters}
                  onClick={() => setShowFilters((v) => !v)}
                >
                  <Icon
                    name="filter"
                    width="14"
                    height="14"
                    aria-hidden="true"
                  />
                </Button>
              </div>
              {showFilters && (
                <FormField label="Labels">
                  <input
                    aria-label="Filter labels"
                    placeholder="topic=design, status=current"
                    value={labelQuery}
                    onChange={(e) => setLabelQuery(e.target.value)}
                  />
                </FormField>
              )}
              {(filter || Object.keys(labelFilter).length > 0) && (
                <div className={styles.filterSummary}>
                  <span>Filtered results</span>
                  <Button
                    type="button"
                    disabled={busy}
                    onClick={() =>
                      choose(() => {
                        reset(true);
                        setQuery("");
                        setFilter("");
                        setLabelQuery("");
                        setLabelFilter({});
                      })
                    }
                  >
                    Clear
                  </Button>
                </div>
              )}
            </form>
            <PanelScroll
              className={styles.listScroll}
              viewportClassName={styles.listViewport}
              scrollbarLabel="Memory list scroll"
            >
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
                  <ErrorNotice error={list.error} className={styles.empty}>
                    Memories could not be loaded. Refresh to try again.
                  </ErrorNotice>
                )}
                {memories.map((m) => {
                  const owning = ownerFor(m),
                    tags = Object.entries(visibleLabels(m.labels));
                  const selectedItem = !creating && selectedKey === key(m.id);
                  const contents = (
                    <>
                      <span className={styles.memoryTitle}>
                        <DocumentIcon />
                        <span title={m.name}>{m.name}</span>
                        <span className={styles.revision}>
                          r{m.revision.toString()}
                        </span>
                      </span>
                      <span className={styles.memoryMeta}>
                        <span className={styles.owner}>
                          {owning.agent && (
                            <span aria-hidden="true">
                              <AgentBrand agent={owning.agent} />
                            </span>
                          )}
                          <span title={owning.name}>{owning.name}</span>
                        </span>
                        {m.dateUpdated && (
                          <RelativeTime value={m.dateUpdated} />
                        )}
                      </span>
                      {tags.length > 0 && (
                        <span className={styles.tags}>
                          {tags.slice(0, 2).map(([k, v]) => (
                            <span key={k} title={`${k}=${v}`}>
                              {k}={v}
                            </span>
                          ))}
                          {tags.length > 2 && <span>+{tags.length - 2}</span>}
                        </span>
                      )}
                    </>
                  );
                  const openMemory = () => {
                    if (selectedItem) return;
                    choose(() => {
                      reset();
                      selectMemory(m.id);
                    });
                  };
                  return onMemoryChange ? (
                    <RouteLink
                      key={key(m.id)}
                      {...memoryLocation(projectHandle, ownerID(m.id))}
                      className={styles.memoryItem}
                      data-selected={selectedItem || undefined}
                      aria-current={selectedItem ? "page" : undefined}
                      aria-disabled={busy || undefined}
                      onClick={(event) => {
                        if (busy) {
                          event.preventDefault();
                          return;
                        }
                        if (
                          event.button !== 0 ||
                          event.metaKey ||
                          event.ctrlKey ||
                          event.shiftKey ||
                          event.altKey
                        )
                          return;
                        event.preventDefault();
                        openMemory();
                      }}
                    >
                      {contents}
                    </RouteLink>
                  ) : (
                    <Button
                      key={key(m.id)}
                      className={styles.memoryItem}
                      disabled={busy}
                      aria-pressed={selectedItem}
                      onClick={openMemory}
                    >
                      {contents}
                    </Button>
                  );
                })}
              </nav>
            </PanelScroll>
          </Section>
        )}
        <div className={styles.detail}>
          {invalidMemoryID ? (
            <Section
              className={styles.placeholder}
              aria-label="Memory unavailable"
            >
              <strong>Memory unavailable</strong>
              <p role="alert">Invalid memory ID.</p>
            </Section>
          ) : creating || selected ? (
            <MemoryDocument
              memory={picked}
              project={projectInfo}
              name={creating ? name : (picked?.name ?? "")}
              content={draft}
              loading={reading}
              error={text.error}
              busy={busy}
              creating={creating}
              tags={tags}
              actions={documentActions}
              rename={renameMemory}
              retry={text.reload}
              labels={labels}
              owner={owner}
              sessions={sessions}
              changeContent={setDraft}
              changeLabels={setLabels}
              changeOwner={setOwner}
              restore={() =>
                act(async () => {
                  if (!picked) return;
                  await c.memories.restore({
                    ref: memoryRef(),
                    dateUpdated: picked.dateUpdated,
                  });
                  c.reads.invalidate(memoryListKey(project));
                  reset(true);
                })
              }
              purge={() => {
                if (!picked) return;
                setConfirmation({
                  title: "Purge memory",
                  text: "Permanently delete this memory and all its versions?",
                  run: async () => {
                    await c.memories.purge({
                      ref: memoryRef(),
                      dateUpdated: picked.dateUpdated,
                    });
                    c.reads.invalidate(memoryListKey(project));
                    reset(true);
                  },
                });
              }}
            />
          ) : (
            <Section className={styles.placeholder} aria-label="Memory preview">
              <DocumentIcon />
              <strong>
                {trash
                  ? "Select a deleted document"
                  : "Your project's shared knowledge"}
              </strong>
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
