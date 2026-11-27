import { EditableValue } from "#src/shared/components/editable-value.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useProject } from "#src/shared/api/use-project.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useEffect, useRef, useState } from "react";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useNavigate } from "@tanstack/react-router";
import { key } from "@lesomnus/payday/store";
import { Id } from "@lesomnus/payday/pdid";
import { Button, Section } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import {
  ResourceFields as Fields,
  ResourceFieldsGroup,
  ResourceIdentityFields,
} from "#src/shared/components/resource-fields.tsx";
import { ProjectIconPicker } from "#src/shared/components/project-icon-picker.tsx";
import { type ProjectIconName } from "#src/shared/icons/icon.tsx";
import { projectIconName } from "#src/shared/components/project-icon.ts";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { useRemote } from "#src/shared/api/use-remote.ts";
import {
  ACCOUNT_INFO_TTL,
  ACCOUNT_LIST_KEY,
} from "#src/shared/api/read-cache.ts";
import { type DashboardConnection } from "./project-dashboard";
import {
  editProject,
  setProjectIcon,
  validateProjectField,
  type ProjectField,
} from "./project-edit";
import { ProjectActions } from "./project-actions";
import { ProjectAdd } from "./project-add";
import { ProjectTable } from "./project-table";
import { containerNamePrefixLength } from "./project-container-name";
import { SessionTable } from "#src/features/session/table/session-table.tsx";
import { SessionCreate } from "#src/features/session/components/session-create.tsx";
import { listAccounts } from "#src/features/accounts/account-data.ts";
import type { Account } from "#gen/cxz/account_pb";
import { UsageReport } from "#src/features/activity/usage-report.tsx";
import styles from "./projects-page.module.css";
import { ListDetailPages } from "@lesomnus/nox";
import {
  ResourceLayout,
  ResourcePage,
  ResourceToolbar,
  ResourceDashboard,
  ResourceDashboardColumn,
  ResourceTableSection,
} from "#src/shared/components/resource-layout.tsx";
import dashboardStyles from "#src/shared/components/resource-dashboard.module.css";
import pageStyles from "#src/shared/components/resource-page.module.css";
import {
  resourceHandle,
  uuid,
  useResourceRoutes,
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
  const { projectLocation } = useResourceRoutes();
  useLocale();
  const navigate = useNavigate();
  const [saved, setSaved] = useState<Project>();
  const [adding, setAdding] = useState(false);
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
  const accounts = useRemote((signal) => listAccounts(c, signal), [c], {
    cache: { store: c.reads, key: ACCOUNT_LIST_KEY, ttl: ACCOUNT_INFO_TTL },
  });
  const { project: selected } = useWorkspaceRoute();
  const resolved = useProject(c, visibleProjects, selected);
  const project = selected ? resolved : undefined;
  return (
    <ResourcePage aria-label={t("Project workspace")}>
      <ResourceToolbar
        aria-label={t("Project toolbar")}
        actions={
          <ToolbarIconButton
            icon="add"
            label={t("Add project")}
            onClick={() => setAdding(true)}
          />
        }
      >
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
      </ResourceToolbar>
      <ListDetailPages
        scope={c}
        open={!!selected}
        list={
          <PanelScroll
            contentClassName={pageStyles.contentPadding}
            scrollbarLabel={t("Project scroll")}
            horizontalScrollbar
          >
            <ResourceLayout>
              {error && <ErrorNotice error={error} />}
              <ProjectTable
                c={c}
                projects={visibleProjects}
                sessions={sessions}
                loading={projectsLoading}
                sessionsLoading={sessionsLoading}
                updated={setSaved}
              />
            </ResourceLayout>
          </PanelScroll>
        }
        detail={
          selected ? (
            <PanelScroll
              contentClassName={pageStyles.contentPadding}
              scrollbarLabel={t("Project scroll")}
            >
              <ResourceLayout>
                {error && <ErrorNotice error={error} />}
                {project ? (
                  <ProjectDashboard
                    key={projectId(project)}
                    c={c}
                    project={project}
                    saved={(updated) => {
                      setSaved(updated);
                      if (
                        selected === project.alias &&
                        !uuid(selected) &&
                        updated.alias !== project.alias
                      )
                        void navigate({
                          ...projectLocation(resourceHandle(updated)),
                          replace: true,
                        });
                    }}
                    erased={() => {
                      setSaved(undefined);
                      const next = visibleProjects.find(
                        (p) => key(p.id) !== key(project.id),
                      );
                      void navigate(
                        projectLocation(next && resourceHandle(next)),
                      );
                    }}
                    sessions={sessions.filter(
                      (s) => s.project && key(s.project.id) === key(project.id),
                    )}
                    accounts={accounts.data ?? []}
                    accountsLoading={accounts.loading}
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
              </ResourceLayout>
            </PanelScroll>
          ) : null
        }
      />
      {adding && (
        <ProjectAdd
          c={c}
          close={() => setAdding(false)}
          added={(project, target) => {
            setAdding(false);
            void navigate(projectLocation(resourceHandle(project), target));
          }}
        />
      )}
    </ResourcePage>
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
  accountsLoading,
  accountsError,
  reloadAccounts,
}: {
  c: DashboardConnection;
  project: Project;
  saved: (project: Project) => void;
  erased: () => void;
  accounts: Account[];
  accountsLoading: boolean;
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
  async function saveIcon(name: ProjectIconName | undefined) {
    const updated = await setProjectIcon(c.projects, p, name);
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
    <ResourceDashboard actionsFirstOnNarrow>
      <ResourceDashboardColumn kind="information">
        <Section aria-label={t("Information")}>
          <ResourceFieldsGroup className={styles.informationFields}>
            <ResourceIdentityFields
              rows={[
                {
                  label: t("Name"),
                  value: editable("name", t("Name")),
                },
                {
                  label: t("Alias"),
                  value: editable("alias", t("Alias")),
                },
                {
                  label: t("Project ID"),
                  className: styles.identifierField,
                  value:
                    p.id.length === 16 ? new Id(p.id).toString() : undefined,
                  description: t(
                    "Canonical UUID used to identify this project across APIs and links.",
                  ),
                  identifier: true,
                },
              ]}
              trailing={{
                width: "var(--project-preview-size)",
                field: {
                  label: t("Project icon preview"),
                  hideLabel: true,
                  className: styles.iconPreview,
                  value: (
                    <ProjectIconPicker
                      label={t("Icon")}
                      value={projectIconName(p)}
                      choose={saveIcon}
                    />
                  ),
                },
              }}
            />
            <Fields
              rows={[
                {
                  label: t("Description"),
                  value: editable("desc", t("Description")),
                },
                {
                  label: t("Workspace"),
                  value: p.workspace,
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
                {
                  label: t("Created"),
                  value: <DateTime value={p.dateCreated} />,
                  labelDetail: <RelativeTime value={p.dateCreated} />,
                },
                {
                  label: t("Updated"),
                  value: <DateTime value={p.dateUpdated} />,
                  labelDetail: <RelativeTime value={p.dateUpdated} />,
                },
              ]}
            />
          </ResourceFieldsGroup>
          {p.status?.error && (
            <ErrorNotice api className={styles.errors} error={p.status.error} />
          )}
        </Section>
        <ResourceTableSection aria-label={t("Project sessions")} fillHeight>
          <SessionTable
            c={c}
            sessions={sessions}
            accounts={accounts}
            accountsLoading={accountsLoading}
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
              fixedProject={projectId(p)}
              centered
              close={() => setAddingSession(false)}
            />
          )}
          {accountsError && (
            <div className={styles.empty}>
              <ErrorNotice error={accountsError} />
              <Button type="button" onClick={reloadAccounts}>
                {t("Retry")}
              </Button>
            </div>
          )}
        </ResourceTableSection>
      </ResourceDashboardColumn>
      <ResourceDashboardColumn kind="activity">
        <section className={dashboardStyles.actions} aria-label={t("Actions")}>
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
          className={dashboardStyles.usageSection}
          hideRefresh
          refreshVersion={refreshVersion}
          onLoadingChange={setUsageLoading}
        />
      </ResourceDashboardColumn>
    </ResourceDashboard>
  );
}
