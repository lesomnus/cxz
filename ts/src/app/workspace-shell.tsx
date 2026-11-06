import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { projectId } from "#src/shared/api/project-id.ts";
import { SidebarHint } from "#src/shared/navigation/sidebar-hint.tsx";
import { useBackgroundSessions } from "#src/features/session/model/use-background-sessions.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  type PropsWithChildren,
  useEffect,
  useCallback,
  useMemo,
  useRef,
  useState,
} from "react";
import type { Session } from "#gen/cxz/session_pb";
import { ContainerState } from "#gen/cxz/project_pb";
import { ActionMenu } from "@lesomnus/nox";
import { Connection } from "#src/shared/api/connection.ts";
import { ProfileMenu } from "./profile-menu";
import { ErrorCenter } from "./error-center";
import { ErrorCenterProvider } from "#src/shared/errors/error-context.tsx";
import { SessionTreeGroup } from "#src/features/session/components/session-tree.tsx";
import { ResourcePanel } from "#src/shared/components/resource-panel.tsx";
import { SessionCreate } from "#src/features/session/components/session-create.tsx";
import { useScrollbars } from "#src/shared/scroll/scrollbars.ts";
import { useResourceInventory } from "#src/shared/api/resource-inventory.ts";
import { key } from "@lesomnus/payday/store";
import { RouteLink, PanelLink } from "#src/shared/navigation/route-link.tsx";
import { useWorkspaceRoute } from "./router";
import { useWorkspaceProject } from "./use-workspace-project";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { WorkspaceContext } from "./workspace-context";
import { useSessionAttention } from "#src/features/session/model/use-session-attention.ts";
import { resumableStates } from "#src/features/session/model/session-actions.ts";
import { useSettings } from "#src/shared/settings/settings.ts";
import {
  matchesResource,
  memoryLocation,
  projectLocation,
  resourceLocation,
  sessionLocation,
  taskLocation,
} from "#src/shared/navigation/resource-route.ts";
type WorkspaceProps = {
  connection: Connection;
  logout: () => Promise<void>;
  exitLabel?: string;
} & PropsWithChildren;
export function Workspace(props: WorkspaceProps) {
  const route = useWorkspaceRoute();
  const [compact, setCompact] = useState(
    () => window.matchMedia("(max-width: 700px)").matches,
  );
  useEffect(() => {
    const media = window.matchMedia("(max-width: 700px)");
    const update = () => setCompact(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  return (
    <ErrorCenterProvider
      store={props.connection.errors}
      localErrors={compact && route.resource === "sessions" && !!route.session}
    >
      <WorkspaceContent {...props} />
    </ErrorCenterProvider>
  );
}
function WorkspaceContent({
  connection: c,
  logout,
  exitLabel = t("Sign out"),
  children,
}: WorkspaceProps) {
  useLocale();
  useScrollbars();
  useEffect(() => c.conversations.mount(), [c]);
  const route = useWorkspaceRoute();
  const resource = route.resource === "not-found" ? "sessions" : route.resource;
  const inventory = useResourceInventory(c);
  useEffect(() => {
    for (const project of inventory.projects) {
      if (!project.status) continue;
      const id = projectId(project);
      c.errors.observe(
        JSON.stringify([c.baseUrl, "project-status", id]),
        project.status.error ?? "",
        {
          source: "resource",
          connection: c.baseUrl,
          project: id,
          method: "Project runtime",
          target: project.name || project.alias || id,
        },
      );
    }
  }, [c, inventory.projects]);
  const selectedSession = inventory.sessions.find((s) =>
    matchesResource(s, route.session),
  );
  const currentProject = useWorkspaceProject(
    c,
    inventory.projects,
    route,
    selectedSession,
  );
  const session = selectedSession?.runtimeId ?? route.session;
  const hasResourcePanel = ![
    "accounts",
    "tasks",
    "projects",
    "memory",
    "search",
  ].includes(resource);
  const settingsTopic = route.settingsTopic;
  const lastSession = useRef("");
  if (route.resource === "sessions" && route.session)
    lastSession.current = selectedSession?.alias || route.session;
  const [settingsFileOpen, setSettingsFileOpen] = useState(false);
  useEffect(() => setSettingsFileOpen(false), [resource, settingsTopic]);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [addingSession, setAddingSession] = useState(false);
  const { store: settings, snapshot } = useSettings();
  const hideStopped = snapshot.document["session.panel.hideStopped"] === true;
  const projectCollapse =
    snapshot.document["session.panel.projectCollapse"] !== false;
  const [panelError, setPanelError] = useState("");
  const sessionPanel = useRef<HTMLElement>(null);
  const positionSessionMenu = useCallback((trigger: DOMRect) => {
    const header = sessionPanel.current?.querySelector("header");
    const edge = header?.getBoundingClientRect().right ?? trigger.right;
    const border = header
      ? parseFloat(getComputedStyle(header).borderRightWidth) || 0
      : 0;
    // Mirror the empty space, not the panel's visible border thickness.
    return {
      left: edge + Math.max(0, edge - border - trigger.right),
      top: trigger.top,
    };
  }, []);
  useEffect(() => setAddingSession(false), [resource, session]);
  const { unread, errors } = useSessionAttention(
    c,
    inventory.sessions,
    resource === "sessions" ? session : "",
  );
  const background = useBackgroundSessions(c, inventory.sessions);
  const projects = inventory.projects;
  const memoryProject =
    projects.find(
      (p) => currentProject && matchesResource(p, currentProject),
    ) ?? projects[0];
  const sessionsByProject = useMemo(() => {
    const grouped = new Map<string, Session[]>();
    for (const session of inventory.sessions) {
      if (
        !session.project ||
        (hideStopped && resumableStates.has(session.status?.state ?? ""))
      )
        continue;
      const id = key(session.project.id);
      const sessions = grouped.get(id) ?? [];
      sessions.push(session);
      grouped.set(id, sessions);
    }
    return grouped;
  }, [inventory.sessions, hideStopped]);
  return (
    <WorkspaceContext.Provider
      value={{
        c,
        projects,
        sessions: inventory.sessions,
        sessionsLoading: inventory.loading.sessions,
        projectsLoading: inventory.loading.projects,
        inventoryError:
          inventory.errors.projects || inventory.errors.sessions
            ? String(inventory.errors.projects || inventory.errors.sessions)
            : "",
        settingsFileOpen,
        setSettingsFileOpen,
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
            <SidebarHint
              key={view}
              label={view === "sessions" ? t("Sessions") : t("Projects")}
            >
              <RouteLink
                {...(route.resource === view
                  ? resourceLocation(view)
                  : view === "projects"
                    ? projectLocation(currentProject)
                    : lastSession.current
                      ? sessionLocation(lastSession.current)
                      : resourceLocation("sessions"))}
                className={`resource-link ${resource === view ? "active" : ""}`}
                aria-label={
                  view === "sessions" ? t("Sessions view") : t("Projects view")
                }
                aria-current={resource === view ? "page" : undefined}
              >
                <ResourceIcon
                  kind={view}
                  unreadCount={view === "sessions" ? unread.size : 0}
                  error={view === "sessions" && errors.size > 0}
                />
              </RouteLink>
            </SidebarHint>
          ))}
          {(
            ["tasks", "accounts", "search", "memory", "auxiliary"] as const
          ).map((view) => (
            <SidebarHint
              key={view}
              label={
                view === "auxiliary"
                  ? "Auxiliary AI"
                  : view[0].toUpperCase() + view.slice(1)
              }
            >
              <RouteLink
                {...(view === "tasks"
                  ? taskLocation({ project: currentProject })
                  : view === "memory"
                    ? memoryProject
                      ? memoryLocation(memoryProject)
                      : projectLocation()
                    : resourceLocation(view))}
                className={`resource-link ${resource === view ? "active" : ""}`}
                aria-label={
                  view === "auxiliary"
                    ? "Auxiliary AI"
                    : view[0].toUpperCase() + view.slice(1)
                }
                aria-current={resource === view ? "page" : undefined}
              >
                <ResourceIcon kind={view} />
              </RouteLink>
            </SidebarHint>
          ))}
          <div className="sidebar-footer">
            <ErrorCenter store={c.errors} />
            <ProfileMenu
              errors={c.errors}
              logout={logout}
              exitLabel={exitLabel}
              openSettings={() => setSettingsFileOpen(false)}
            />
          </div>
        </nav>
        {hasResourcePanel && (
          <ResourcePanel
            ref={sessionPanel}
            unreadSelector={
              resource === "sessions" ? '[data-unread="true"]' : undefined
            }
            className={resource === "sessions" ? "session-panel" : ""}
            title={
              resource === "sessions"
                ? t("Sessions")
                : resource === "settings"
                  ? t("Settings")
                  : resource === "auxiliary"
                    ? "Auxiliary AI"
                    : resource[0].toUpperCase() + resource.slice(1)
            }
            actions={
              resource === "sessions" && (
                <ActionMenu
                  label="Session panel options"
                  placement="right"
                  motion={{ from: "top-left", effect: "reveal" }}
                  position={positionSessionMenu}
                  triggerClassName="session-panel-menu-trigger"
                  items={[
                    {
                      label: "Add session",
                      icon: (
                        <svg
                          width="16"
                          height="16"
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="1.6"
                          aria-hidden="true"
                        >
                          <path d="M12 5v14M5 12h14" />
                        </svg>
                      ),
                      run: () => setAddingSession(true),
                    },
                    {
                      label: projectCollapse
                        ? t("Disable project collapse")
                        : t("Enable project collapse"),
                      icon: (
                        <svg
                          width="16"
                          height="16"
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="1.6"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          aria-hidden="true"
                        >
                          <path
                            d={
                              projectCollapse
                                ? "m7 9 5-5 5 5M7 15l5 5 5-5"
                                : "m7 4 5 5 5-5M7 20l5-5 5 5"
                            }
                          />
                        </svg>
                      ),
                      run: () => {
                        try {
                          settings.set(
                            "session.panel.projectCollapse",
                            !projectCollapse,
                          );
                          setPanelError("");
                        } catch (error) {
                          setPanelError(String(error));
                        }
                      },
                    },
                    {
                      label: hideStopped
                        ? "Show stopped sessions"
                        : "Hide stopped sessions",
                      icon: (
                        <svg
                          width="16"
                          height="16"
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="1.6"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          aria-hidden="true"
                        >
                          {hideStopped ? (
                            <>
                              <path d="M3 12s3.5-7 9-7 9 7 9 7-3.5 7-9 7-9-7-9-7Z" />
                              <circle cx="12" cy="12" r="3" />
                            </>
                          ) : (
                            <path d="M3 3l18 18M10.6 10.6a2 2 0 0 0 2.8 2.8M9.9 5.2A12 12 0 0 1 12 5c5.5 0 9 7 9 7a18 18 0 0 1-3.2 4.1M6.2 6.2A18 18 0 0 0 3 12s3.5 7 9 7a10 10 0 0 0 4.1-.9" />
                          )}
                        </svg>
                      ),
                      run: () => {
                        try {
                          settings.set(
                            "session.panel.hideStopped",
                            !hideStopped,
                          );
                          setPanelError("");
                        } catch (error) {
                          setPanelError(String(error));
                        }
                      },
                    },
                  ]}
                />
              )
            }
            aria-label={
              resource === "sessions"
                ? t("Session list")
                : resource === "settings"
                  ? t("Settings navigation")
                  : `${resource} navigation`
            }
          >
            {resource === "settings" ? (
              <nav
                className="settings-topics"
                aria-label={t("Settings topics")}
              >
                {(
                  [
                    ["general", "/setting/general", t("General")],
                    ["editor", "/setting/editor", t("Editor")],
                    ["connections", "/setting/connection", t("Connections")],
                    ["logs", "/setting/logs", t("Logs")],
                  ] as const
                ).map(([topic, to, label]) => (
                  <PanelLink
                    key={topic}
                    to={to}
                    aria-current={settingsTopic === topic ? "page" : undefined}
                    aria-controls="settings-editor"
                    onClick={() => {
                      setSettingsFileOpen(false);
                    }}
                  >
                    {label}
                  </PanelLink>
                ))}
              </nav>
            ) : resource === "sessions" ? (
              <div
                className="session-tree"
                aria-label={t("Projects and sessions")}
              >
                {projects
                  .filter(
                    (p) =>
                      p.status?.containerState !== ContainerState.ABSENT &&
                      p.status?.containerState !== ContainerState.STOPPED &&
                      sessionsByProject.has(key(p.id)),
                  )
                  .map((p) => (
                    <SessionTreeGroup
                      key={projectId(p)}
                      project={p}
                      items={sessionsByProject.get(key(p.id)) ?? []}
                      loading={inventory.loading.sessions}
                      selected={session}
                      unread={unread}
                      errors={errors}
                      background={background}
                      open={!collapsed.has(projectId(p))}
                      collapsible={projectCollapse}
                      toggle={() =>
                        setCollapsed((old) => {
                          const next = new Set(old);
                          if (next.has(projectId(p))) next.delete(projectId(p));
                          else next.add(projectId(p));
                          return next;
                        })
                      }
                    />
                  ))}
                {inventory.loading.sessions && !sessionsByProject.size && (
                  <small className="tree-message">{t("Loading…")}</small>
                )}
              </div>
            ) : (
              <nav className="settings-topics" aria-label="Resource navigation">
                <PanelLink
                  {...(resource === "memory"
                    ? memoryProject
                      ? memoryLocation(memoryProject)
                      : projectLocation()
                    : resourceLocation(resource))}
                  aria-current="page"
                >
                  {resource === "memory"
                    ? "Shared memory"
                    : resource === "auxiliary"
                      ? "Auxiliary AI"
                      : resource[0].toUpperCase() + resource.slice(1)}
                </PanelLink>
              </nav>
            )}
            {!!(inventory.errors.projects || inventory.errors.sessions) && (
              <ErrorNotice
                error={String(
                  inventory.errors.projects || inventory.errors.sessions,
                )}
              />
            )}
            {resource === "sessions" && panelError && (
              <ErrorNotice error={panelError} />
            )}
            {inventory.loading.projects && !projects.length && (
              <p className="muted">{t("Loading projects…")}</p>
            )}
          </ResourcePanel>
        )}
        {children}
        {addingSession && (
          <SessionCreate
            c={c}
            projects={projects}
            centered
            close={() => setAddingSession(false)}
          />
        )}
      </div>
    </WorkspaceContext.Provider>
  );
}
