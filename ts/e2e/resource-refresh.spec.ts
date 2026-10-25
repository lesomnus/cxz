import { Buffer } from "node:buffer";
import { expect, test } from "@playwright/test";
import { projectFixture } from "../test-support/projects";

test.use({
  viewport: { width: 1440, height: 900 },
  isMobile: false,
  hasTouch: false,
});

function frames(rows: unknown[]) {
  return Buffer.concat([
    ...rows.map((row) => {
      const body = Buffer.from(JSON.stringify(row));
      const header = Buffer.alloc(5);
      header.writeUInt32BE(body.length, 1);
      return Buffer.concat([header, body]);
    }),
    Buffer.from([2, 0, 0, 0, 2, 123, 125]),
  ]);
}

for (const view of ["sessions", "projects"] as const) {
  test(`${view} keeps its existing DOM while an unchanged resource snapshot reconnects`, async ({
    page,
  }) => {
    await page.request.post("/auth/login", {
      headers: { Origin: "https://127.0.0.1:18081" },
      data: { token: "a".repeat(32) },
    });
    const id = (value: string) =>
      Buffer.from(value.padEnd(16, "0")).toString("base64");
    const projects = Array.from({ length: 3 }, (_, i) => ({
      id: projectFixture(i + 1).id,
      alias: `project-${i}`,
      name: `Project ${i}`,
      workspace: `/workspaces/project-${i}`,
      status: { containerState: "CONTAINER_STATE_ABSENT" },
    }));
    const sessions = projects.flatMap((project, i) =>
      Array.from({ length: 2 }, (_, j) => ({
        id: id(`session-${i}-${j}`),
        runtimeId: `session-${i}-${j}`,
        alias: `chat-${i}-${j}`,
        name: `Conversation ${i}-${j}`,
        listed: true,
        agent: "codex",
        project,
        status: { state: "stopped" },
      })),
    );
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const reads = { ProjectService: 0, SessionService: 0 };
    const watches = { ProjectService: 0, SessionService: 0 };
    await page.route("**/cxz.*Service/*", async (route) => {
      const match = new URL(route.request().url()).pathname.match(
        /cxz\.(ProjectService|SessionService)\/(List|Watch)$/,
      );
      if (!match) return route.continue();
      const service = match[1] as keyof typeof reads;
      const rows = service === "ProjectService" ? projects : sessions;
      if (match[2] === "List") {
        reads[service]++;
        return route.fulfill({
          json: { items: rows.slice(0, 1), next: "more" },
        });
      }
      if (++watches[service] > 1) await held;
      return route.fulfill({
        contentType: "application/connect+json",
        body: frames([
          {
            snapshotComplete: false,
            items: rows.slice(0, 1).map((value) => ({ id: value.id, value })),
          },
          {
            snapshotComplete: true,
            items: rows.slice(1).map((value) => ({ id: value.id, value })),
          },
        ]),
      });
    });
    try {
      await page.goto(
        view === "projects"
          ? `/projects?project=${projectFixture(3).uuid}`
          : "/sessions",
      );
      const selector =
        view === "projects"
          ? 'section[aria-label="Information"]'
          : ".session-tree .tree-session";
      const content = page.locator(selector);
      await expect(content).toHaveCount(view === "projects" ? 1 : 6);
      if (view === "projects") await expect(content).toContainText("Project 2");
      await page.evaluate((selector) => {
        const nodes = Array.from(document.querySelectorAll(selector));
        const state = { nodes, removed: false };
        (window as unknown as { inventoryProbe: typeof state }).inventoryProbe =
          state;
        const observer = new MutationObserver(() => {
          if (nodes.some((node) => !node.isConnected)) state.removed = true;
        });
        observer.observe(document.body, { childList: true, subtree: true });
      }, selector);
      await expect
        .poll(() => Math.min(reads.ProjectService, reads.SessionService))
        .toBeGreaterThanOrEqual(2);
      await expect
        .poll(() => Math.min(watches.ProjectService, watches.SessionService))
        .toBeGreaterThanOrEqual(2);
      await expect(content).toHaveCount(view === "projects" ? 1 : 6);
      release();
      // A further request means the replacement snapshot was consumed and the
      // stream closed; assert across the complete refresh, not just its probe.
      await expect
        .poll(() => Math.min(reads.ProjectService, reads.SessionService))
        .toBeGreaterThanOrEqual(3);
      expect(
        await page.evaluate(() => {
          const state = (
            window as unknown as {
              inventoryProbe: { nodes: Element[]; removed: boolean };
            }
          ).inventoryProbe;
          return (
            !state.removed && state.nodes.every((node) => node.isConnected)
          );
        }),
      ).toBe(true);
      if (view === "projects") await expect(content).toContainText("Project 2");
    } finally {
      release();
      await page.unrouteAll({ behavior: "wait" });
    }
  });
}
