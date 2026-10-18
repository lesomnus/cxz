import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useNavigate } from "@tanstack/react-router";
import { key } from "@lesomnus/payday/store";
import { PageToolbar } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { PanelLink } from "#src/shared/navigation/route-link.tsx";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { SessionIdentity } from "#src/features/session/components/session-identity.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ProjectPicker } from "#src/shared/components/project-picker.tsx";
import styles from "./projects-page.module.css";

export function ProjectsPage({
  projects,
  sessions,
  projectsLoading,
  sessionsLoading,
  error,
  expandProject,
}: {
  projects: Project[];
  sessions: Session[];
  projectsLoading: boolean;
  sessionsLoading?: boolean;
  error?: string;
  expandProject: (id: string) => void;
}) {
  useLocale();
  const navigate = useNavigate();
  const { project: selected } = useWorkspaceRoute();
  const project =
    projects.find((item) => item.runtimeId === selected) ??
    (!selected ? projects[0] : undefined);
  const items = project
    ? sessions.filter(
        (session) =>
          session.project && key(session.project.id) === key(project.id),
      )
    : [];
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
            <>
              <dl className={styles.details}>
                <div>
                  <dt>{t("Workspace")}</dt>
                  <dd>{project.workspace || "—"}</dd>
                </div>
                <div>
                  <dt>{t("Alias")}</dt>
                  <dd>{project.alias || "—"}</dd>
                </div>
                <div>
                  <dt>{t("Status")}</dt>
                  <dd>{project.status?.state || "—"}</dd>
                </div>
              </dl>
              <section aria-label={t("Project sessions")}>
                <h2 className={styles.sectionTitle}>{t("Sessions")}</h2>
                <div className={styles.sessions}>
                  {items.map((session) => (
                    <PanelLink
                      to="/sessions/$sessionId"
                      params={{ sessionId: session.runtimeId }}
                      className={styles.session}
                      key={session.runtimeId}
                      onClick={() => expandProject(project.runtimeId)}
                    >
                      <SessionIdentity session={session} />
                    </PanelLink>
                  ))}
                </div>
                {sessionsLoading && !items.length && (
                  <p className="muted">{t("Loading sessions…")}</p>
                )}
                {!sessionsLoading && !items.length && (
                  <p className="muted">{t("No sessions")}</p>
                )}
              </section>
            </>
          ) : projectsLoading ? (
            <p className="muted">{t("Loading projects…")}</p>
          ) : (
            <p className="muted">
              {selected ? t("Project unavailable") : t("No projects yet.")}
            </p>
          )}
        </div>
      </PanelScroll>
    </main>
  );
}
