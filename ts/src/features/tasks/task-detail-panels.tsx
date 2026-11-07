import type { CSSProperties, ReactNode } from "react";
import { Button, ScrollArea, Section, SectionGroup } from "@lesomnus/nox";
import { Id } from "@lesomnus/payday/pdid";
import type { Task } from "#gen/cxz/task_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { ResourceFields } from "#src/shared/components/resource-fields.tsx";
import {
  projectLocation,
  resourceHandle,
} from "#src/shared/navigation/resource-route.ts";
import { ResourceNavigationButton } from "#src/shared/components/resource-navigation-button.tsx";
import { SessionNavigationButton } from "#src/shared/components/session-navigation-button.tsx";
import { ResourceControlGroup } from "#src/shared/components/resource-control-group.tsx";
import { Icon } from "#src/shared/icons/icon.tsx";
import type { TaskAssignmentAction } from "./task-assignment";
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
        <SectionGroup className={styles.content}>
          <Section aria-label="Task title" className={styles.title}>
            {title}
          </Section>
          <Section aria-label="Task description" className={styles.description}>
            {description}
          </Section>
        </SectionGroup>
        <SectionGroup className={styles.sidebar}>
          <Section className={styles.information} aria-label="Task information">
            <ScrollArea
              className={styles.metadata}
              viewportClassName={styles.metadataViewport}
              contentClassName={styles.metadataContent}
              scrollbarOffset={0}
              style={
                {
                  // Center the handle in the Section's existing right padding,
                  // rather than reserve an extra gutter inside the fields.
                  "--scrollbar-offset": "calc(var(--ui-section-padding) / 2)",
                } as CSSProperties
              }
              aria-label="Task metadata"
              scrollbarLabel="Scroll task information"
            >
              {controls}
              {metadata}
              {notices}
            </ScrollArea>
          </Section>
          <SectionGroup
            role="region"
            aria-label="Task actions"
            className={styles.actions}
          >
            {actions}
          </SectionGroup>
        </SectionGroup>
      </ResourceLayout>
    </ScrollArea>
  );
}

export function TaskMetadata({
  task,
  project = task.project,
  source,
}: {
  task: Task;
  project?: Project;
  source?: Session;
}) {
  return (
    <>
      <ResourceFields
        columns={2}
        variant="controls"
        rows={[
          [
            "Project",
            project ? (
              <ResourceNavigationButton
                {...projectLocation(resourceHandle(project))}
                name={project.name || project.alias || resourceHandle(project)}
                alias={project.alias}
              />
            ) : undefined,
          ],
          [
            "Created from",
            source ? (
              <SessionNavigationButton
                session={source}
                search={task.sourceSeq ? { seq: String(task.sourceSeq) } : {}}
              />
            ) : task.sourceSession ? (
              "Session unavailable"
            ) : undefined,
          ],
        ]}
      />
      {Object.keys(task.labels).length > 0 && (
        <ResourceFields
          columns={2}
          rows={[
            ...Object.entries(task.labels).map(
              ([label, value]) => [label, value] as const,
            ),
          ]}
        />
      )}
      <ResourceFields
        columns={[1, 2]}
        variant="controls"
        density="compact"
        rows={[
          {
            label: "Task ID",
            value:
              task.id.length === 16 ? new Id(task.id).toString() : undefined,
            identifier: true,
          },
          [
            "Created",
            <DateTime value={task.dateCreated} showOffset={false} />,
            undefined,
            <RelativeTime value={task.dateCreated} />,
          ],
        ]}
      />
    </>
  );
}

export function TaskAssigneeControl({
  task,
  owner,
  action,
  busy,
  loading,
  activate,
}: {
  task: Task;
  owner?: Session;
  action: TaskAssignmentAction;
  busy: boolean;
  loading: boolean;
  activate: () => void;
}) {
  return (
    <ResourceControlGroup
      label="Assigned session controls"
      // Project occupies half a row and Assigned two thirds. Account for the
      // group's padding so the session button has the same width as Project.
      leadingWidth="calc(75% + 1.5 * var(--ui-control-inset))"
      actions={
        <Button
          type="button"
          aria-label={
            action.kind === "random"
              ? "Assign randomly"
              : action.kind === "review"
                ? "Request task review"
                : "No assignment action"
          }
          disabled={
            busy ||
            action.kind === "none" ||
            (action.kind === "random" && !action.sessions.length)
          }
          loading={loading}
          onClick={activate}
        >
          <Icon
            name={
              action.kind === "random"
                ? "dice"
                : action.kind === "review"
                  ? "send-message"
                  : "dot"
            }
          />
        </Button>
      }
    >
      {owner ? (
        <SessionNavigationButton session={owner} />
      ) : task.assignee ? (
        "Session unavailable"
      ) : (
        "Unassigned"
      )}
    </ResourceControlGroup>
  );
}
