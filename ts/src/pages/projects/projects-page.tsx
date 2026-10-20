import { t } from "#src/shared/i18n/i18n.ts";
import { useState, type ReactNode } from "react";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useNavigate } from "@tanstack/react-router";
import { key } from "@lesomnus/payday/store";
import { Id } from "@lesomnus/payday/pdid";
import { PageToolbar, Button, EditableValue } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { FieldLabel } from "#src/shared/components/field-label.tsx";
import { IdentifierValue } from "#src/shared/components/identifier-value.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { projectBranch, type DashboardConnection } from "./project-dashboard";
import {
  editProject,
  validateProjectField,
  type ProjectField,
} from "./project-edit";
import { ProjectActions } from "./project-actions";
import { ProjectSessions } from "./project-sessions";
import { SessionCreate } from "#src/features/session/components/session-create.tsx";
import { listAccounts } from "#src/features/accounts/account-data.ts";
import type { Account } from "#gen/cxz/account_pb";
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
  const [saved, setSaved] = useState<Project>();
  const visibleProjects = projects.map((p) => {
    if (!saved || key(p.id) !== key(saved.id)) return p;
    const a = p.dateUpdated,
      b = saved.dateUpdated;
    return b &&
      (!a ||
        b.seconds > a.seconds ||
        (b.seconds === a.seconds && b.nanos > a.nanos))
      ? saved
      : p;
  });
  const accounts = useRemote((signal) => listAccounts(c, signal), [c]);
  const { project: selected } = useWorkspaceRoute();
  const project =
    visibleProjects.find((item) => item.runtimeId === selected) ??
    (!selected ? visibleProjects[0] : undefined);
  return (
    <main className={styles.page} aria-label={t("Project workspace")}>
      <PageToolbar aria-label={t("Project toolbar")}>
        <ProjectPicker
          projects={visibleProjects}
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
              saved={setSaved}
              purged={() => {
                setSaved(undefined);
                const next = visibleProjects.find(
                  (p) => key(p.id) !== key(project.id),
                );
                void navigate({
                  to: "/projects",
                  search: { project: next?.runtimeId },
                });
              }}
              sessions={sessions.filter(
                (s) => s.project && key(s.project.id) === key(project.id),
              )}
              accounts={accounts.data ?? []}
              accountsError={accounts.error}
              reloadAccounts={accounts.reload}
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
  saved,
  purged,
  sessions,
  sessionsLoading,
  accounts,
  accountsError,
  reloadAccounts,
}: {
  c: DashboardConnection;
  project: Project;
  saved: (project: Project) => void;
  purged: () => void;
  accounts: Account[];
  accountsError: string;
  reloadAccounts: () => void;
  sessions: Session[];
  sessionsLoading?: boolean;
}) {
  async function save(field: ProjectField, value: string) {
    const updated = await editProject(c.projects, p, field, value);
    c.store.apply("cxz.Project", [{ id: updated.id, value: updated }]);
    saved(updated);
  }
  const editable = (field: ProjectField, label: string) => (
    <div className={styles.editable}>
      <EditableValue
        label={label}
        value={p[field]}
        save={(value) => save(field, value)}
        validate={(value) => validateProjectField(field, value)}
        monospace
      />
    </div>
  );
  const [addingSession, setAddingSession] = useState(false);
  const branch = useRemote(
    (signal) => projectBranch(c, p, signal),
    [c, p.runtimeId, p.status?.state, p.status?.remoteWorkspace],
  );
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
      <div className={styles.information}>
        <section className={styles.section} aria-label={t("Information")}>
          <h2 className={styles.sectionTitle}>{t("Information")}</h2>
          <Fields
            rows={[
              [
                t("Project ID"),
                <IdentifierValue
                  value={
                    p.id.length === 16 ? new Id(p.id).toString() : undefined
                  }
                />,
              ],
              [t("Name"), editable("name", t("Name"))],
              [t("Alias"), editable("alias", t("Alias"))],
              [t("Description"), editable("desc", t("Description"))],
              [t("Workspace"), p.workspace],
              [t("Configuration"), p.config],
              [
                t("Runtime ID"),
                <IdentifierValue value={p.runtimeId} />,
                t(
                  "Project identifier used in API calls and links. Stored as runtime_id, separately from the database UUID and container ID.",
                ),
              ],
              [t("Listed"), String(p.listed)],
              [t("Created"), <DateTime value={p.dateCreated} />],
              [t("Updated"), <DateTime value={p.dateUpdated} />],
              [t("Deleted"), <DateTime value={p.dateErased} />],
            ]}
          />
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
              [t("Workspace"), p.status?.remoteWorkspace],
              [t("Remote user"), p.status?.remoteUser],
              [t("Git branch"), git],
              [
                t("Provisioning"),
                [p.status?.provisionState, p.status?.provisionStep]
                  .filter(Boolean)
                  .join(" · "),
                t(
                  "Status and current step of preparing the project container and workspace.",
                ),
              ],
            ]}
          />
          {p.status?.error && (
            <p className={styles.errors} role="alert">
              {p.status.error}
            </p>
          )}
          {branch.error && (
            <p className={styles.empty} role="status">
              {branch.error}
            </p>
          )}
        </section>
        <section className={styles.section} aria-label={t("Diagnostics")}>
          <h2 className={styles.sectionTitle}>{t("Diagnostics")}</h2>
          <Fields
            rows={[
              [
                t("Container ID"),
                <IdentifierValue value={p.status?.containerId} />,
              ],
              [t("Provision attempts"), p.status?.provisionAttempt.toString()],
            ]}
          />
        </section>
        <section className={styles.section} aria-label={t("Actions")}>
          <h2 className={styles.sectionTitle}>{t("Actions")}</h2>
          <ProjectActions
            c={c}
            project={p}
            sessions={sessions}
            purged={purged}
          />
        </section>
      </div>
      <section
        className={`${styles.section} ${styles.sessionSection}`}
        aria-label={t("Project sessions")}
      >
        <ProjectSessions
          c={c}
          sessions={sessions}
          accounts={accounts}
          loading={sessionsLoading}
          addSession={() => setAddingSession(true)}
        />
        {addingSession && (
          <SessionCreate
            c={c}
            projects={[p]}
            centered
            close={() => setAddingSession(false)}
          />
        )}
        {accountsError && (
          <div className={styles.empty}>
            <p role="alert">{accountsError}</p>
            <Button type="button" onClick={reloadAccounts}>
              {t("Retry")}
            </Button>
          </div>
        )}
      </section>
    </div>
  );
}

function Fields({ rows }: { rows: [string, ReactNode, string?][] }) {
  return (
    <dl className={styles.details}>
      {rows.map(([label, value, description]) => (
        <div key={label}>
          <dt>
            <FieldLabel label={label} description={description} />
          </dt>
          <dd>{value === undefined || value === "" ? "—" : value}</dd>
        </div>
      ))}
    </dl>
  );
}
