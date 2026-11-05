import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { Popover } from "@lesomnus/nox";
import { TaskStatus } from "#gen/cxz/task_pb";
import { DateTime } from "#src/shared/components/date-time.tsx";
import {
  dateTimeDate,
  formatRelativeDateTime,
} from "#src/shared/components/date-time-format.ts";
import { taskPriorities, taskStatusName } from "./task-model";
import styles from "./task-table.module.css";
import taskStyles from "./tasks.module.css";

const priorityPaths = [
  "M5.5 3v6m5-6v6M5.5 12h.01m4.99 0h.01",
  "M8 13V3M4 7l4-4 4 4",
  "M4 6h8M4 10h8",
  "M8 3v10M4 9l4 4 4-4",
  "m4 4 4 4 4-4m-8 5 4 4 4-4",
] as const;
const priorityPath = (priority: number) =>
  priorityPaths[priority] ?? "M6 6a2 2 0 1 1 3 1.75C8 8.25 8 9 8 9.5M8 12h.01";

export function taskPriorityMask(priority: number) {
  return `url("data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="black" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="${priorityPath(priority)}"/></svg>`)}")`;
}

export function TaskPrioritySymbol({ priority }: { priority: number }) {
  return (
    <svg
      className={`${styles.priorityGlyph} ${taskStyles.priority}`}
      data-priority={priority}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d={priorityPath(priority)} />
    </svg>
  );
}

export function TaskStatusIcon({ status }: { status: TaskStatus }) {
  const label = taskStatusName(status);
  return (
    <Popover content={label}>
      <span
        className={styles.indicator}
        role="img"
        aria-label={label}
        tabIndex={0}
      >
        <svg
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <circle cx="8" cy="8" r="5.5" />
          {status === TaskStatus.IN_PROGRESS ? (
            <path
              d="M8 2.5a5.5 5.5 0 0 1 0 11Z"
              fill="currentColor"
              stroke="none"
            />
          ) : status === TaskStatus.WAITING ? (
            <path d="M6.5 5.75v4.5m3-4.5v4.5" />
          ) : status === TaskStatus.DONE ? (
            <path d="m5 8 2 2 4-4" />
          ) : status === TaskStatus.CANCELED ? (
            <path d="m4 12 8-8" />
          ) : status !== TaskStatus.TODO ? (
            <path d="M6.5 6.25a1.5 1.5 0 1 1 2 1.4c-.5.2-.5.6-.5 1.1M8 11h.01" />
          ) : null}
        </svg>
      </span>
    </Popover>
  );
}

export function TaskPriority({ priority }: { priority: number }) {
  const label = `${taskPriorities[priority] ?? "Unknown"} priority`;
  return (
    <Popover content={label}>
      <span
        className={styles.indicator}
        role="img"
        aria-label={label}
        tabIndex={0}
      >
        <TaskPrioritySymbol priority={priority} />
      </span>
    </Popover>
  );
}

export function TaskTime({
  value,
  now,
  locale,
}: {
  value?: Timestamp;
  now: number;
  locale: string;
}) {
  const date = dateTimeDate(value);
  if (!date) return <>—</>;
  return (
    <Popover content={<DateTime value={value} />}>
      <time dateTime={date.toISOString()}>
        {formatRelativeDateTime(date, locale, now)}
      </time>
    </Popover>
  );
}
