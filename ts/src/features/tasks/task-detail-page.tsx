import { useEffect, useMemo, useRef } from "react";
import { Section, SlidingPage, SlidingPages } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { Icon } from "#src/shared/icons/icon.tsx";
import { TaskDetails } from "./task-dialog";
import { TaskStatusIcon } from "./task-indicators";
import { tableTasks, taskKey } from "./task-model";
import { TaskPageWheel } from "./task-page-wheel";
import styles from "./task-detail-page.module.css";
import pageStyles from "#src/shared/components/resource-page.module.css";

export function TaskDetailPage({
  c,
  project,
  tasks,
  sessions,
  selected,
  latest,
  assignee,
  run,
  busy,
  open,
  close,
  saved,
  setBusy,
  onDirtyChange,
  editorRevision,
}: {
  c: Connection;
  project: Project;
  tasks: Task[];
  sessions: Session[];
  selected: string;
  latest?: Task;
  assignee?: string;
  run?: string;
  busy: boolean;
  open: (task: Task) => void;
  close: () => void;
  saved: (task: Task) => void;
  setBusy: (busy: boolean) => void;
  onDirtyChange: (dirty: boolean) => void;
  editorRevision: number;
}) {
  const stage = useRef<HTMLDivElement>(null);
  const gesture = useRef(new TaskPageWheel());
  const ordered = useMemo(() => {
    const filtered = tableTasks(tasks, assignee, run);
    return latest && !filtered.some((task) => taskKey(task) === taskKey(latest))
      ? tableTasks([...filtered, latest])
      : filtered;
  }, [tasks, assignee, run, latest]);
  const current = latest ? taskKey(latest) : selected;
  const index = ordered.findIndex((task) => taskKey(task) === current);
  const previous = index > 0 ? ordered[index - 1] : undefined;
  const next = index >= 0 ? ordered[index + 1] : undefined;
  const pages =
    index < 0
      ? [{ value: current, task: undefined }]
      : ordered
          .slice(Math.max(0, index - 1), index + 2)
          .map((task) => ({ value: taskKey(task), task }));
  const navigation = useRef({ previous, next, busy, open });
  navigation.current = { previous, next, busy, open };
  const move = (direction: number) => {
    const value = navigation.current;
    const task = direction < 0 ? value.previous : value.next;
    if (task && !value.busy) value.open(task);
  };
  useEffect(() => {
    const root = stage.current!;
    const inside = (target: EventTarget | null) =>
      target instanceof Element && !!target.closest("[data-task-detail-panel]");
    const duration = () =>
      parseFloat(
        getComputedStyle(root).getPropertyValue("--task-page-duration"),
      ) || 0;
    const wheel = (event: WheelEvent) => {
      if (
        inside(event.target) ||
        event.ctrlKey ||
        Math.abs(event.deltaX) > Math.abs(event.deltaY)
      )
        return;
      event.preventDefault();
      const unit =
        event.deltaMode === 1
          ? 16
          : event.deltaMode === 2
            ? root.clientHeight
            : 1;
      const direction = gesture.current.step(
        event.deltaY * unit,
        performance.now(),
        duration(),
      );
      if (direction) move(direction);
    };
    let touch: { x: number; y: number } | undefined;
    const start = (event: TouchEvent) => {
      const point = event.touches[0];
      touch =
        !inside(event.target) && event.touches.length === 1 && point
          ? { x: point.clientX, y: point.clientY }
          : undefined;
    };
    const drag = (event: TouchEvent) => {
      if (touch) event.preventDefault();
    };
    const end = (event: TouchEvent) => {
      const point = event.changedTouches[0];
      if (touch && point) {
        const delta = touch.y - point.clientY;
        if (
          Math.abs(delta) > Math.abs(touch.x - point.clientX) &&
          Math.abs(delta) >= 64
        ) {
          event.preventDefault();
          move(Math.sign(delta));
        }
      }
      touch = undefined;
    };
    const cancel = () => {
      touch = undefined;
    };
    root.addEventListener("wheel", wheel, { passive: false });
    root.addEventListener("touchstart", start, { passive: true });
    root.addEventListener("touchmove", drag, { passive: false });
    root.addEventListener("touchend", end, { passive: false });
    root.addEventListener("touchcancel", cancel);
    return () => {
      root.removeEventListener("wheel", wheel);
      root.removeEventListener("touchstart", start);
      root.removeEventListener("touchmove", drag);
      root.removeEventListener("touchend", end);
      root.removeEventListener("touchcancel", cancel);
    };
  }, []);
  return (
    <div
      ref={stage}
      className={styles.stage}
      data-task-deck
      tabIndex={0}
      aria-label="Task navigation"
      onKeyDown={(event) => {
        if ((event.target as Element).closest("[data-task-detail-panel]"))
          return;
        const direction = ["PageUp", "ArrowUp"].includes(event.key)
          ? -1
          : ["PageDown", "ArrowDown"].includes(event.key)
            ? 1
            : 0;
        if (direction) {
          event.preventDefault();
          move(direction);
        }
      }}
    >
      <div
        className={`${pageStyles.content} ${styles.peek} ${styles.previous}`}
      >
        {previous && (
          <Neighbor
            task={previous}
            direction="previous"
            disabled={busy}
            open={open}
          />
        )}
      </div>
      <SlidingPages
        value={current}
        orientation="vertical"
        resize="none"
        className={`${pageStyles.content} ${styles.pages}`}
      >
        {pages.map(({ value, task }) => (
          <SlidingPage
            key={value}
            value={value}
            className={styles.panel}
            role="region"
            aria-label="Task details"
            data-task-detail-panel={task?.alias || selected}
          >
            <TaskDetails
              active={value === current}
              key={`${value}:${editorRevision}`}
              c={c}
              project={project}
              selected={task ? taskKey(task) : selected}
              latest={task}
              sessions={sessions}
              presentation="page"
              close={close}
              saved={saved}
              setBusy={value === current ? setBusy : undefined}
              onDirtyChange={value === current ? onDirtyChange : undefined}
            />
          </SlidingPage>
        ))}
      </SlidingPages>
      <div className={`${pageStyles.content} ${styles.peek} ${styles.next}`}>
        {next && (
          <Neighbor task={next} direction="next" disabled={busy} open={open} />
        )}
      </div>
    </div>
  );
}

function Neighbor({
  task,
  direction,
  disabled,
  open,
}: {
  task: Task;
  direction: "previous" | "next";
  disabled: boolean;
  open: (task: Task) => void;
}) {
  return (
    <Section
      as="button"
      type="button"
      className={styles.neighbor}
      disabled={disabled}
      onClick={() => open(task)}
      aria-label={`${direction === "previous" ? "Previous" : "Next"} task: ${task.name}`}
      data-task-neighbor={direction}
    >
      <TaskStatusIcon status={task.status} />
      <span className={styles.identity}>
        <strong>{task.name}</strong>
        <small>{task.alias}</small>
      </span>
      <Icon name={direction === "previous" ? "previous-task" : "next-task"} />
    </Section>
  );
}
