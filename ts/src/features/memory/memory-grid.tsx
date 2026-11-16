import { Icon } from "#src/shared/icons/icon.tsx";
import { useId } from "react";
import { Button, CompactNumber } from "@lesomnus/nox";
import type { Memory } from "#gen/cxz/memory_pb";
import type { Session } from "#gen/cxz/session_pb";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import type { MemoryCatalog } from "./memory-catalog";
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
  list,
  projectHandle,
  sessions,
  session,
}: {
  list: MemoryCatalog;
  projectHandle: string;
  sessions: readonly Session[];
  session?: Uint8Array;
}) {
  const prefix = useId();
  const sessionId = sessionUUID(session);
  const memories = list.memories.filter(
    (memory) => !sessionId || memory.ownerSession === sessionId,
  );
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
      {list.loading && !list.loaded ? (
        <p className={styles.state} role="status">
          Loading memories…
        </p>
      ) : !list.loaded && list.error ? null : !memories.length ? (
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
