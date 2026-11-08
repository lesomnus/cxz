import { useLayoutEffect, useRef } from "react";
import { Button, DialogReveal, MenuSurface } from "@lesomnus/nox";
import { key } from "@lesomnus/payday/store";
import type { Project } from "#gen/cxz/project_pb";
import {
  useProjectTasks,
  type TaskCatalog,
} from "#src/features/tasks/task-catalog.ts";
import { createdWorkTasks } from "#src/features/session/model/created-work-tasks.ts";
import type { WorkGroup } from "#src/features/session/model/work-groups.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ResourceNavigationButton } from "#src/shared/components/resource-navigation-button.tsx";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { taskLocation } from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./created-tasks-dialog.module.css";

export type WorkTaskSource = {
  project: Project;
  session: string;
  taskCatalog: TaskCatalog;
};

function TaskList({
  source,
  group,
  close,
}: {
  source: WorkTaskSource;
  group: WorkGroup;
  close: () => void;
}) {
  const snapshot = useProjectTasks(source, projectId(source.project));
  const tasks = createdWorkTasks(snapshot.tasks, source.session, group);
  return (
    <PanelScroll className={styles.scroll} contentClassName={styles.list}>
      {tasks.map((task) => (
        <ResourceNavigationButton
          key={key(task.id)}
          {...taskLocation({ task, project: source.project })}
          name={task.name || task.alias}
          alias={task.alias}
          className={styles.task}
          onClick={close}
        />
      ))}
      {snapshot.error && (
        <div className={styles.state} role="alert">
          <span>{snapshot.error}</span>
          <Button type="button" onClick={snapshot.refresh}>
            {t("Retry")}
          </Button>
        </div>
      )}
      {!tasks.length && !snapshot.error && (
        <p className={styles.state} role="status">
          {t(
            snapshot.loading
              ? "Loading…"
              : "No Tasks were created in this range.",
          )}
        </p>
      )}
    </PanelScroll>
  );
}

/** Click-only, non-modal Task navigation; never changes transcript geometry. */
export function CreatedTasksDialog({
  id,
  anchor,
  source,
  group,
  partial,
  close,
}: {
  id: string;
  anchor: HTMLButtonElement;
  source?: WorkTaskSource;
  group: WorkGroup;
  partial: boolean;
  close: (restoreFocus?: boolean) => void;
}) {
  useLocale();
  const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const node = root.current;
    if (!node) return;
    const position = () => {
      if (!anchor.isConnected) return close();
      const rect = anchor.getBoundingClientRect();
      const gap =
        parseFloat(getComputedStyle(node).getPropertyValue("--ui-space")) || 4;
      const available = Math.max(0, rect.top - 2 * gap);
      const above = available >= Math.min(120, innerHeight / 3);
      node.style.maxHeight = `${Math.max(0, above ? available : innerHeight - rect.bottom - 2 * gap)}px`;
      const box = node.getBoundingClientRect();
      node.style.left = `${Math.max(gap, Math.min(rect.left, innerWidth - box.width - gap))}px`;
      node.style.top = `${above ? rect.top - box.height - gap : rect.bottom + gap}px`;
      node.style.visibility = "visible";
    };
    position();
    node
      .querySelector<HTMLButtonElement>("button")
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
      if (!node.contains(event.target as Node)) close();
    };
    const observer = new ResizeObserver(position);
    observer.observe(node);
    observer.observe(anchor);
    window.addEventListener("resize", position);
    document.addEventListener("pointerdown", outside, true);
    document.addEventListener("keydown", escape, true);
    document.addEventListener("scroll", scroll, true);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", position);
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
      className={styles.dialog}
      role="dialog"
      aria-label={t("Created tasks")}
      aria-modal={false}
    >
      <DialogReveal
        from="bottom-left"
        className={styles.content}
        contentClassName={styles.content}
      >
        <header className={styles.header}>
          <strong>{t("Created tasks")}</strong>
          <ToolbarIconButton
            icon="close"
            label={t("Close")}
            onClick={() => close(true)}
          />
        </header>
        {source ? (
          <TaskList source={source} group={group} close={() => close()} />
        ) : (
          <p className={styles.state} role="status">
            {t("Task information is unavailable.")}
          </p>
        )}
        {partial && (
          <p className={styles.note}>
            {t("Only the loaded conversation range is shown.")}
          </p>
        )}
      </DialogReveal>
    </MenuSurface>
  );
}
