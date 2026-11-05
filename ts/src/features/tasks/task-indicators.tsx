import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { Popover } from "@lesomnus/nox";
import { TaskStatus } from "#gen/cxz/task_pb";
import { DateTime } from "#src/shared/components/date-time.tsx";
import {
  dateTimeDate,
  formatRelativeDateTime,
} from "#src/shared/components/date-time-format.ts";
import { taskStatusName } from "./task-model";
import styles from "./task-table.module.css";
import taskStyles from "./tasks.module.css";

const priorities = ["Highest", "High", "Normal", "Low", "Lowest"];

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
  const label = `${priorities[priority] ?? "Unknown"} priority`;
  return (
    <Popover content={label}>
      <span
        className={`${styles.indicator} ${taskStyles.priority}`}
        data-priority={priority}
        role="img"
        aria-label={label}
        tabIndex={0}
      >
        <span className={styles.priorityDot} aria-hidden="true" />
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
