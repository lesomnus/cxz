import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  redirect,
  useRouterState,
  type RouterHistory,
} from "@tanstack/react-router";
import type { ComponentType, PropsWithChildren } from "react";
import {
  resourceLocation,
  sessionLocation,
  projectLocation,
  accountLocation,
  taskLocation,
  parseRouteSearch,
  stringifyRouteSearch,
} from "#src/shared/navigation/resource-route.ts";

export type ResourceView =
  | "sessions"
  | "projects"
  | "tasks"
  | "settings"
  | "accounts"
  | "search"
  | "memory"
  | "activity"
  | "auxiliary"
  | "not-found";
export type SettingsTopic = "general" | "editor" | "connections" | "logs";
type WorkspaceRouterContext = {
  shell: ComponentType<PropsWithChildren>;
  view: ComponentType;
};
declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    resource?: ResourceView;
    settingsTopic?: SettingsTopic;
  }
  interface Register {
    router: ReturnType<typeof createWorkspaceRouter>;
  }
}
const rootRoute = createRootRouteWithContext<WorkspaceRouterContext>()({
  component: RouterShell,
});
function RouterShell() {
  const { shell: Shell } = rootRoute.useRouteContext();
  return (
    <Shell>
      <Outlet />
    </Shell>
  );
}
function RouteView() {
  const { view: View } = rootRoute.useRouteContext();
  return <View />;
}
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ ...resourceLocation("sessions"), replace: true });
  },
});
const connectionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "c/$connection",
  component: RouteView,
});
const sessionListRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "session",
  staticData: { resource: "sessions" },
});
const sessionRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "session/$sessionId",
  staticData: { resource: "sessions" },
  validateSearch: (s: Record<string, unknown>): { seq?: string } => ({
    seq:
      typeof s.seq === "string" && /^[1-9]\d*$/.test(s.seq) ? s.seq : undefined,
  }),
});
const projectsRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "project",
  staticData: { resource: "projects" },
});
const projectRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "project/$projectId",
  staticData: { resource: "projects" },
});
const accountsRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "account",
  staticData: { resource: "accounts" },
});
const accountRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "account/$accountId",
  staticData: { resource: "accounts" },
});
const taskSearch = (
  s: Record<string, unknown>,
): { project?: string; assignee?: string; run?: string } => {
  const value = (key: string) =>
    typeof s[key] === "string" && (s[key] as string).length <= 256
      ? (s[key] as string)
      : undefined;
  return {
    project: value("project"),
    assignee: value("assignee"),
    run: value("run"),
  };
};
const tasksRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "task",
  staticData: { resource: "tasks" },
  validateSearch: taskSearch,
});
const taskRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "task/$taskId",
  staticData: { resource: "tasks" },
  validateSearch: taskSearch,
});
const searchRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "search",
  staticData: { resource: "search" },
});
const memoryRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "memory",
  staticData: { resource: "memory" },
  validateSearch: (s: Record<string, unknown>) => ({
    project: typeof s.project === "string" ? s.project : undefined,
  }),
});
const activityRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "activity",
  staticData: { resource: "activity" },
});
const auxiliaryRoute = createRoute({
  getParentRoute: () => connectionRoute,
  path: "auxiliary",
  staticData: { resource: "auxiliary" },
});
const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "setting",
  component: RouteView,
});
const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/setting/general", replace: true });
  },
});
const generalRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "general",
  staticData: { resource: "settings", settingsTopic: "general" },
});
const editorRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "editor",
  staticData: { resource: "settings", settingsTopic: "editor" },
});
const connectionsRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "connection",
  staticData: { resource: "settings", settingsTopic: "connections" },
});
const logsRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "logs",
  staticData: { resource: "settings", settingsTopic: "logs" },
});
const notFoundRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$",
  component: RouteView,
  staticData: { resource: "not-found" },
  beforeLoad: ({ location }) => {
    const parts = location.pathname.split("/").filter(Boolean);
    const query = location.search as Record<string, unknown>;
    let handle: string | undefined;
    try {
      handle = parts[1] && decodeURIComponent(parts[1]);
    } catch {
      return;
    }
    if (parts.length > 2) return;
    if (parts[0] === "sessions")
      throw redirect({
        ...(handle ? sessionLocation(handle) : resourceLocation("sessions")),
        search: { seq: typeof query.seq === "string" ? query.seq : undefined },
        replace: true,
      });
    if (parts[0] === "projects" && !handle)
      throw redirect({
        ...projectLocation(
          typeof query.project === "string" ? query.project : undefined,
        ),
        replace: true,
      });
    if (parts[0] === "accounts" && !handle)
      throw redirect({
        ...accountLocation(
          typeof query.account === "string" ? query.account : undefined,
        ),
        replace: true,
      });
    if (parts[0] === "tasks" && !handle)
      throw redirect({
        ...taskLocation({
          ...taskSearch(query),
          task: typeof query.task === "string" ? query.task : undefined,
        }),
        replace: true,
      });
    if (
      ["search", "memory", "activity", "auxiliary"].includes(parts[0]) &&
      !handle
    )
      throw redirect({
        ...resourceLocation(
          parts[0] as "search" | "memory" | "activity" | "auxiliary",
        ),
        replace: true,
      });
    if (
      parts[0] === "settings" &&
      (!handle || ["general", "editor", "connections", "logs"].includes(handle))
    ) {
      const to =
        handle === "logs"
          ? "/setting/logs"
          : handle === "editor"
          ? "/setting/editor"
          : handle === "connections"
            ? "/setting/connection"
            : "/setting/general";
      throw redirect({ to, replace: true });
    }
  },
});
const routeTree = rootRoute.addChildren([
  indexRoute,
  connectionRoute.addChildren([
    sessionListRoute,
    sessionRoute,
    projectsRoute,
    projectRoute,
    accountsRoute,
    accountRoute,
    tasksRoute,
    taskRoute,
    searchRoute,
    memoryRoute,
    activityRoute,
    auxiliaryRoute,
  ]),
  settingsRoute.addChildren([
    settingsIndexRoute,
    generalRoute,
    editorRoute,
    connectionsRoute,
    logsRoute,
  ]),
  notFoundRoute,
]);
export function createWorkspaceRouter(
  context: WorkspaceRouterContext,
  history?: RouterHistory,
) {
  return createRouter({
    routeTree,
    context,
    history,
    parseSearch: parseRouteSearch,
    stringifySearch: stringifyRouteSearch,
    defaultPreload: false,
    trailingSlash: "never",
    scrollRestoration: false,
  });
}
export function useWorkspaceRoute() {
  const match = useRouterState({ select: (state) => state.matches.at(-1) });
  const detailTaskSearch = taskRoute.useSearch({ shouldThrow: false });
  const listTaskSearch = tasksRoute.useSearch({ shouldThrow: false });
  const connection = connectionRoute.useParams({
    shouldThrow: false,
  })?.connection;
  const resource =
    connection && connection !== "_"
      ? "not-found"
      : (match?.staticData.resource ?? "sessions");
  return {
    resource,
    connection,
    settingsTopic: match?.staticData.settingsTopic ?? "general",
    session: sessionRoute.useParams({ shouldThrow: false })?.sessionId ?? "",
    seq: sessionRoute.useSearch({ shouldThrow: false })?.seq,
    account: accountRoute.useParams({ shouldThrow: false })?.accountId ?? "",
    project: projectRoute.useParams({ shouldThrow: false })?.projectId,
    tasks: {
      ...(detailTaskSearch ?? listTaskSearch ?? {}),
      task: taskRoute.useParams({ shouldThrow: false })?.taskId,
    },
    memoryProject: memoryRoute.useSearch({ shouldThrow: false })?.project,
  };
}
