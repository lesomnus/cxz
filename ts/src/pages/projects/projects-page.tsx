import { useProject } from "#src/shared/api/use-project.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useNavigate } from "@tanstack/react-router";
import { key } from "@lesomnus/payday/store";
import { Id } from "@lesomnus/payday/pdid";
import {
  PageToolbar,
  Button,
  EditableValue,
  Section,
  SectionGroup,
  SlidingPages,
  SlidingPage,
} from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { ResourceFields as Fields } from "#src/shared/components/resource-fields.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { type DashboardConnection } from "./project-dashboard";
import {
  editProject,
  validateProjectField,
  type ProjectField,
} from "./project-edit";
import { ProjectActions } from "./project-actions";
import { ProjectTable } from "./project-table";
import { containerNamePrefixLength } from "./project-container-name";
import { SessionTable } from "#src/features/session/table/session-table.tsx";
import { SessionCreate } from "#src/features/session/components/session-create.tsx";
import { listAccounts } from "#src/features/accounts/account-data.ts";
import type { Account } from "#gen/cxz/account_pb";
import { UsageReport } from "#src/features/activity/usage-report.tsx";
import styles from "./projects-page.module.css";
import dashboardStyles from "#src/shared/components/resource-dashboard.module.css";
import {
  projectLocation,
  resourceHandle,
  uuid,
} from "#src/shared/navigation/resource-route.ts";

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
  const resolved = useProject(c, visibleProjects, selected);
  const project = selected ? resolved : undefined;
  const visitedList = useRef<DashboardConnection | undefined>(
    selected ? undefined : c,
  );
  const previousProject = useRef<
    { c: DashboardConnection; project: Project } | undefined
  >(project ? { c, project } : undefined);
  useLayoutEffect(() => {
    if (!selected) visitedList.current = c;
    if (project) previousProject.current = { c, project };
  }, [c, selected, project]);
  // Keep the outgoing dashboard present while it slides back offscreen.
  const detail = selected
    ? project
    : previousProject.current?.c === c
      ? previousProject.current.project
      : undefined;
  return (
    <main className={styles.page} aria-label={t("Project workspace")}>
      <PageToolbar className={styles.toolbar} aria-label={t("Project toolbar")}>
        <div className={styles.toolbarContent}>
          <ProjectPicker
            projects={visibleProjects}
            value={projectId(project) ?? ""}
            loading={projectsLoading}
            choose={(project) =>
              void navigate(
                projectLocation(
                  resourceHandle(
                    visibleProjects.find((p) => projectId(p) === project)!,
                  ),
                ),
              )
            }
          />
        </div>
      </PageToolbar>
      <SlidingPages
        className={styles.pages}
        value={selected ? "detail" : "list"}
        resize="none"
      >
        <SlidingPage value="list" className={styles.slide}>
          {(!selected || visitedList.current === c) && (
            <PanelScroll
              className={styles.scroll}
              contentClassName={styles.scrollContent}
              scrollbarLabel={t("Project scroll")}
              horizontalScrollbar
            >
              <div className={styles.content}>
                {error && <p role="alert">{error}</p>}
                <ProjectTable
                  c={c}
                  projects={visibleProjects}
                  sessions={sessions}
                  loading={projectsLoading}
                  sessionsLoading={sessionsLoading}
                  updated={setSaved}
                />
              </div>
            </PanelScroll>
          )}
        </SlidingPage>
        <SlidingPage value="detail" className={styles.slide}>
          {(selected || detail) && (
            <PanelScroll
              className={styles.scroll}
              contentClassName={styles.scrollContent}
              scrollbarLabel={t("Project scroll")}
            >
              <div className={styles.content}>
                {error && <p role="alert">{error}</p>}
                {detail ? (
                  <ProjectDashboard
                    key={projectId(detail)}
                    c={c}
                    project={detail}
                    saved={(updated) => {
                      setSaved(updated);
                      if (
                        selected === detail.alias &&
                        !uuid(selected) &&
                        updated.alias !== detail.alias
                      )
                        void navigate({
                          ...projectLocation(resourceHandle(updated)),
                          replace: true,
                        });
                    }}
                    erased={() => {
                      setSaved(undefined);
                      const next = visibleProjects.find(
                        (p) => key(p.id) !== key(detail.id),
                      );
                      void navigate(
                        projectLocation(next && resourceHandle(next)),
                      );
                    }}
                    sessions={sessions.filter(
                      (s) => s.project && key(s.project.id) === key(detail.id),
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
          )}
        </SlidingPage>
      </SlidingPages>
    </main>
  );
}

function ProjectDashboard({
  c,
  project: p,
  saved,
  erased,
  sessions,
  sessionsLoading,
  accounts,
  accountsError,
  reloadAccounts,
}: {
  c: DashboardConnection;
  project: Project;
  saved: (project: Project) => void;
  erased: () => void;
  accounts: Account[];
  accountsError: string;
  reloadAccounts: () => Promise<void>;
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
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [usageLoading, setUsageLoading] = useState(false);
  const refreshRequest = useRef<AbortController | undefined>(undefined);
  useEffect(() => () => refreshRequest.current?.abort(), []);
  async function refreshProject() {
    const controller = new AbortController();
    refreshRequest.current?.abort();
    refreshRequest.current = controller;
    c.inventory.refresh();
    setRefreshVersion((version) => version + 1);
    try {
      const [updated] = await Promise.all([
        c.projects.get(
          { ref: { key: { case: "id", value: p.id } }, select: { all: true } },
          { signal: controller.signal, timeoutMs: 10000 },
        ),
        reloadAccounts(),
      ]);
      controller.signal.throwIfAborted();
      c.store.apply("cxz.Project", [{ id: updated.id, value: updated }]);
      saved(updated);
    } finally {
      if (refreshRequest.current === controller)
        refreshRequest.current = undefined;
    }
  }
  return (
    <div className={`${styles.dashboard} ${dashboardStyles.layout}`}>
      <div className={styles.columns}>
        <SectionGroup className={styles.information}>
          <Section aria-label={t("Information")}>
            <Fields
              rows={[
                {
                  label: t("Project ID"),
                  value:
                    p.id.length === 16 ? new Id(p.id).toString() : undefined,
                  description: t(
                    "Canonical UUID used to identify this project across APIs and links.",
                  ),
                  identifier: true,
                },
                [t("Name"), editable("name", t("Name"))],
                [t("Alias"), editable("alias", t("Alias"))],
                [t("Description"), editable("desc", t("Description"))],
                {
                  label: t("Workspace"),
                  value: p.workspace,
                  fullWidth: true,
                  path: true,
                },
                {
                  label: t("Container name"),
                  value: p.status?.containerName,
                  identifier: true,
                  identifierLength: containerNamePrefixLength(
                    p.status?.containerName,
                  ),
                },
                {
                  label: t("Container ID"),
                  value: p.status?.containerId,
                  identifier: true,
                },
                [
                  t("Created"),
                  <DateTime value={p.dateCreated} />,
                  undefined,
                  <RelativeTime value={p.dateCreated} />,
                ],
                [
                  t("Updated"),
                  <DateTime value={p.dateUpdated} />,
                  undefined,
                  <RelativeTime value={p.dateUpdated} />,
                ],
              ]}
            />
            {p.status?.error && (
              <p className={styles.errors} role="alert">
                {p.status.error}
              </p>
            )}
          </Section>
          <Section
            className={styles.sessionSection}
            aria-label={t("Project sessions")}
          >
            <SessionTable
              c={c}
              sessions={sessions}
              accounts={accounts}
              loading={sessionsLoading}
              variant="overview"
              showToolbar={false}
              horizontalScrollbar
              fillHeight
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
          </Section>
        </SectionGroup>
        <SectionGroup className={styles.usageColumn}>
          <section className={styles.actions} aria-label={t("Actions")}>
            <ProjectActions
              c={c}
              project={p}
              sessions={sessions}
              erased={erased}
              updated={saved}
              refresh={refreshProject}
              refreshing={usageLoading}
            />
          </section>
          <UsageReport
            c={c}
            project={projectId(p)}
            className={styles.usageSection}
            hideRefresh
            refreshVersion={refreshVersion}
            onLoadingChange={setUsageLoading}
          />
        </SectionGroup>
      </div>
    </div>
  );
}
