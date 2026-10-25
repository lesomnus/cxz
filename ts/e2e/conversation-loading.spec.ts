import { expect, test } from "@playwright/test";

function gate() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}
async function signIn(page: import("@playwright/test").Page) {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  // End the stream to exercise reconnects after the initial load too.
  await page.route("**/cxz.SessionService/Events", (route) =>
    route.fulfill({
      contentType: "application/connect+json",
      body: Buffer.from([2, 0, 0, 0, 2, 123, 125]),
    }),
  );
}

for (const empty of [false, true]) {
  test(`initial conversation loading stages finish for ${empty ? "empty" : "populated"} history and stay finished on reconnect`, async ({
    page,
  }) => {
    const session = gate(),
      history = gate();
    try {
      await signIn(page);
      let streamRequests = 0;
      page.on("request", (request) => {
        if (request.url().endsWith("/cxz.SessionService/Events"))
          streamRequests++;
      });
      await page.route("**/cxz.SessionService/History", (route) =>
        route.fulfill({ json: {} }),
      );
      await page.route("**/cxz.SessionService/Get", async (route) => {
        await session.promise;
        await route.continue();
      });
      await page.route("**/cxz.SessionService/Transcript", async (route) => {
        await history.promise;
        await route.fulfill({
          json: {
            events: empty
              ? []
              : [{ seq: "1", kind: "assistant", text: "Loaded response" }],
            snapshotSeq: "4",
          },
        });
      });
      await page.goto("/c/_/session/session");
      const pane = page.locator(".transcript");
      await expect(
        page.getByRole("status").filter({ hasText: "Loading session…" }),
      ).toBeVisible();
      await expect(pane).toHaveAttribute("aria-busy", "true");
      session.release();
      await expect(
        page.getByRole("status").filter({ hasText: "Loading conversation…" }),
      ).toBeVisible();
      history.release();
      await expect(pane).toHaveAttribute("aria-busy", "false");
      await expect(
        page
          .getByRole("status")
          .filter({ hasText: /Loading (session|conversation)/ }),
      ).toHaveCount(0);
      await expect(
        pane.getByText("Loaded response", { exact: true }),
      ).toHaveCount(empty ? 0 : 1);
      await expect.poll(() => streamRequests).toBeGreaterThan(1);
      await expect(pane).toHaveAttribute("aria-busy", "false");
    } finally {
      session.release();
      history.release();
      await page.unrouteAll({ behavior: "wait" });
    }
  });
}

test("initial load failure is visible until an automatic retry succeeds", async ({
  page,
}) => {
  const retry = gate();
  try {
    await signIn(page);
    let requests = 0;
    await page.route("**/cxz.SessionService/Transcript", async (route) => {
      if (++requests === 1)
        return route.fulfill({
          status: 503,
          json: { code: "unavailable", message: "Temporarily offline" },
        });
      await retry.promise;
      await route.fulfill({ json: { snapshotSeq: "4" } });
    });
    await page.goto("/c/_/session/session");
    const status = page
      .getByRole("status")
      .filter({ hasText: "Retrying conversation…" });
    await expect(status).toBeVisible();
    await expect(status).toContainText("Temporarily offline");
    await expect.poll(() => requests).toBeGreaterThan(1);
    await expect(
      page.getByRole("status").filter({ hasText: "Loading conversation…" }),
    ).toBeVisible();
    retry.release();
    await expect(page.locator(".transcript")).toHaveAttribute(
      "aria-busy",
      "false",
    );
    await expect(status).toHaveCount(0);
  } finally {
    retry.release();
    await page.unrouteAll({ behavior: "wait" });
  }
});

for (const resolved of [true, false]) {
  test(`initial catch-up ${resolved ? "never flashes resolved" : "preserves unresolved"} approvals from a stale catalog`, async ({
    page,
  }) => {
    await signIn(page);
    const request = {
      seq: "1",
      runId: "run",
      kind: "approval",
      text: "Bash",
      requestId: "permission",
    };
    await page.addInitScript(() => {
      (window as any).approvalFlashed = false;
      new MutationObserver(() => {
        if (document.querySelector(".question-cards .approval"))
          (window as any).approvalFlashed = true;
      }).observe(document, { childList: true, subtree: true });
    });
    await page.route("**/cxz.SessionService/Get", async (route) => {
      const response = await route.fetch();
      const json = await response.json();
      json.status = {
        runId: "run",
        state: "waiting_input",
        lastSeq: "1",
        pending: [request],
      };
      await route.fulfill({ response, json });
    });
    await page.route("**/cxz.SessionService/Transcript", (route) =>
      route.fulfill({
        json: { snapshotSeq: "1", events: [request] },
      }),
    );
    await page.route("**/cxz.SessionService/History", async (route) => {
      const after = Number(route.request().postDataJSON().afterSeq ?? 0);
      if (after >= 3) return route.fulfill({ json: {} });
      // Resolving history arrives well after the automatic-approval UI grace.
      await new Promise((resolve) => setTimeout(resolve, 350));
      await route.fulfill({
        json: {
          events: [
            ...(resolved
              ? [
                  {
                    seq: "2",
                    runId: "run",
                    requestId: "permission",
                    kind: "approval_resolved",
                    text: "allowed",
                  },
                ]
              : []),
            {
              seq: "3",
              runId: "run",
              kind: "state",
              text: resolved ? "idle" : "waiting_input",
            },
          ],
        },
      });
    });
    await page.goto("/c/_/session/session");
    await expect(page.locator(".transcript")).toHaveAttribute(
      "aria-busy",
      "false",
    );
    if (resolved) {
      await page.waitForTimeout(250);
      await expect(
        page.getByRole("button", { name: "Allow", exact: true }),
      ).toHaveCount(0);
      expect(await page.evaluate(() => (window as any).approvalFlashed)).toBe(
        false,
      );
    } else {
      await expect(
        page.getByRole("button", { name: "Allow", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Deny", exact: true }),
      ).toBeVisible();
    }
  });
}
