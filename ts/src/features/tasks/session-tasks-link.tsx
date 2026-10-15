import type { Connection } from "#src/shared/api/connection.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { useProjectTasks } from "./task-catalog";
import { activeTask } from "./task-model";
import styles from "./tasks.module.css";

export function SessionTasksLink({
  c,
  project,
  session,
  run,
}: {
  c: Connection;
  project: string;
  session: string;
  run: string;
}) {
  const state = useProjectTasks(c, project);
  const count = state.tasks.filter(
    (task) =>
      task.assignee === session && task.assigneeRun === run && activeTask(task),
  ).length;
  if (!project) return null;
  return (
    <RouteLink
      to="/tasks"
      search={{ project, assignee: session, run }}
      className={styles.sessionTasks}
      data-active={count > 0}
      aria-label={`Project tasks: ${count} assigned to this run`}
      title={
        state.error
          ? "Project tasks unavailable"
          : `${count} active project tasks assigned to this run`
      }
    >
      <span className={styles.count}>
        {state.loading && !state.tasks.length
          ? "…"
          : state.error && !state.tasks.length
            ? "—"
            : Math.min(999, count)}
      </span>
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        <path d="m3 6 1.5 1.5L7 4M10 6h11M3 12h3m4 0h11M3 18h3m4 0h11" />
      </svg>
    </RouteLink>
  );
}
