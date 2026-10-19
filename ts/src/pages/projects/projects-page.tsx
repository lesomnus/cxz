import { t } from "#src/shared/i18n/i18n.ts";
import { useId, useState } from "react";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { Link, useNavigate } from "@tanstack/react-router";
import { key } from "@lesomnus/payday/store";
import { PageToolbar, PanelItem, Button } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { IdentifierValue } from "#src/shared/components/identifier-value.tsx";
import { SessionIdentity } from "#src/features/session/components/session-identity.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useProjectTasks } from "#src/features/tasks/task-catalog.ts";
import {
  activeTask,
  taskKey,
  taskStatusName,
  taskTime,
  ownerIssue,
} from "#src/features/tasks/task-model.ts";
import { TaskStatus } from "#gen/cxz/task_pb";
import {
  projectBranch,
  sessionCounts,
  type DashboardConnection,
} from "./project-dashboard";
import { Fields, SessionDashboard } from "./session-dashboard";
import styles from "./projects-page.module.css";

export function ProjectsPage({
  c,
  projects,
  sessions,
  projectsLoading,
  sessionsLoading,
  error,
}: {
  c: DashboardConnection;
  projects: Project[];
  sessions: Session[];
  projectsLoading: boolean;
  sessionsLoading?: boolean;
  error?: string;
}) {
  useLocale();
  const navigate = useNavigate();
  const { project: selected } = useWorkspaceRoute();
  const project =
    projects.find((item) => item.runtimeId === selected) ??
    (!selected ? projects[0] : undefined);
  return (
    <main className={styles.page} aria-label={t("Project workspace")}>
      <PageToolbar aria-label={t("Project toolbar")}>
        <ProjectPicker
          projects={projects}
          value={project?.runtimeId ?? ""}
          loading={projectsLoading}
          choose={(project) =>
            void navigate({ to: "/projects", search: { project } })
          }
        />
      </PageToolbar>
      <PanelScroll
        className={styles.scroll}
        scrollbarLabel={t("Project scroll")}
      >
        <div className={styles.content}>
          {error && <p role="alert">{error}</p>}
          {project ? (
            <ProjectDashboard
              key={project.runtimeId}
              c={c}
              project={project}
              sessions={sessions.filter(
                (s) => s.project && key(s.project.id) === key(project.id),
              )}
              sessionsLoading={sessionsLoading}
            />
          ) : (
            <p className="muted">
              {projectsLoading
                ? t("Loading projects…")
                : selected
                  ? t("Project unavailable")
                  : t("No projects yet.")}
            </p>
          )}
        </div>
      </PanelScroll>
    </main>
  );
}

