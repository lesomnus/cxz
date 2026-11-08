import { Icon } from "#src/shared/icons/icon.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useEffect, useState, useRef } from "react";
import { Button, Section, SegmentedControl, FormField } from "@lesomnus/nox";
import type { Memory } from "#gen/cxz/memory_pb";
import {
  MemoryService,
  MemoryScope,
  MemoryReadFormat,
  type MemoryVersion,
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
import type { MemoryPreviewMode } from "./memory-preview";
import styles from "./memory-library.module.css";

const ownerID = (id: Uint8Array) =>
  publicUUID({ key: { case: "id", value: id } }) ?? "";
const key = (id: Uint8Array) =>
  Array.from(id, (b) => b.toString(16).padStart(2, "0")).join("");
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
  memoryId,
  onMemoryChange,
}: {
  c: Connection;
  project: string;
  projectHandle?: string;
  projectInfo?: Project;
  catalog?: boolean;
  sessions?: Session[];
  memoryId?: string;
  onMemoryChange?: (id?: string, replace?: boolean) => void;
}) {
  const [trash, setTrash] = useState(false),
    [query, setQuery] = useState(""),
    [filter, setFilter] = useState(""),
    [labelQuery, setLabelQuery] = useState(""),
    [labelFilter, setLabelFilter] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<Uint8Array | undefined>(() =>
      memoryId ? uuid(memoryId) : undefined,
    ),
    [showFilters, setShowFilters] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [revision, setRevision] = useState(0n);
  const [previewMode, setPreviewMode] = useState<MemoryPreviewMode>("preview");
  const [confirmation, setConfirmation] = useState<{
    title: string;
    text: string;
    run: () => Promise<unknown>;
  }>();
  const lastRouteID = useRef(memoryId);
  const invalidMemoryID = !!(onMemoryChange && memoryId && !uuid(memoryId));
  const mutationPending = useRef(false);
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
    selected ? { ref: memoryRef(), format: MemoryReadFormat.FULL } : undefined,
    { watch: false },
  );
  const historical = useConnectionQuery(
    c.queries,
    MemoryService.method.read,
    selected && revision > 0n
      ? { ref: memoryRef(), revision, format: MemoryReadFormat.FULL }
      : undefined,
    { watch: false },
  );
  const read = revision > 0n ? historical.data : text.data;
  const picked = text.data?.memory;
  const selectedRevision =
    revision || text.data?.revision || picked?.revision || 0n;
  const comparison = useConnectionQuery(
    c.queries,
    MemoryService.method.read,
    selected && previewMode === "changes" && selectedRevision > 1n
      ? {
          ref: memoryRef(),
          revision: selectedRevision,
          baseRevision: selectedRevision - 1n,
          format: MemoryReadFormat.DIFF,
        }
      : undefined,
    { watch: false },
  );
  const reading =
    text.loading ||
    (revision > 0n && historical.loading) ||
    (!read && !text.error && !historical.error);
  const versions = useRemote(
    async (signal) => {
      const versions: MemoryVersion[] = [];
      const seen = new Set<bigint>();
      let beforeRevision = 0n;
      do {
        const page = await c.memories.versions(
          { ref: memoryRef(), beforeRevision, size: 100 },
          { signal, timeoutMs: 15000 },
        );
        versions.push(...page.versions);
        beforeRevision = page.nextRevision;
        if (beforeRevision && seen.has(beforeRevision))
          throw new Error("Invalid memory revision cursor");
        seen.add(beforeRevision);
      } while (beforeRevision);
      return { id: selectedKey, versions };
    },
    [c, project, selectedKey],
    { enabled: !!selected },
  );
  useEffect(() => {
    if (
      !onMemoryChange &&
      !selected &&
      !invalidMemoryID &&
      !list.loading &&
      list.data?.key === listKey &&
      list.data !== selectionSource.current
    )
      selectMemory(memories[0]?.id, true);
  }, [selectedKey, list.loading, list.data, listKey, invalidMemoryID]);
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
    if (onMemoryChange && memoryId && picked && ownerID(picked.id) === memoryId)
      setTrash(!!picked.dateDeleted);
  }, [picked?.id, picked?.dateDeleted, memoryId, onMemoryChange]);
  function selectMemory(id: Uint8Array | undefined, replace = false) {
    setRevision(0n);
    setPreviewMode("preview");
    setSelected(id);
    const publicID = id ? ownerID(id) : undefined;
    lastRouteID.current = publicID;
    onMemoryChange?.(publicID, replace);
  }
  function choose(run: () => void) {
    if (!busy) run();
  }
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
    setRevision(0n);
    setPreviewMode("preview");
    selectionSource.current = list.data;
    setSelected(undefined);
    setError("");
    if (updateRoute) selectMemory(undefined, true);
  }
  async function copyMemory() {
    if (!read || reading) return;
    setError("");
    try {
      await navigator.clipboard.writeText(read.content);
    } catch (error) {
      setError(`Could not copy memory: ${String(error)}`);
    }
  }
  function remove() {
    if (!picked) return;
    setConfirmation({
      title: "Erase memory",
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
  async function renameMemory(value: string) {
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
  const documentActions: MemoryDocumentAction[] = [
    {
      label: "Erase",
      icon: "erase",
      danger: true,
      disabled: busy || reading || !picked || trash,
      run: remove,
    },
    {
      label: "Copy",
      icon: "copy",
      disabled: busy || reading || !read || trash,
      run: copyMemory,
    },
    {
      label: "Reload",
      icon: "refresh",
      disabled: busy || reading || !picked,
      run: () =>
        act(async () => {
          c.reads.invalidate(memoryListKey(project));
          comparison.reload();
          await Promise.all([
            text.refresh(),
            versions.reload(),
            ...(revision > 0n ? [historical.refresh()] : []),
          ]);
        }),
    },
  ];
  return (
    <div className={styles.content}>
      {(error || list.error || text.error || historical.error) && (
        <ErrorNotice
          className={styles.error}
          error={error || list.error || text.error || historical.error}
        />
      )}
      <div
        className={styles.columns}
        data-catalog={catalog}
        data-collection={
          catalog && onMemoryChange && !memoryId ? "true" : undefined
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
                        : "No memories yet."}
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
                  const selectedItem = selectedKey === key(m.id);
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
          ) : selected ? (
            <MemoryDocument
              memory={picked}
              project={projectInfo}
              name={picked?.name ?? ""}
              content={read?.content ?? ""}
              loading={reading}
              error={revision > 0n ? historical.error : text.error}
              busy={busy}
              revision={revision}
              versions={
                versions.data?.id === selectedKey ? versions.data.versions : []
              }
              versionsLoading={versions.loading}
              versionsError={versions.error}
              changeRevision={setRevision}
              previewMode={previewMode}
              changePreviewMode={setPreviewMode}
              comparison={{
                revision: selectedRevision,
                data: comparison.data,
                loading: comparison.loading,
                error: comparison.error,
                retry: comparison.reload,
              }}
              actions={documentActions}
              rename={renameMemory}
              retry={revision > 0n ? historical.reload : text.reload}
              sessions={sessions}
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
                Select a memory to read it. Each memory is one Markdown
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
