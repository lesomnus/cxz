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
export type SettingsTopic = "general" | "editor";

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
    throw redirect({ to: "/sessions", replace: true });
  },
});
const sessionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "sessions",
  component: RouteView,
});
const sessionListRoute = createRoute({
  getParentRoute: () => sessionsRoute,
  path: "/",
  staticData: { resource: "sessions" },
});
const sessionRoute = createRoute({
  getParentRoute: () => sessionsRoute,
  path: "$sessionId",
  staticData: { resource: "sessions" },
  validateSearch: (search: Record<string, unknown>): { seq?: string } => ({
    seq:
      typeof search.seq === "string" && /^[1-9]\d*$/.test(search.seq)
        ? search.seq
        : undefined,
  }),
});
const projectsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "projects",
  component: RouteView,
  staticData: { resource: "projects" },
  validateSearch: (search: Record<string, unknown>): { project?: string } => ({
    project:
      typeof search.project === "string" && search.project.length <= 256
        ? search.project
        : undefined,
  }),
});
const accountsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "accounts",
  component: RouteView,
  staticData: { resource: "accounts" },
  validateSearch: (search: Record<string, unknown>): { account?: string } => ({
    account:
      typeof search.account === "string" &&
      /^[a-z][a-z0-9-]{0,62}$/.test(search.account)
        ? search.account
        : undefined,
  }),
});
const tasksRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "tasks",
  component: RouteView,
  staticData: { resource: "tasks" },
  validateSearch: (
    search: Record<string, unknown>,
  ): { project?: string; task?: string; assignee?: string; run?: string } => {
    const value = (key: string) =>
      typeof search[key] === "string" && (search[key] as string).length <= 256
        ? (search[key] as string)
        : undefined;
    return {
      project: value("project"),
      task: value("task"),
      assignee: value("assignee"),
      run: value("run"),
    };
  },
});
const searchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "search",
  component: RouteView,
  staticData: { resource: "search" },
});
const memoryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "memory",
  component: RouteView,
  staticData: { resource: "memory" },
});
const activityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "activity",
  component: RouteView,
  staticData: { resource: "activity" },
});
const auxiliaryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "auxiliary",
  component: RouteView,
  staticData: { resource: "auxiliary" },
});
const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "settings",
  component: RouteView,
});
const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/settings/general", replace: true });
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
const notFoundRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$",
  component: RouteView,
  staticData: { resource: "not-found" },
});
const routeTree = rootRoute.addChildren([
  indexRoute,
  sessionsRoute.addChildren([sessionListRoute, sessionRoute]),
  projectsRoute,
  accountsRoute,
  tasksRoute,
  searchRoute,
  memoryRoute,
  activityRoute,
  auxiliaryRoute,
  settingsRoute.addChildren([settingsIndexRoute, generalRoute, editorRoute]),
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
    // SessionHistory owns fetching and transcript scroll restoration.
    defaultPreload: false,
    trailingSlash: "never",
    scrollRestoration: false,
  });
}

export function useWorkspaceRoute() {
  const data = useRouterState({
    select: (state) => state.matches.at(-1)?.staticData,
  });
  const session =
    sessionRoute.useParams({ shouldThrow: false })?.sessionId ?? "";
  return {
    resource: data?.resource ?? "sessions",
    settingsTopic: data?.settingsTopic ?? "general",
    session,
    seq: sessionRoute.useSearch({ shouldThrow: false })?.seq,
    account: accountsRoute.useSearch({ shouldThrow: false })?.account ?? "",
    project: projectsRoute.useSearch({ shouldThrow: false })?.project,
    tasks: tasksRoute.useSearch({ shouldThrow: false }) ?? {},
  };
}
