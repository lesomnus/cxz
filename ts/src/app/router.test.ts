import { createMemoryHistory } from "@tanstack/react-router";
import { expect, test } from "vitest";
import { createWorkspaceRouter } from "./router";

test.each(["/c/_/activity", "/activity"])(
  "a removed Activity bookmark shows the not-found page: %s",
  async (path) => {
    const history = createMemoryHistory({ initialEntries: [path] });
    const router = createWorkspaceRouter(
      { shell: () => null, view: () => null },
      history,
    );
    try {
      await router.load();
      expect(router.state.matches.at(-1)?.staticData.resource).toBe(
        "not-found",
      );
      expect(router.state.location.pathname).toBe(path);
    } finally {
      history.destroy();
    }
  },
);

const project = "00000000-0000-8000-8007-000000000001";
const memory = "00000000-0000-8000-8007-000000000200";
test.each([
  ["/c/_/errors", "errors", {}],
  [`/c/_/project/${project}/memory`, "memory", { projectId: project }],
  [
    `/c/_/project/${project}/memory/${memory}`,
    "memory",
    { projectId: project, memoryId: memory },
  ],
  [`/c/_/project/${project}`, "projects", { projectId: project }],
  ["/c/_/project/oak/memory", "memory", { projectId: "oak" }],
  [
    `/c/_/project/oak/memory/${memory}`,
    "memory",
    { projectId: "oak", memoryId: memory },
  ],
])(
  "project and Memory routes remain distinct: %s",
  async (path, resource, params) => {
    const history = createMemoryHistory({ initialEntries: [path] });
    const router = createWorkspaceRouter(
      { shell: () => null, view: () => null },
      history,
    );
    try {
      await router.load();
      expect(router.state.matches.at(-1)?.staticData.resource).toBe(resource);
      expect(router.state.matches.at(-1)?.params).toMatchObject(params);
      expect(router.state.location.pathname).toBe(path);
    } finally {
      history.destroy();
    }
  },
);
test.each([
  "/c/_/memory",
  `/c/_/memory?project=${project}`,
  `/memory?project=${project}`,
])("removed Memory addresses are not redirected: %s", async (path) => {
  const history = createMemoryHistory({ initialEntries: [path] });
  const router = createWorkspaceRouter(
    { shell: () => null, view: () => null },
    history,
  );
  try {
    await router.load();
    expect(router.state.location.href).toBe(path);
    expect(router.state.matches.at(-1)?.staticData.resource).toBe("not-found");
  } finally {
    history.destroy();
  }
});

test.each([
  ["/c/_/project/cxz/task", { projectId: "cxz" }],
  [
    "/c/_/project/cxz/task/fix-login",
    { projectId: "cxz", taskId: "fix-login" },
  ],
  [
    `/c/_/project/${project}/task/${memory}`,
    { projectId: project, taskId: memory },
  ],
])(
  "Task addresses keep project and task references in the path: %s",
  async (path, params) => {
    const history = createMemoryHistory({ initialEntries: [path] });
    const router = createWorkspaceRouter(
      { shell: () => null, view: () => null },
      history,
    );
    try {
      await router.load();
      expect(router.state.matches.at(-1)?.staticData.resource).toBe("tasks");
      expect(router.state.matches.at(-1)?.params).toMatchObject(params);
      expect(router.state.location.pathname).toBe(path);
    } finally {
      history.destroy();
    }
  },
);

test("Errors history is a direct bookmark with validated tab state", async () => {
  const history = createMemoryHistory({
    initialEntries: ["/c/_/errors?tab=history"],
  });
  const router = createWorkspaceRouter(
    { shell: () => null, view: () => null },
    history,
  );
  try {
    await router.load();
    expect(router.state.matches.at(-1)?.staticData.resource).toBe("errors");
    expect(router.state.location.search).toMatchObject({ tab: "history" });
    expect(router.state.location.href).toBe("/c/_/errors?tab=history");
  } finally {
    history.destroy();
  }
});
