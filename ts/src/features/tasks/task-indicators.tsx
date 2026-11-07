import { Icon, iconMask, type IconName } from "#src/shared/icons/icon.tsx";
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

const priorityIcons = [
  "priority-highest",
  "priority-high",
  "priority-normal",
  "priority-low",
  "priority-lowest",
] as const;
const priorityIcon = (priority: number): IconName =>
  priorityIcons[priority] ?? "priority-unknown";
const statusIcons: Partial<Record<TaskStatus, IconName>> = {
  [TaskStatus.TODO]: "task-todo",
  [TaskStatus.IN_PROGRESS]: "task-in-progress",
  [TaskStatus.WAITING]: "task-waiting",
  [TaskStatus.DONE]: "task-done",
  [TaskStatus.CANCELED]: "task-canceled",
};

export function taskPriorityMask(priority: number) {
  return iconMask(priorityIcon(priority));
}

export function TaskPrioritySymbol({ priority }: { priority: number }) {
  return (
    <Icon
      name={priorityIcon(priority)}
      className={`${styles.priorityGlyph} ${taskStyles.priority}`}
      data-priority={priority}
    />
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
        <Icon name={statusIcons[status] ?? "unknown"} />
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
