import { createMemoryHistory } from "@tanstack/react-router";
import { expect, test } from "vitest";
import { createWorkspaceRouter } from "./router";

test("the entry route chooses the named default connection", async () => {
  const history = createMemoryHistory({ initialEntries: ["/"] });
  const router = createWorkspaceRouter(
    {
      shell: () => null,
      view: () => null,
      loadConnections: async () => [
        { alias: "local", handle: "@local", baseUrl: "", primary: false },
        { alias: "west", handle: "@west", baseUrl: "", primary: true },
      ],
    },
    history,
  );
  try {
    await router.load();
    const result = router._serverResult;
    expect(result?.type).toBe("redirect");
    if (result?.type === "redirect")
      expect(result.redirect.headers.get("Location")).toBe("/c/@west/session");
  } finally {
    history.destroy();
  }
});

test.each(["/log", "/setting/logs"])(
  "Logs are a standalone page: %s",
  async (path) => {
    const history = createMemoryHistory({ initialEntries: [path] });
    const router = createWorkspaceRouter(
      { shell: () => null, view: () => null },
      history,
    );
    try {
      await router.load();
      if (path === "/setting/logs") {
        // Server loads return a redirect response instead of navigating history.
        const result = router._serverResult;
        expect(result?.type).toBe("redirect");
        if (result?.type === "redirect")
          expect(result.redirect.headers.get("Location")).toBe("/log");
      } else {
        expect(router.state.location.pathname).toBe("/log");
        expect(router.state.matches.at(-1)?.staticData.resource).toBe("logs");
      }
    } finally {
      history.destroy();
    }
  },
);

test.each(["/c/@local/activity", "/activity"])(
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
test.each(["/auxiliary", "/c/@local/auxiliary", "/settings/auxiliary"])(
  "old Auxiliary AI addresses redirect into Settings: %s",
  async (path) => {
    const history = createMemoryHistory({ initialEntries: [path] });
    const router = createWorkspaceRouter(
      { shell: () => null, view: () => null },
      history,
    );
    try {
      await router.load();
      const result = router._serverResult;
      expect(result?.type).toBe("redirect");
      if (result?.type === "redirect")
        expect(result.redirect.headers.get("Location")).toBe(
          "/setting/auxiliary",
        );
    } finally {
      history.destroy();
    }
  },
);
test("Auxiliary AI is a settings topic", async () => {
  const history = createMemoryHistory({
    initialEntries: ["/setting/auxiliary"],
  });
  const router = createWorkspaceRouter(
    { shell: () => null, view: () => null },
    history,
  );
  try {
    await router.load();
    expect(router.state.matches.at(-1)?.staticData).toMatchObject({
      resource: "settings",
      settingsTopic: "auxiliary",
    });
  } finally {
    history.destroy();
  }
});
const memory = "00000000-0000-8000-8007-000000000200";
test.each([
  ["/c/@local/errors", "errors", {}],
  [`/c/@local/project/${project}/memory`, "memory", { projectId: project }],
  [
    `/c/@local/project/${project}/memory/${memory}`,
    "memory",
    { projectId: project, memoryId: memory },
  ],
  [`/c/@local/project/${project}`, "projects", { projectId: project }],
  ["/c/@local/project/oak/memory", "memory", { projectId: "oak" }],
  [
    `/c/@local/project/oak/memory/${memory}`,
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
  "/c/@local/memory",
  `/c/@local/memory?project=${project}`,
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
  ["/c/@local/project/cxz/task", { projectId: "cxz" }],
  [
    "/c/@local/project/cxz/task/fix-login",
    { projectId: "cxz", taskId: "fix-login" },
  ],
  [
    `/c/@local/project/${project}/task/${memory}`,
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

test("Errors accepts old bookmarks without filtering the unified list", async () => {
  const history = createMemoryHistory({
    initialEntries: ["/c/@local/errors?tab=history"],
  });
  const router = createWorkspaceRouter(
    { shell: () => null, view: () => null },
    history,
  );
  try {
    await router.load();
    expect(router.state.matches.at(-1)?.staticData.resource).toBe("errors");
    expect(router.state.location.href).toBe("/c/@local/errors?tab=history");
  } finally {
    history.destroy();
  }
});
