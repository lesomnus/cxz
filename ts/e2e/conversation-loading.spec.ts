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
      await page.goto("/sessions/session");
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
    await page.goto("/sessions/session");
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
