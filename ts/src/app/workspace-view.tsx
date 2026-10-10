import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useContext } from "react";
import { SettingsPage } from "#src/pages/settings/settings-page.tsx";
import { useWorkspaceRoute } from "./router";
import { WorkspaceContext } from "./workspace-context";
import { ProjectsPage } from "#src/pages/projects/projects-page.tsx";
import { SessionsPage } from "#src/pages/sessions/sessions-page.tsx";
import { NotFoundPage } from "#src/pages/not-found/not-found-page.tsx";
export function WorkspaceView() {
  useLocale();
  const {
    c,
    projects,
    projectsLoading,
    settingsFileOpen,
    setSettingsFileOpen,
    expandProject,
  } = useContext(WorkspaceContext)!;
  const { resource, session, settingsTopic } = useWorkspaceRoute();
  if (resource === "not-found") return <NotFoundPage />;
  if (resource === "settings")
    return (
      <SettingsPage
        topic={settingsTopic}
        fileOpen={settingsFileOpen}
        setFileOpen={setSettingsFileOpen}
      />
    );
  if (resource === "projects")
    return (
      <ProjectsPage
        projects={projects}
        projectsLoading={projectsLoading}
        expandProject={expandProject}
      />
    );
  return <SessionsPage c={c} id={session} projects={projects} />;
}
