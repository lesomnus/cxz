import { WorkResourcesMenu } from "./work-resources-menu";
import { MenuScrollArea, MenuItem, MenuItemLabel } from "@lesomnus/nox";
import { key } from "@lesomnus/payday/store";
import type { Project } from "#gen/cxz/project_pb";
import type { Task } from "#gen/cxz/task_pb";
import type { WorkTaskState } from "#src/features/session/model/work-tasks.ts";
import { TaskPrioritySymbol } from "#src/features/tasks/task-indicators.tsx";
import { MenuLink } from "#src/shared/navigation/route-link.tsx";
import { taskLocation } from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./work-resources-menu.module.css";

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

export function CreatedTasksMenu({
  project,
  tasks,
  state,
  ...props
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
  return (
    <WorkResourcesMenu {...props} label={t("Created tasks")}>
      <TaskList
        project={project}
        tasks={tasks}
        state={state}
        close={() => props.close()}
      />
    </WorkResourcesMenu>
  );
}
