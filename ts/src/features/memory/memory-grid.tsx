import { Icon } from "#src/shared/icons/icon.tsx";
import { useEffect, useId } from "react";
import { Button, CompactNumber } from "@lesomnus/nox";
import type { Memory } from "#gen/cxz/memory_pb";
import { MemoryScope } from "#gen/cxz/memory_svc_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import { projectRef } from "#src/shared/api/project-id.ts";
import { MEMORY_LIST_TTL, memoryListKey } from "#src/shared/api/read-cache.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { RelativeTime } from "#src/shared/components/date-time.tsx";
import {
  memoryLocation,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import styles from "./memory-grid.module.css";

function owner(memory: Memory, sessions: readonly Session[]) {
  const session = sessions.find(
    (session) => resourceHandle({ id: session.id }) === memory.ownerSession,
  );
  return {
    name:
      session?.name ||
      session?.alias ||
      (memory.ownerSession
        ? `Session ${memory.ownerSession.slice(0, 8)}`
        : "Project memory"),
    agent: session?.agent || memory.labels["legacy.agent"] || "",
  };
}
function MemoryCard({
  memory,
  project,
  sessions,
  prefix,
}: {
  memory: Memory;
  project: string;
  sessions: readonly Session[];
  prefix: string;
}) {
  const owning = owner(memory, sessions);
  const id = resourceHandle({ id: memory.id });
  return (
    <RouteLink
      {...memoryLocation(project, id)}
      className={styles.card}
      aria-label={memory.name}
      aria-describedby={`${prefix}-${id}-owner ${prefix}-${id}-footer`}
    >
      <span id={`${prefix}-${id}-owner`} className={styles.header}>
        <span className={styles.owner}>
          <span className={styles.logo}>
            {["codex", "claude"].includes(owning.agent) ? (
              <AgentBrand agent={owning.agent} />
            ) : (
              <Icon name="document" width="14" height="14" aria-hidden="true" />
            )}
          </span>
          <span title={owning.name}>{owning.name}</span>
        </span>
        <RelativeTime value={memory.dateUpdated ?? memory.dateCreated} />
      </span>
      <span className={styles.title} title={memory.name}>
        {memory.name}
      </span>
      <span id={`${prefix}-${id}-footer`} className={styles.footer}>
        <span className={styles.revision}>
          r
          <CompactNumber
            value={Number(memory.revision)}
            mode="integer"
            digits={3}
            label={`Revision ${memory.revision}`}
          />
        </span>
      </span>
    </RouteLink>
  );
}
export function MemoryGrid({
  c,
  project,
  projectHandle,
  sessions,
  session,
}: {
  c: Connection;
  project: string;
  projectHandle: string;
  sessions: readonly Session[];
  session?: Uint8Array;
}) {
  const prefix = useId();
  const sessionId = sessionUUID(session);
  const cacheKey = memoryListKey(project);
  const list = useRemote(
    async (signal) => {
      const items = new Map<string, Memory>();
      const cursors = new Set<string>();
      let after = "";
      do {
        const page = await c.memories.list(
          {
            project: projectRef(project),
            scope: MemoryScope.ACTIVE,
            size: 500,
            after,
          },
          { signal, timeoutMs: 30000 },
        );
        for (const item of page.items)
          items.set(resourceHandle({ id: item.id }), item);
        if (page.next && cursors.has(page.next))
          throw new Error("Invalid memory page cursor");
        after = page.next;
        cursors.add(after);
      } while (after);
      return {
        project,
        items: [...items.values()].sort(
          (a, b) =>
            Number(
              (b.dateUpdated?.seconds ?? 0n) - (a.dateUpdated?.seconds ?? 0n),
            ) ||
            (b.dateUpdated?.nanos ?? 0) - (a.dateUpdated?.nanos ?? 0) ||
            a.name.localeCompare(b.name) ||
            resourceHandle({ id: a.id }).localeCompare(
              resourceHandle({ id: b.id }),
            ),
        ),
      };
    },
    [c, project],
    { cache: { store: c.reads, key: cacheKey, ttl: MEMORY_LIST_TTL } },
  );
  // Selection is local. Check freshness without canceling an in-flight project read.
  useEffect(() => {
    if (!list.loading && c.reads.peek(cacheKey)?.fresh === false)
      list.revalidate();
  }, [c, project, sessionId]);
  const data = list.data?.project === project ? list.data : undefined;
  const memories =
    data?.items.filter(
      (memory) => !sessionId || memory.ownerSession === sessionId,
    ) ?? [];
  return (
    <section
      className={styles.collection}
      aria-label="Project memories"
      aria-busy={list.loading}
    >
      {list.error && (
        <div className={styles.state}>
          <ErrorNotice error={list.error}>
            Memories could not be loaded. {list.error}
          </ErrorNotice>
          <Button variant="toolbar" onClick={() => void list.reload()}>
            Retry
          </Button>
        </div>
      )}
      {list.loading && !data ? (
        <p className={styles.state} role="status">
          Loading memories…
        </p>
      ) : !data && list.error ? null : !memories.length ? (
        <p className={styles.state}>
          {sessionId ? "No memories for this session." : "No memories yet."}
        </p>
      ) : (
        <nav className={styles.grid} aria-label="Memories">
          {memories.map((memory) => (
            <MemoryCard
              key={resourceHandle({ id: memory.id })}
              memory={memory}
              project={projectHandle}
              sessions={sessions}
              prefix={prefix}
            />
          ))}
        </nav>
      )}
    </section>
  );
}
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
