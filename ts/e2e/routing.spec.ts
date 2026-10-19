import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});
async function signIn(page: import("@playwright/test").Page) {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
}

test("direct routes, reload, browser history and new tabs preserve workspace navigation", async ({
  page,
  context,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await signIn(page);
  const response = await page.goto("/sessions/session");
  expect(response!.status()).toBe(200);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible();
  await input.fill("Draft across routes");
  const session = page.getByRole("link", { name: /demo-chat/ });
  await expect(session).toHaveAttribute("href", "/sessions/session");
  await expect(session).toHaveClass(/active/);
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page.getByRole("menuitem", { name: "Settings", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/general$/);
  await page.getByRole("link", { name: "Editor", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/editor$/);
  await page.goBack();
  await expect(
    page.getByRole("heading", { name: "General", exact: true }),
  ).toBeVisible();
  await page.goBack();
  await expect(input).toHaveValue("Draft across routes");
  await page.goForward();
  await expect(page).toHaveURL(/\/settings\/general$/);
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(input).toHaveValue("Draft across routes");
  await page.reload();
  await expect(page).toHaveURL(/\/sessions\/session$/);
  await expect(input).toBeVisible();
  const popupPromise = context.waitForEvent("page");
  await session.click({ modifiers: ["Control"] });
  const popup = await popupPromise;
  await popup.waitForLoadState();
  await expect(popup).toHaveURL(/\/sessions\/session$/);
  await expect(
    popup.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  await popup.close();
  await page.getByRole("link", { name: "Projects view", exact: true }).click();
  await expect(page).toHaveURL(/\/projects$/);
  // Selecting a project session keeps the dashboard open; only its explicit
  // conversation link navigates to the session.
  const workspace = page.getByRole("main", { name: "Project workspace" });
  await expect(workspace).toBeVisible();
  await expect(
    workspace.getByRole("combobox", { name: "Select project", exact: true }),
  ).toBeVisible();
  await workspace
    .getByRole("region", { name: "Session list", exact: true })
    .getByRole("button")
    .first()
    .click();
  await expect(page).toHaveURL(/\/projects$/);
  await workspace
    .getByRole("region", { name: "Session details", exact: true })
    .getByRole("link", { name: "Open conversation", exact: true })
    .click();
  await expect(page).toHaveURL(/\/sessions\/session$/);
  expect(errors).toEqual([]);
});

test("a deep link survives sign-in, redirects and unknown routes stay navigable", async ({
  page,
}) => {
  await page.goto("/settings/editor");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/editor$/);
  await expect(
    page.getByRole("heading", { name: "Editor", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Editor", exact: true }),
  ).toBeVisible();
  await page.goto("/settings");
  await expect(page).toHaveURL(/\/settings\/general$/);
  await page.goto("/");
  await expect(page).toHaveURL(/\/sessions$/);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/unknown/page");
  await expect(
    page.getByRole("heading", { name: "Page not found" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Back to sessions" }).click();
  await expect(page).toHaveURL(/\/sessions$/);
  expect((await page.request.get("/assets/missing.js")).status()).toBe(404);
  expect((await page.request.get("/auth/missing")).status()).toBe(404);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/sessions/session");
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page.getByRole("menuitem", { name: "Sign out", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Connect", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(/\/sessions\/session$/);
});

test("profile sign out failures stay visible and can be retried", async ({
  page,
}) => {
  await signIn(page);
  await page.goto("/accounts");
  await page.route("**/auth/logout", (route) => route.fulfill({ status: 500 }));
  const profile = page.getByRole("button", { name: "Profile", exact: true });
  await profile.click();
  await page.getByRole("menuitem", { name: "Sign out", exact: true }).click();
  await expect(page.locator(".profile-error")).toContainText("Sign out failed");
  await page.unroute("**/auth/logout");
  await profile.click();
  await page.getByRole("menuitem", { name: "Sign out", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Connect", exact: true }),
  ).toBeVisible();
});

// A browser attached straight to one gateway has exactly one connection, and it
// is the one that served the page: there is nothing to pick and nothing to
// forget. See docs/plans/web-connections.md.
test("a direct connection is listed once, read-only, and reaches this installation", async ({
  page,
}) => {
  // Count what listing costs, because the row rendering correctly is not the
  // same as the page having finished: the connections tab used to rebuild its
  // client on every render and re-list on every answer, so it rendered exactly
  // this row -- and passed this test -- while spinning the tab into a freeze.
  // Every list reads this key. See cxz#230.
  await page.addInitScript(() => {
    const w = window as unknown as { __reads: number };
    w.__reads = 0;
    const read = Storage.prototype.getItem;
    Storage.prototype.getItem = function (key: string) {
      if (key === "cxz.connections") w.__reads++;
      return read.call(this, key);
    };
  });
  await signIn(page);
  await page.goto("/settings/connections");
  await expect(
    page.getByRole("heading", { name: "Connections", exact: true }),
  ).toBeVisible();
  const reads = () =>
    page.evaluate(() => (window as unknown as { __reads: number }).__reads);
  const rows = page.locator(".connection-list tbody tr");
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute("data-read-only", "true");
  await expect(rows.first()).toContainText("default");
  await expect(rows.first()).toContainText("served this page");
  await expect(rows.first()).toContainText(new URL(page.url()).origin);
  // Nothing in the page offers to remove or edit it.
  await expect(
    page.getByRole("button", { name: /forget|remove|delete/i }),
  ).toHaveCount(0);
  // An open tab that has answered is done asking. A page that is looping reads
  // this thousands of times in half a second, so the assertion is that the
  // second sample equals the first rather than any particular budget.
  const settled = await reads();
  await page.waitForTimeout(500);
  expect(await reads()).toBe(settled);
});
