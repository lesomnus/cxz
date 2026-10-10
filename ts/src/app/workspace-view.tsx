import { useLocale } from "../shared/i18n/i18n-react";
import { useContext } from "react";
import { SettingsPage } from "../pages/settings/settings-page";
import { useWorkspaceRoute } from "./router";
import { WorkspaceContext } from "./workspace-context";
import { ProjectsPage } from "../pages/projects/projects-page";
import { SessionsPage } from "../pages/sessions/sessions-page";
import { NotFoundPage } from "../pages/not-found/not-found-page";
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
