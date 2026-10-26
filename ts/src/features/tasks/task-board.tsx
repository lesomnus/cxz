import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { formatOffsetDateTime } from "#src/shared/components/date-time-format.ts";
import { useEffect, useState } from "react";
import { Button, LinkButton } from "@lesomnus/nox";
import { createLink } from "@tanstack/react-router";
import type { Session } from "#gen/cxz/session_pb";
import { TaskStatus, type Task } from "#gen/cxz/task_pb";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { relativeMessageTime } from "#src/features/session/conversation/message-time.ts";
import { boardTasks, ownerIssue, taskTime, taskKey } from "./task-model";
import styles from "./tasks.module.css";

const priorityLabels = ["Highest", "High", "Normal", "Low", "Lowest"];
const OwnerLink = createLink(LinkButton);

function TaskCard({
  task,
  sessions,
  sessionsLoading,
  open,
  now,
}: {
  task: Task;
  sessions: Session[];
  sessionsLoading?: boolean;
  open: (task: Task) => void;
  now: number;
}) {
  const owner = sessions.find((session) => session.runtimeId === task.assignee);
  const issue = ownerIssue(task, sessions, sessionsLoading);
  const date = new Date(taskTime(task));
  const relative = relativeMessageTime(date, new Date(now));
  return (
    <article className={styles.card} data-task-alias={task.alias}>
      <Button
        className={styles.cardMain}
        type="button"
        onClick={() => open(task)}
        aria-label={`Open task: ${task.name}`}
      >
        <span className={styles.cardHeading}>
          <span
            className={styles.priority}
            data-priority={task.priority}
            role="img"
            aria-label={`${priorityLabels[task.priority] ?? "Unknown"} priority`}
            title={`${priorityLabels[task.priority] ?? "Unknown"} priority`}
          />
          <small className={styles.alias}>{task.alias}</small>
        </span>
        <strong className={styles.cardTitle}>{task.name}</strong>
      </Button>
      <div className={styles.cardMeta}>
        {owner ? (
          <OwnerLink
            className={styles.owner}
            {...sessionLocation(owner)}
            title={owner.name || owner.alias}
            trailing={null}
          >
            <span className={styles.ownerLabel}>
              <AgentBrand agent={owner.agent} />
              <span>{owner.alias}</span>
            </span>
          </OwnerLink>
        ) : (
          <span className={styles.quiet}>
            {task.assignee ? "Session unavailable" : "Unassigned"}
          </span>
        )}
        {relative ? (
          <time
            dateTime={date.toISOString()}
            title={`Updated ${formatOffsetDateTime(date)}`}
          >
            {relative}
          </time>
        ) : (
          <DateTime
            value={date.getTime()}
            title={`Updated ${formatOffsetDateTime(date)}`}
          />
        )}
      </div>
      {issue && <span className={styles.ownerIssue}>{issue}</span>}
    </article>
  );
}

export function TaskBoard({
  tasks,
  sessions,
  sessionsLoading,
  assignee = "",
  run = "",
  open,
}: {
  tasks: Task[];
  sessions: Session[];
  sessionsLoading?: boolean;
  assignee?: string;
  run?: string;
  open: (task: Task) => void;
}) {
  const [allDone, setAllDone] = useState(false);
  const [canceled, setCanceled] = useState(false);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30000);
    return () => clearInterval(timer);
  }, []);
  const columns = boardTasks(tasks, assignee, run, allDone);
  const canceledTasks = tasks.filter(
    (task) =>
      task.status === TaskStatus.CANCELED &&
      (!assignee || task.assignee === assignee) &&
      (!run || task.assigneeRun === run),
  );
  return (
    <>
      <PanelScroll
        className={styles.boardArea}
        scrollbarLabel="Scroll task board"
      >
        <div className={styles.board} aria-label="Project task board">
          {columns.map((column) => (
            <section
              className={styles.column}
              key={column.value}
              aria-label={`${column.label} tasks`}
            >
              <header>
                <h2>{column.label}</h2>
                <span className={styles.count}>{column.total}</span>
              </header>
              <PanelScroll scrollbarLabel={`Scroll ${column.label} tasks`}>
                {!column.items.length && (
                  <p className={styles.empty}>No tasks</p>
                )}
                {column.items.map((task) => (
                  <TaskCard
                    key={taskKey(task)}
                    task={task}
                    sessions={sessions}
                    sessionsLoading={sessionsLoading}
                    open={open}
                    now={now}
                  />
                ))}
                {column.items.length < column.total && (
                  <Button
                    type="button"
                    className={styles.more}
                    onClick={() => setAllDone(true)}
                  >
                    Show all completed
                  </Button>
                )}
              </PanelScroll>
            </section>
          ))}
        </div>
      </PanelScroll>
      {!!canceledTasks.length && (
        <div className={styles.canceled}>
          <Button
            type="button"
            aria-expanded={canceled}
            onClick={() => setCanceled(!canceled)}
          >
            {canceled ? "Hide canceled" : "Show canceled"}
            <span className={styles.count}>{canceledTasks.length}</span>
          </Button>
          {canceled && (
            <PanelScroll
              className={styles.canceledList}
              scrollbarLabel="Scroll canceled tasks"
            >
              {canceledTasks.map((task) => (
                <TaskCard
                  key={taskKey(task)}
                  task={task}
                  sessions={sessions}
                  open={open}
                  now={now}
                />
              ))}
            </PanelScroll>
          )}
        </div>
      )}
    </>
  );
}