function ProjectDashboard({
  c,
  project: p,
  sessions,
  sessionsLoading,
}: {
  c: DashboardConnection;
  project: Project;
  sessions: Session[];
  sessionsLoading?: boolean;
}) {
  const [selected, select] = useState("");
  const detailsId = useId();
  const session = sessions.find((s) => s.runtimeId === selected);
  const counts = sessionCounts(sessions);
  const tasks = useProjectTasks(c, p.runtimeId);
  const branch = useRemote(
    (signal) => projectBranch(c, p, signal),
    [c, p.runtimeId, p.status?.state, p.status?.remoteWorkspace],
  );
  const outstanding = tasks.tasks.filter((task) =>
    [TaskStatus.TODO, TaskStatus.IN_PROGRESS, TaskStatus.WAITING].includes(
      task.status,
    ),
  );
  const recentTasks = [...outstanding]
    .sort(
      (a, b) =>
        Number(activeTask(b)) - Number(activeTask(a)) ||
        a.priority - b.priority ||
        taskTime(b) - taskTime(a),
    )
    .slice(0, 6);
  const git = branch.loading
    ? t("Loading…")
    : branch.error
      ? t("Unavailable")
      : branch.data?.state === "branch"
        ? branch.data.value
        : branch.data?.state === "detached"
          ? `${branch.data.value} · ${t("Detached HEAD")}`
          : branch.data?.state === "missing"
            ? t("Not a Git workspace")
            : branch.data?.state === "offline"
              ? t("Project is not running")
              : t("Unavailable");
  return (
    <div className={styles.dashboard}>
      <section
        className={`${styles.section} ${styles.sessionSection}`}
        aria-label={t("Project sessions")}
      >
        <PanelScroll
          key={selected}
          className={styles.sessionDetailsScroll}
          scrollbarLabel={t("Session details")}
        >
          {session ? (
            <SessionDashboard
              key={`${session.runtimeId}/${session.status?.runId}`}
              c={c}
              session={session}
              id={detailsId}
            />
          ) : (
            <p className={styles.selectionPrompt}>
              {t("Select a session to inspect its activity and settings.")}
            </p>
          )}
        </PanelScroll>
        <h2 className={styles.sectionTitle}>{t("Sessions")}</h2>
        <PanelScroll
          className={styles.sessionScroll}
          role="region"
          aria-label={t("Session list")}
          scrollbarLabel={t("Session list")}
        >
          <div className={styles.sessions}>
            {sessions.map((s) => (
              <PanelItem
                type="button"
                className={styles.session}
                key={s.runtimeId}
                aria-pressed={selected === s.runtimeId}
                aria-controls={selected === s.runtimeId ? detailsId : undefined}
                onClick={() => select(s.runtimeId)}
              >
                <SessionIdentity session={s} />
                <span className={styles.sessionState}>
                  {s.status?.state || t("Unknown")}
                </span>
              </PanelItem>
            ))}
          </div>
          {sessionsLoading && !sessions.length && (
            <p className={styles.empty}>{t("Loading sessions…")}</p>
          )}
          {!sessionsLoading && !sessions.length && (
            <p className={styles.empty}>{t("No sessions")}</p>
          )}
        </PanelScroll>
      </section>
      <div className={styles.information}>
        {p.desc && <p className={styles.description}>{p.desc}</p>}
        {p.status?.error && (
          <p className={styles.errors} role="alert">
            {p.status.error}
          </p>
        )}
        <section
          aria-label={t("Session overview")}
          className={styles.counts}
          aria-busy={sessionsLoading}
        >
          {[
            [t("Sessions"), counts.total],
            [t("Working"), counts.working],
            [t("Waiting for input"), counts.waiting],
            [t("Idle"), counts.idle],
            [t("Stopped"), counts.stopped],
            ...(counts.unknown ? [[t("Unknown"), counts.unknown]] : []),
          ].map(([label, count]) => (
            <div key={label}>
              <span>{label}</span>
              <strong>{sessionsLoading ? "—" : count}</strong>
            </div>
          ))}
        </section>
        <section className={styles.section} aria-label={t("Environment")}>
          <div className={styles.sectionHeading}>
            <h2>{t("Environment")}</h2>
            <Button
              className={styles.refresh}
              type="button"
              disabled={branch.loading || p.status?.state !== "running"}
              onClick={branch.reload}
            >
              {t("Refresh")}
            </Button>
          </div>
          <Fields
            rows={[
              [t("Status"), p.status?.state],
              [t("Workspace"), p.workspace],
              [t("Container workspace"), p.status?.remoteWorkspace],
              [t("Remote user"), p.status?.remoteUser],
              [t("Alias"), p.alias],
              [t("Git branch"), git],
              [
                t("Provisioning"),
                [p.status?.provisionState, p.status?.provisionStep]
                  .filter(Boolean)
                  .join(" · "),
              ],
            ]}
          />
          {branch.error && (
            <p className={styles.empty} role="status">
              {branch.error}
            </p>
          )}
        </section>
        <section className={styles.section} aria-label={t("Project tasks")}>
          <div className={styles.sectionHeading}>
            <h2>{t("Tasks")}</h2>
            <Link to="/tasks" search={{ project: p.runtimeId }}>
              {t("Open task board")}
            </Link>
          </div>
          <div className={styles.taskCounts}>
            {[TaskStatus.TODO, TaskStatus.IN_PROGRESS, TaskStatus.WAITING].map(
              (status) => (
                <span key={status}>
                  {t(
                    taskStatusName(status) as
                      | "Todo"
                      | "In progress"
                      | "Waiting",
                  )}{" "}
                  <b>
                    {tasks.loading
                      ? "—"
                      : tasks.tasks.filter((task) => task.status === status)
                          .length}
                  </b>
                </span>
              ),
            )}
          </div>
          {tasks.loading && (
            <p className={styles.empty} role="status">
              {t("Loading tasks…")}
            </p>
          )}
          {tasks.error && (
            <div>
              <p role="alert">{tasks.error}</p>
              <Button type="button" onClick={tasks.refresh}>
                {t("Retry")}
              </Button>
            </div>
          )}
          <ul className={styles.taskList}>
            {recentTasks.map((task) => (
              <li key={taskKey(task)}>
                <Link
                  to="/tasks"
                  search={{ project: p.runtimeId, task: taskKey(task) }}
                >
                  <span>{task.name || task.alias}</span>
                  <small>
                    {t(
                      taskStatusName(task.status) as
                        | "Todo"
                        | "In progress"
                        | "Waiting",
                    )}
                    {task.assignee &&
                      ` · ${sessions.find((s) => s.runtimeId === task.assignee)?.alias || task.assignee}`}
                  </small>
                </Link>
                {ownerIssue(task, sessions, sessionsLoading) && (
                  <span className={styles.empty}>
                    {t(
                      ownerIssue(task, sessions, sessionsLoading) as
                        | "Previous run"
                        | "Session stopped"
                        | "Session unavailable",
                    )}
                  </span>
                )}
              </li>
            ))}
          </ul>
          {!tasks.loading && !tasks.error && !outstanding.length && (
            <p className={styles.empty}>{t("No outstanding tasks")}</p>
          )}
          {outstanding.length > recentTasks.length && (
            <p className={styles.empty}>
              {t("{count} more tasks", {
                count: outstanding.length - recentTasks.length,
              })}
            </p>
          )}
        </section>

        <section
          className={`${styles.section} ${styles.diagnostics}`}
          aria-label={t("Diagnostics")}
        >
          <h2 className={styles.sectionTitle}>{t("Diagnostics")}</h2>
          <h3 className={styles.subheading}>{t("Project")}</h3>
          <Fields
            rows={[
              [t("Runtime ID"), <IdentifierValue value={p.runtimeId} />],
              [
                t("Container ID"),
                <IdentifierValue value={p.status?.containerId} />,
              ],
              [t("Provision attempts"), p.status?.provisionAttempt.toString()],
              [t("Created"), <DateTime value={p.dateCreated} />],
              [t("Updated"), <DateTime value={p.dateUpdated} />],
            ]}
          />
          <h3 className={styles.subheading}>{t("Session")}</h3>
          <Fields
            rows={[
              [t("Runtime ID"), <IdentifierValue value={session?.runtimeId} />],
              [t("Run ID"), <IdentifierValue value={session?.status?.runId} />],
              [
                t("Provider session ID"),
                <IdentifierValue value={session?.status?.vendorId} />,
              ],
              [t("Permission mode"), session?.status?.permissionMode],
              [t("Last event"), session?.status?.lastSeq.toString()],
            ]}
          />
        </section>
      </div>
    </div>
  );
}
