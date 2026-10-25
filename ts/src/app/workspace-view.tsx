import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useContext } from "react";
import { ConnectionsPage } from "#src/pages/settings/connections-page.tsx";
import { SettingsPage } from "#src/pages/settings/settings-page.tsx";
import { LogsPage } from "#src/pages/settings/logs-page.tsx";
import { useWorkspaceRoute } from "./router";
import { WorkspaceContext } from "./workspace-context";
import { ProjectsPage } from "#src/pages/projects/projects-page.tsx";
import { SessionsPage } from "#src/pages/sessions/sessions-page.tsx";
import { NotFoundPage } from "#src/pages/not-found/not-found-page.tsx";
import { AccountsPage } from "#src/pages/accounts/accounts-page.tsx";
import { SearchPage } from "#src/pages/search/search-page.tsx";
import { MemoryPage } from "#src/pages/memory/memory-page.tsx";
import { ActivityPage } from "#src/pages/activity/activity-page.tsx";
import { AuxiliaryPage } from "#src/pages/auxiliary/auxiliary-page.tsx";
import { TasksPage } from "#src/pages/tasks/tasks-page.tsx";
export function WorkspaceView() {
  useLocale();
  const {
    c,
    projects,
    sessions,
    sessionsLoading,
    projectsLoading,
    inventoryError,
    settingsFileOpen,
    setSettingsFileOpen,
  } = useContext(WorkspaceContext)!;
  const { resource, session, settingsTopic } = useWorkspaceRoute();
  if (resource === "not-found") return <NotFoundPage />;
  if (resource === "tasks")
    return (
      <TasksPage
        c={c}
        projects={projects}
        sessions={sessions}
        projectsLoading={projectsLoading}
        sessionsLoading={sessionsLoading}
      />
    );
  if (resource === "accounts")
    return (
      <AccountsPage
        c={c}
        sessions={sessions}
        sessionsLoading={sessionsLoading}
      />
    );
  if (resource === "search") return <SearchPage c={c} projects={projects} />;
  if (resource === "memory") return <MemoryPage c={c} sessions={sessions} />;
  if (resource === "activity")
    return <ActivityPage c={c} sessions={sessions} />;
  if (resource === "auxiliary") return <AuxiliaryPage c={c} />;
  if (resource === "settings" && settingsTopic === "connections")
    return <ConnectionsPage />;
  if (resource === "settings" && settingsTopic === "logs") return <LogsPage />;
  if (resource === "settings")
    return (
      <SettingsPage
        topic={settingsTopic === "editor" ? "editor" : "general"}
        fileOpen={settingsFileOpen}
        setFileOpen={setSettingsFileOpen}
      />
    );
  if (resource === "projects")
    return (
      <ProjectsPage
        c={c}
        projects={projects}
        sessions={sessions}
        sessionsLoading={sessionsLoading}
        projectsLoading={projectsLoading}
        error={inventoryError}
      />
    );
  return <SessionsPage c={c} id={session} projects={projects} />;
}
