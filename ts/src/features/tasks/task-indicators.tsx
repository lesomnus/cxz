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

const prioritySymbols = [
  { path: "M4 4h8v8H4Z", solid: true },
  { path: "M8 3 13 12H3Z", solid: true },
  { path: "M11 8a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z", solid: true },
  { path: "m4.5 11.5 7-7", solid: false },
  { path: "M9.5 8a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0Z", solid: true },
] as const;
const prioritySymbol = (priority: number) =>
  prioritySymbols[priority] ?? {
    path: "M6 6a2 2 0 1 1 3 1.75C8 8.25 8 9 8 9.5M8 12h.01",
    solid: false,
  };

export function taskPriorityMask(priority: number) {
  const symbol = prioritySymbol(priority);
  return `url("data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="${symbol.solid ? "black" : "none"}" stroke="${symbol.solid ? "none" : "black"}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="${symbol.path}"/></svg>`)}")`;
}

export function TaskPrioritySymbol({ priority }: { priority: number }) {
  const symbol = prioritySymbol(priority);
  return (
    <svg
      className={`${styles.priorityGlyph} ${taskStyles.priority}`}
      data-priority={priority}
      viewBox="0 0 16 16"
      fill={symbol.solid ? "currentColor" : "none"}
      stroke={symbol.solid ? "none" : "currentColor"}
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d={symbol.path} />
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
