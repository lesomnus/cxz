import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import type { Project } from "#gen/cxz/project_pb";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
export function ProjectsPage({
  projects,
  projectsLoading,
  expandProject,
}: {
  projects: Project[];
  projectsLoading: boolean;
  expandProject: (id: string) => void;
}) {
  useLocale();
  return (
    <main className="resource-view">
      <header>
        <div>
          <strong>{t("Projects")}</strong>
          <small>{t("Select a project to browse its sessions.")}</small>
        </div>
      </header>
      <div className="resource-view-content">
        {projects.map((p) => (
          <RouteLink
            to="/sessions"
            className="project-row"
            key={p.runtimeId}
            onClick={() => expandProject(p.runtimeId)}
          >
            <ResourceIcon kind="projects" />
            <span>
              <strong>{p.name || p.alias}</strong>
              <small>
                {p.workspace || p.alias} · {p.status?.state}
              </small>
            </span>
            <span aria-hidden="true">→</span>
          </RouteLink>
        ))}
        {projectsLoading && !projects.length && (
          <p className="muted">{t("Loading projects…")}</p>
        )}
        {!projectsLoading && !projects.length && (
          <p className="muted">{t("No projects yet.")}</p>
        )}
      </div>
    </main>
  );
}
