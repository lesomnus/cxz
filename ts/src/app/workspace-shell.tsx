import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  type PropsWithChildren,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { Session } from "#gen/cxz/session_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { ProfileMenu } from "./profile-menu";
import { SessionTreeGroup } from "#src/features/session/components/session-tree.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { useScrollbars } from "#src/shared/scroll/scrollbars.ts";
import { useResourceInventory } from "#src/shared/api/resource-inventory.ts";
import { key } from "@lesomnus/payday/store";
import { RouteLink, PanelLink } from "#src/shared/navigation/route-link.tsx";
import { useWorkspaceRoute } from "./router";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { WorkspaceContext } from "./workspace-context";
import { useSessionAttention } from "#src/features/session/model/use-session-attention.ts";
export function Workspace({
  connection: c,
  logout,
  exitLabel = t("Sign out"),
  children,
}: {
  connection: Connection;
  logout: () => Promise<void>;
  exitLabel?: string;
} & PropsWithChildren) {
  useLocale();
  useScrollbars();
  const route = useWorkspaceRoute();
  const resource = route.resource === "not-found" ? "sessions" : route.resource;
  const session = route.session;
  const settingsTopic = route.settingsTopic;
  const lastSession = useRef("");
  if (route.resource === "sessions") lastSession.current = session;
  const [settingsFileOpen, setSettingsFileOpen] = useState(false);
  useEffect(() => setSettingsFileOpen(false), [resource, settingsTopic]);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const inventory = useResourceInventory(c);
  const unread = useSessionAttention(
    c,
    inventory.sessions,
    resource === "sessions" ? session : "",
  );
  const projects = inventory.projects;
  const sessionsByProject = useMemo(() => {
    const grouped = new Map<string, Session[]>();
    for (const session of inventory.sessions) {
      if (!session.project) continue;
      const id = key(session.project.id);
      const sessions = grouped.get(id) ?? [];
      sessions.push(session);
      grouped.set(id, sessions);
    }
    return grouped;
  }, [inventory.sessions]);
  function expandProject(id: string) {
    setCollapsed((old) => {
      const next = new Set(old);
      next.delete(id);
      return next;
    });
  }
  return (
    <WorkspaceContext.Provider
      value={{
        c,
        projects,
        sessions: inventory.sessions,
        sessionsLoading: inventory.loading.sessions,
        projectsLoading: inventory.loading.projects,
        settingsFileOpen,
        setSettingsFileOpen,
        expandProject,
      }}
    >
      <div
        className={`workspace ${resource === "accounts" ? "accounts-open" : ""} ${(session && resource === "sessions") || route.resource === "not-found" ? "conversation-open" : ""} ${resource === "settings" ? "settings-open" : !session || resource !== "sessions" ? "resource-open" : ""}`}
      >
        <nav className="resource-sidebar" aria-label={t("Resources")}>
          <span className="brand" aria-label="cxz">
            cxz
          </span>
          {(["sessions", "projects"] as const).map((view) => (
            <RouteLink
              key={view}
              {...(view === "projects"
                ? { to: "/projects" as const }
                : lastSession.current
                  ? {
                      to: "/sessions/$sessionId" as const,
                      params: { sessionId: lastSession.current },
                    }
                  : { to: "/sessions" as const })}
              className={`resource-link ${resource === view ? "active" : ""}`}
              aria-label={
                view === "sessions" ? t("Sessions view") : t("Projects view")
              }
              aria-current={resource === view ? "page" : undefined}
              title={view === "sessions" ? t("Sessions") : t("Projects")}
            >
              <ResourceIcon
                kind={view}
                unreadCount={view === "sessions" ? unread.size : 0}
              />
              <span>{view === "sessions" ? t("Sessions") : t("Projects")}</span>
            </RouteLink>
          ))}
          {(
            ["accounts", "search", "memory", "activity", "auxiliary"] as const
          ).map((view) => (
            <RouteLink
              key={view}
              to={`/${view}`}
              className={`resource-link ${resource === view ? "active" : ""}`}
              aria-label={
                view === "auxiliary"
                  ? "Auxiliary AI"
                  : view[0].toUpperCase() + view.slice(1)
              }
              title={
                view === "auxiliary"
                  ? "Auxiliary AI"
                  : view[0].toUpperCase() + view.slice(1)
              }
              aria-current={resource === view ? "page" : undefined}
            >
              <ResourceIcon kind={view} />
              <span>
                {view === "auxiliary"
                  ? "Auxiliary AI"
                  : view[0].toUpperCase() + view.slice(1)}
              </span>
            </RouteLink>
          ))}
          <ProfileMenu
            logout={logout}
            exitLabel={exitLabel}
            openSettings={() => setSettingsFileOpen(false)}
          />
        </nav>
        {resource !== "accounts" && (
          <aside
            className={`resource-panel${resource === "sessions" ? " session-panel" : ""}`}
            aria-label={
              resource === "sessions"
                ? t("Session list")
                : resource === "settings"
                  ? t("Settings navigation")
                  : resource === "projects"
                    ? t("Project list")
                    : `${resource} navigation`
            }
          >
            <header>
              <strong>
                {resource === "sessions"
                  ? t("Sessions")
                  : resource === "settings"
                    ? t("Settings")
                    : resource === "projects"
                      ? t("Projects")
                      : resource === "auxiliary"
                        ? "Auxiliary AI"
                        : resource[0].toUpperCase() + resource.slice(1)}
              </strong>
              {resource === "sessions" && (
                <RouteLink
                  to="/sessions"
                  title="Manage sessions"
                  aria-label="Manage sessions"
                >
                  ＋
                </RouteLink>
              )}
            </header>
            <p className="muted">{new URL(c.baseUrl).host}</p>
            <PanelScroll>
              {resource === "settings" ? (
                <nav
                  className="settings-topics"
                  aria-label={t("Settings topics")}
                >
                  {(["general", "editor"] as const).map((topic) => (
                    <PanelLink
                      key={topic}
                      to={
                        topic === "general"
                          ? "/settings/general"
                          : "/settings/editor"
                      }
                      aria-current={
                        settingsTopic === topic ? "page" : undefined
                      }
                      aria-controls="settings-editor"
                      onClick={() => {
                        setSettingsFileOpen(false);
                      }}
                    >
                      {topic === "general" ? t("General") : t("Editor")}
                    </PanelLink>
                  ))}
                </nav>
              ) : resource === "sessions" ? (
                <div
                  className="session-tree"
                  aria-label={t("Projects and sessions")}
                >
                  {projects.map((p) => (
                    <SessionTreeGroup
                      key={p.runtimeId}
                      project={p}
                      items={sessionsByProject.get(key(p.id)) ?? []}
                      loading={inventory.loading.sessions}
                      selected={session}
                      unread={unread}
                      open={!collapsed.has(p.runtimeId)}
                      toggle={() =>
                        setCollapsed((old) => {
                          const next = new Set(old);
                          if (next.has(p.runtimeId)) next.delete(p.runtimeId);
                          else next.add(p.runtimeId);
                          return next;
                        })
                      }
                    />
                  ))}
                </div>
              ) : resource === "projects" ? (
                projects.map((p) => (
                  <PanelLink
                    key={p.runtimeId}
                    to="/sessions"
                    onClick={() => expandProject(p.runtimeId)}
                  >
                    {p.name || p.alias}
                  </PanelLink>
                ))
              ) : (
                <nav
                  className="settings-topics"
                  aria-label="Resource navigation"
                >
                  <PanelLink to={`/${resource}`} aria-current="page">
                    {resource === "memory"
                      ? "Shared memory"
                      : resource === "auxiliary"
                        ? "Auxiliary AI"
                        : resource[0].toUpperCase() + resource.slice(1)}
                  </PanelLink>
                </nav>
              )}
              {!!(inventory.errors.projects || inventory.errors.sessions) && (
                <p role="alert">
                  {String(
                    inventory.errors.projects || inventory.errors.sessions,
                  )}
                </p>
              )}
              {inventory.loading.projects && !projects.length && (
                <p className="muted">{t("Loading projects…")}</p>
              )}
            </PanelScroll>
          </aside>
        )}
        {children}
      </div>
    </WorkspaceContext.Provider>
  );
}
