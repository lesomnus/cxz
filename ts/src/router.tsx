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

export type ResourceView = "sessions" | "projects" | "settings" | "not-found";
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
});
const projectsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "projects",
  component: RouteView,
  staticData: { resource: "projects" },
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
  };
}
