import { Icon } from "#src/shared/icons/icon.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useEffect, useLayoutEffect, useState, useRef, useId } from "react";
import {
  ActionMenu,
  Button,
  Section,
  SegmentedControl,
  Form,
  FormField,
  ValueMenu,
} from "@lesomnus/nox";
import { SourceEditor } from "@lesomnus/nox/editor";
import type { Memory } from "#gen/cxz/memory_pb";
import { MemoryScope, MemoryReadFormat } from "#gen/cxz/memory_svc_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectRef } from "#src/shared/api/project-id.ts";
import { memoryLocation, uuid } from "#src/shared/navigation/resource-route.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { publicUUID } from "#src/shared/logs/rpc-diagnostics.ts";
import { memoryListKey } from "#src/shared/api/read-cache.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useEditorSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import { RelativeTime } from "#src/shared/components/date-time.tsx";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { MemoryVersionChanges } from "./memory-version-changes";
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
const migrationLabels = (labels: Record<string, string>) =>
  Object.fromEntries(
    Object.entries(labels).filter(([k]) => k.startsWith("legacy.")),
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
    [original, setOriginal] = useState(""),
    [name, setName] = useState(""),
    [labels, setLabels] = useState(""),
    [owner, setOwner] = useState(""),
    [preview, setPreview] = useState(true),
    [editing, setEditing] = useState(false),
    [showFilters, setShowFilters] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [history, setHistory] = useState(false),
    [compare, setCompare] = useState<bigint>();
  const [confirmation, setConfirmation] = useState<{
    title: string;
    text: string;
    run: () => Promise<unknown>;
  }>();
  const lastRouteID = useRef(memoryId);
  const invalidMemoryID = !!(onMemoryChange && memoryId && !uuid(memoryId));
  const formId = useId();
  const mutationPending = useRef(false);
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
  useEffect(() => {
    if (read) {
      setDraft(read.content);
      setOriginal(read.content);
      setName(read.memory?.name ?? "");
      setLabels(labelsText(visibleLabels(read.memory?.labels ?? {})));
      setCompare(undefined);
    }
  }, [text.data]);
  const dirty = creating
    ? !!(draft || name || labels)
    : !!read &&
      (draft !== original ||
        name !== picked?.name ||
        labels !== labelsText(visibleLabels(picked?.labels ?? {})));
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
    setEditing(false);
    setPreview(true);
    setDraft("");
    setOriginal("");
    setName("");
    setLabels("");
    setHistory(false);
    setCompare(undefined);
    setError("");
    if (updateRoute) selectMemory(undefined, true);
  }
  function newMemory(content = "", title = "", tags = "") {
    choose(() => {
      reset();
      createKey.current = crypto.randomUUID();
      onCreatingChange?.(true);
      selectMemory(undefined);
      setCreating(true);
      setEditing(true);
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
      const tagMap = {
        ...(!creating ? migrationLabels(picked?.labels ?? {}) : {}),
        ...parseLabels(labels),
      };
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
      c.reads.invalidate(memoryListKey(project));
      setOriginal(draft);
      selectMemory(saved.id, true);
      setCreating(false);
      setEditing(false);
      setPreview(true);
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
        c.reads.invalidate(memoryListKey(project));
        reset(true);
      },
    });
  }
  function restoreVersion() {
    const restore = async () => {
      const past = await c.memories.read({
        ref: memoryRef(),
        revision: compare,
        format: MemoryReadFormat.FULL,
      });
      setDraft(past.content);
      setEditing(true);
      setPreview(false);
      setCompare(undefined);
    };
    if (dirty)
      setConfirmation({
        title: "Discard unsaved changes",
        text: "Replace your unsaved changes with this version?",
        run: restore,
      });
    else void act(restore);
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
      if (creating) {
        reset(true);
        onCreatingChange?.(false);
        list.reload();
        return;
      }
      setDraft(read?.content ?? "");
      setOriginal(read?.content ?? "");
      setName(picked?.name ?? "");
      setLabels(labelsText(visibleLabels(picked?.labels ?? {})));
      setEditing(false);
      setPreview(true);
      setError("");
    });
  }
  const ownerInfo = picked ? ownerFor(picked) : undefined;
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
            <Section
              className={styles.section}
              aria-label="Memory document"
              aria-busy={reading}
            >
              <header className={styles.documentHeader}>
                <div className={styles.documentTitle}>
                  <DocumentIcon />
                  <strong>
                    {creating ? "New memory" : picked?.name || "Memory"}
                    {dirty ? " · Unsaved" : ""}
                  </strong>
                </div>
                {!creating && picked && (
                  <ActionMenu
                    label="Memory actions"
                    items={[
                      ...(!trash
                        ? [
                            {
                              label: "Copy",
                              disabled: busy,
                              run: () =>
                                newMemory(draft, `${name} copy`, labels),
                            },
                          ]
                        : []),
                      {
                        label: "Reload",
                        disabled: busy,
                        run: () =>
                          choose(() => {
                            c.reads.invalidate(memoryListKey(project));
                            void text.reload();
                          }),
                      },
                      ...(!trash
                        ? [
                            {
                              label: "Delete",
                              danger: true,
                              disabled: busy,
                              run: remove,
                            },
                          ]
                        : []),
                    ]}
                  />
                )}
              </header>
              {reading ? (
                <p className={styles.empty}>Loading memory…</p>
              ) : text.error && !creating ? (
                <Button onClick={text.reload}>Retry memory</Button>
              ) : (
                <>
                  {!creating && picked && (
                    <div className={styles.documentMeta}>
                      <span className={styles.owner}>
                        {ownerInfo?.agent && (
                          <span aria-hidden="true">
                            <AgentBrand agent={ownerInfo.agent} />
                          </span>
                        )}
                        <span>{ownerInfo?.name}</span>
                      </span>
                      <span>Revision {picked.revision.toString()}</span>
                      {picked.dateUpdated && (
                        <RelativeTime value={picked.dateUpdated} />
                      )}
                    </div>
                  )}
                  {!editing && tags.length > 0 && (
                    <div className={styles.tags}>
                      {tags.map(([k, v]) => (
                        <span key={k}>
                          {k}={v}
                        </span>
                      ))}
                    </div>
                  )}
                  {trash ? (
                    <div className={styles.trashNotice}>
                      <p>
                        In trash until{" "}
                        {picked?.dateDeleted
                          ? new Date(
                              Number(picked.dateDeleted.seconds) * 1000,
                            ).toLocaleString()
                          : ""}
                      </p>
                      <div className={styles.actions}>
                        <Button
                          disabled={busy || !picked}
                          onClick={() =>
                            void act(async () => {
                              await c.memories.restore({
                                ref: memoryRef(),
                                dateUpdated: picked!.dateUpdated,
                              });
                              c.reads.invalidate(memoryListKey(project));
                              reset(true);
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
                                c.reads.invalidate(memoryListKey(project));
                                reset(true);
                              },
                            })
                          }
                        >
                          Purge
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <>
                      {editing && (
                        <Form
                          as="fieldset"
                          columns={1}
                          disabled={busy}
                          className={styles.editFields}
                        >
                          <FormField label="Name" htmlFor={`${formId}-name`}>
                            <input
                              id={`${formId}-name`}
                              aria-label="Memory name"
                              value={name}
                              onChange={(e) => setName(e.target.value)}
                            />
                          </FormField>
                          <FormField
                            label="Labels"
                            htmlFor={`${formId}-labels`}
                          >
                            <input
                              id={`${formId}-labels`}
                              aria-label="Memory labels"
                              placeholder="topic=design"
                              value={labels}
                              onChange={(e) => setLabels(e.target.value)}
                            />
                          </FormField>
                          {creating && sessions.length > 0 && (
                            <FormField label="Owner session">
                              <ValueMenu
                                label="Owner session"
                                value={owner}
                                options={[
                                  { value: "", label: "No owner session" },
                                  ...sessions.map((s) => ({
                                    value: ownerID(s.id),
                                    label: s.alias || s.name || s.runtimeId,
                                  })),
                                ]}
                                choose={setOwner}
                                disabled={busy}
                              />
                            </FormField>
                          )}
                        </Form>
                      )}
                      <div className={styles.documentTools}>
                        <div className={styles.actions}>
                          {editing ? (
                            <>
                              <Button
                                disabled={
                                  busy || !name.trim() || (!creating && !dirty)
                                }
                                loading={busy}
                                onClick={() => void save()}
                              >
                                Save
                              </Button>
                              <Button disabled={busy} onClick={cancelEdit}>
                                Cancel
                              </Button>
                            </>
                          ) : (
                            <Button
                              disabled={busy || !picked}
                              onClick={() => {
                                setEditing(true);
                                setPreview(false);
                              }}
                            >
                              Edit
                            </Button>
                          )}
                        </div>
                        <Button
                          disabled={busy}
                          aria-pressed={!preview}
                          onClick={() => setPreview((v) => !v)}
                        >
                          {preview ? "Source" : "Preview"}
                        </Button>
                      </div>
                    </>
                  )}
                  {preview || trash ? (
                    <div className={styles.documentBody}>
                      {draft.trim() ? (
                        <Markdown text={draft} />
                      ) : (
                        <p className={styles.empty}>This document is empty.</p>
                      )}
                    </div>
                  ) : (
                    <div className={styles.editor}>
                      <SourceEditor
                        path={`memory-${selectedKey || "new"}.md`}
                        ariaLabel="Memory content"
                        value={draft}
                        readOnly={!editing || busy}
                        onChange={setDraft}
                        theme={theme}
                        settings={settings}
                      />
                    </div>
                  )}
                  {!creating && picked && (
                    <>
                      <details
                        className={styles.disclosure}
                        open={history}
                        onToggle={(e) => setHistory(e.currentTarget.open)}
                      >
                        <summary>Version history</summary>
                        {versions.loading && (
                          <p className={styles.empty}>Loading versions…</p>
                        )}
                        {versions.error && (
                          <ErrorNotice error={versions.error} />
                        )}
                        <div className={styles.versionList}>
                          {versions.data?.versions.map((v) => (
                            <Button
                              key={v.revision.toString()}
                              aria-pressed={compare === v.revision}
                              aria-label={`r${v.revision}`}
                              aria-describedby={`${formId}-version-${v.revision}`}
                              onClick={() => setCompare(v.revision)}
                            >
                              <span>r{v.revision.toString()}</span>
                              <MemoryVersionChanges
                                version={v}
                                id={`${formId}-version-${v.revision}`}
                              />
                            </Button>
                          ))}
                        </div>
                        {difference.loading && compare !== undefined && (
                          <p className={styles.empty}>Loading comparison…</p>
                        )}
                        {difference.error && (
                          <ErrorNotice error={difference.error} />
                        )}
                        {difference.data && compare !== undefined && (
                          <>
                            {difference.data.format ===
                            MemoryReadFormat.DIFF ? (
                              <pre className={styles.diff}>
                                {difference.data.diff || "No body changes."}
                              </pre>
                            ) : (
                              <p className={styles.empty}>
                                This version is no longer available for
                                comparison. Read the current document above.
                              </p>
                            )}
                            {!trash && (
                              <Button
                                disabled={
                                  busy ||
                                  compare === picked.revision ||
                                  difference.data.format !==
                                    MemoryReadFormat.DIFF
                                }
                                onClick={restoreVersion}
                              >
                                Use this version
                              </Button>
                            )}
                          </>
                        )}
                      </details>
                      <details className={styles.disclosure}>
                        <summary>Document details</summary>
                        <dl className={styles.metadata}>
                          <dt>Memory ID</dt>
                          <dd>{ownerID(picked.id)}</dd>
                          <dt>Owner session</dt>
                          <dd>{picked.ownerSession || "No owner session"}</dd>
                          {Object.entries(migrationLabels(picked.labels)).map(
                            ([k, v]) => (
                              <div key={k}>
                                <dt>{k}</dt>
                                <dd>{v}</dd>
                              </div>
                            ),
                          )}
                        </dl>
                      </details>
                    </>
                  )}
                </>
              )}
            </Section>
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
