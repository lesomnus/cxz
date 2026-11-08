import { useLayoutEffect, useRef } from "react";
import {
  DialogReveal,
  MenuSurface,
  MenuFrame,
  MenuScrollArea,
  MenuItem,
  MenuItemLabel,
} from "@lesomnus/nox";
import { key } from "@lesomnus/payday/store";
import type { Project } from "#gen/cxz/project_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { WorkTaskState } from "#src/features/session/model/work-tasks.ts";
import { TaskPrioritySymbol } from "#src/features/tasks/task-indicators.tsx";
import { MenuLink } from "#src/shared/navigation/route-link.tsx";
import { taskLocation } from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./created-tasks-menu.module.css";

function TaskList({
  project,
  tasks,
  state,
  close,
}: {
  project: Project;
  tasks: Task[];
  state: WorkTaskState;
  close: () => void;
}) {
  return (
    <MenuScrollArea
      viewportClassName="created-tasks-menu-viewport"
      scrollbarLabel={t("Tasks")}
    >
      {tasks.map((task) => (
        <MenuLink
          key={key(task.id)}
          {...taskLocation({ task, project })}
          role="menuitem"
          icon={<TaskPrioritySymbol priority={task.priority} />}
          title={task.name || task.alias}
          aria-description={task.alias || undefined}
          onClick={close}
        >
          <MenuItemLabel>{task.name || task.alias}</MenuItemLabel>
        </MenuLink>
      ))}
      {state.error && (
        <div className={styles.state} role="alert">
          <span>{state.error}</span>
          <MenuItem type="button" role="menuitem" onClick={state.refresh}>
            <MenuItemLabel>{t("Retry")}</MenuItemLabel>
          </MenuItem>
        </div>
      )}
      {!tasks.length && !state.error && (
        <p className={styles.state} role="status">
          {t(
            state.loading ? "Loading…" : "No Tasks were created in this range.",
          )}
        </p>
      )}
      {tasks.length > 0 && state.loading && (
        <p className={styles.note} role="status">
          {t("Loading…")}
        </p>
      )}
    </MenuScrollArea>
  );
}

/** Click-only, non-modal Task navigation; never changes transcript geometry. */
export function CreatedTasksMenu({
  id,
  anchor,
  project,
  tasks,
  state,
  partial,
  close,
}: {
  id: string;
  anchor: HTMLButtonElement;
  project: Project;
  tasks: Task[];
  state: WorkTaskState;
  partial: boolean;
  close: (restoreFocus?: boolean) => void;
}) {
  useLocale();
  const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const node = root.current;
    if (!node) return;
    let frame = 0;
    const position = () => {
      if (!anchor.isConnected) return close();
      const rect = anchor.getBoundingClientRect();
      const gap =
        parseFloat(getComputedStyle(node).getPropertyValue("--ui-space")) || 4;
      const available = Math.max(0, rect.top - 2 * gap);
      const above = available >= Math.min(120, innerHeight / 3);
      const budget = Math.max(
        0,
        Math.min(
          innerHeight / 2,
          above ? available : innerHeight - rect.bottom - 2 * gap,
        ),
      );
      const frame = getComputedStyle(node);
      const chrome =
        parseFloat(frame.paddingTop) +
        parseFloat(frame.paddingBottom) +
        parseFloat(frame.borderTopWidth) +
        parseFloat(frame.borderBottomWidth) +
        [...node.querySelectorAll<HTMLElement>("[data-menu-fixed]")].reduce(
          (sum, region) => sum + region.offsetHeight,
          0,
        );
      node.style.maxHeight = `${budget}px`;
      node.style.setProperty(
        "--menu-scroll-max-height",
        `${Math.max(0, budget - chrome)}px`,
      );
      const box = node.getBoundingClientRect();
      node.style.left = `${Math.max(gap, Math.min(rect.left, innerWidth - box.width - gap))}px`;
      node.style.top = `${above ? rect.top - box.height - gap : rect.bottom + gap}px`;
      node.style.visibility = "visible";
    };
    const schedulePosition = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        position();
      });
    };
    position();
    node
      .querySelector<HTMLElement>('[role="menuitem"]')
      ?.focus({ preventScroll: true });
    const outside = (event: PointerEvent) => {
      if (
        !node.contains(event.target as Node) &&
        !anchor.contains(event.target as Node)
      )
        close();
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      close(true);
    };
    const scroll = (event: Event) => {
      if (!node.contains(event.target as Node)) schedulePosition();
    };
    const visibility = new IntersectionObserver(([entry]) => {
      if (!entry.isIntersecting) close();
    });
    visibility.observe(anchor);
    const observer = new ResizeObserver(schedulePosition);
    observer.observe(node);
    observer.observe(anchor);
    window.addEventListener("resize", schedulePosition);
    document.addEventListener("pointerdown", outside, true);
    document.addEventListener("keydown", escape, true);
    document.addEventListener("scroll", scroll, true);
    return () => {
      observer.disconnect();
      visibility.disconnect();
      cancelAnimationFrame(frame);
      window.removeEventListener("resize", schedulePosition);
      document.removeEventListener("pointerdown", outside, true);
      document.removeEventListener("keydown", escape, true);
      document.removeEventListener("scroll", scroll, true);
    };
  }, [anchor, close]);
  return (
    <MenuSurface
      id={id}
      anchor={anchor}
      ref={root}
      className={styles.menu}
      role="menu"
      aria-label={t("Created tasks")}
      onKeyDown={(event) => {
        if (!["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key))
          return;
        event.preventDefault();
        const items = [
          ...event.currentTarget.querySelectorAll<HTMLElement>(
            '[role="menuitem"]:not(:disabled):not([aria-disabled="true"])',
          ),
        ];
        const current = items.indexOf(document.activeElement as HTMLElement);
        const next =
          event.key === "Home"
            ? 0
            : event.key === "End"
              ? items.length - 1
              : (current + (event.key === "ArrowUp" ? -1 : 1) + items.length) %
                items.length;
        items[next]?.focus({ preventScroll: true });
        items[next]?.scrollIntoView({ block: "nearest" });
      }}
      onBlur={(event) => {
        const target = event.relatedTarget as Node | null;
        if (!event.currentTarget.contains(target) && !anchor.contains(target))
          close();
      }}
    >
      <DialogReveal
        from="bottom-left"
        className={styles.content}
        contentClassName={styles.content}
      >
        <MenuFrame
          footer={
            partial && (
              <p className={styles.note}>
                {t("Only the loaded conversation range is shown.")}
              </p>
            )
          }
        >
          <TaskList
            project={project}
            tasks={tasks}
            state={state}
            close={() => close()}
          />
        </MenuFrame>
      </DialogReveal>
    </MenuSurface>
  );
}
