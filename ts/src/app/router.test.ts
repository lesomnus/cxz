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
