import type { ReactNode } from "react";
import { ScrollArea, Section, SectionGroup } from "@lesomnus/nox";
import { Id } from "@lesomnus/payday/pdid";
import type { Task } from "#gen/cxz/task_pb";
import type { Session } from "#gen/cxz/session_pb";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { ResourceFields } from "#src/shared/components/resource-fields.tsx";
import { AgentIdentity } from "#src/shared/components/agent-identity.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import styles from "./task-detail-panels.module.css";
import { ResourceLayout } from "#src/shared/components/resource-layout.tsx";

export function TaskDetailPanels({
  controls,
  metadata,
  title,
  description,
  actions,
  notices,
}: {
  controls: ReactNode;
  metadata: ReactNode;
  title: ReactNode;
  description: ReactNode;
  actions: ReactNode;
  notices: ReactNode;
}) {
  return (
    <ScrollArea
      className={styles.viewport}
      contentClassName={styles.viewportContent}
      aria-label="Task content"
      scrollbarLabel="Scroll task details"
    >
      <ResourceLayout className={styles.layout}>
        <SectionGroup className={styles.sidebar}>
          <Section className={styles.information} aria-label="Task information">
            <ScrollArea
              className={styles.metadata}
              aria-label="Task metadata"
              scrollbarLabel="Scroll task information"
            >
              {controls}
              {metadata}
              {notices}
            </ScrollArea>
          </Section>
          <Section aria-label="Task actions" className={styles.actions}>
            {actions}
          </Section>
        </SectionGroup>
        <SectionGroup className={styles.content}>
          <Section aria-label="Task title" className={styles.title}>
            {title}
          </Section>
          <Section aria-label="Task description" className={styles.description}>
            {description}
          </Section>
        </SectionGroup>
      </ResourceLayout>
    </ScrollArea>
  );
}

export function TaskMetadata({
  task,
  owner,
  source,
  issue,
}: {
  task: Task;
  owner?: Session;
  source?: Session;
  issue: string;
}) {
  return (
    <ResourceFields
      columns={1}
      rows={[
        {
          label: "Task ID",
          value: task.id.length === 16 ? new Id(task.id).toString() : undefined,
          identifier: true,
        },
        [
          "Assigned to",
          owner ? (
            <RouteLink {...sessionLocation(owner)} className={styles.session}>
              <AgentIdentity
                agent={owner.agent}
                name={owner.name || owner.alias}
                alias={owner.alias}
              />
            </RouteLink>
          ) : task.assignee ? (
            "Session unavailable"
          ) : (
            "Unassigned"
          ),
          issue
            ? `${issue}. Release makes this task available to another session.`
            : undefined,
        ],
        ...(issue ? [["Assignment", issue] as const] : []),
        ...(source || task.sourceSession
          ? [
              [
                "Created from",
                source ? (
                  <RouteLink
                    {...sessionLocation(source)}
                    search={
                      task.sourceSeq ? { seq: String(task.sourceSeq) } : {}
                    }
                    className={styles.session}
                  >
                    {source.name || source.alias}
                  </RouteLink>
                ) : (
                  "Session unavailable"
                ),
              ] as const,
            ]
          : []),
        [
          "Created",
          <DateTime value={task.dateCreated} />,
          undefined,
          <RelativeTime value={task.dateCreated} />,
        ],
        [
          "Updated",
          <DateTime value={task.dateUpdated} />,
          undefined,
          <RelativeTime value={task.dateUpdated} />,
        ],
        ...Object.entries(task.labels).map(
          ([label, value]) => [label, value] as const,
        ),
      ]}
    />
  );
}
