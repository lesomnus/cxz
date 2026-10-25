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
  const response = await page.goto("/c/_/session/demo-chat");
  expect(response!.status()).toBe(200);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible();
  await input.fill("Draft across routes");
  const session = page.getByRole("link", { name: /demo-chat/ });
  await expect(session).toHaveAttribute("href", "/c/_/session/demo-chat");
  await expect(session).toHaveClass(/active/);
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page.getByRole("menuitem", { name: "Settings", exact: true }).click();
  await expect(page).toHaveURL(/\/setting\/general$/);
  await page.getByRole("link", { name: "Editor", exact: true }).click();
  await expect(page).toHaveURL(/\/setting\/editor$/);
  await page.goBack();
  await expect(
    page.getByRole("heading", { name: "General", exact: true }),
  ).toBeVisible();
  await page.goBack();
  await expect(input).toHaveValue("Draft across routes");
  await page.goForward();
  await expect(page).toHaveURL(/\/setting\/general$/);
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(input).toHaveValue("Draft across routes");
  await page.reload();
  await expect(page).toHaveURL(/\/c\/_\/session\/demo-chat$/);
  await expect(input).toBeVisible();
  const popupPromise = context.waitForEvent("page");
  await session.click({ modifiers: ["Control"] });
  const popup = await popupPromise;
  await popup.waitForLoadState();
  await expect(popup).toHaveURL(/\/c\/_\/session\/demo-chat$/);
  await expect(
    popup.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  await popup.close();
  await page.getByRole("link", { name: "Projects view", exact: true }).click();
  await expect(page).toHaveURL(/\/c\/_\/project$/);
  // Row selection keeps the project dashboard open.
  const workspace = page.getByRole("main", { name: "Project workspace" });
  await expect(workspace).toBeVisible();
  await expect(
    workspace.getByRole("combobox", { name: "Select project", exact: true }),
  ).toBeVisible();
  await workspace
    .getByRole("region", { name: "Session list", exact: true })
    .locator("tbody tr")
    .first()
    .getByRole("checkbox")
    .click();
  await expect(page).toHaveURL(/\/c\/_\/project$/);
  await expect(
    workspace.getByRole("checkbox", {
      name: "Select all sessions",
      exact: true,
    }),
  ).toBeChecked();
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(page).toHaveURL(/\/c\/_\/session\/demo-chat$/);
  expect(errors).toEqual([]);
});

test("a deep link survives sign-in, redirects and unknown routes stay navigable", async ({
  page,
}) => {
  await page.goto("/setting/editor");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(page).toHaveURL(/\/setting\/editor$/);
  await expect(
    page.getByRole("heading", { name: "Editor", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Editor", exact: true }),
  ).toBeVisible();
  await page.goto("/setting");
  await expect(page).toHaveURL(/\/setting\/general$/);
  await page.goto("/");
  await expect(page).toHaveURL(/\/c\/_\/session$/);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/unknown/page");
  await expect(
    page.getByRole("heading", { name: "Page not found" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Back to sessions" }).click();
  await expect(page).toHaveURL(/\/c\/_\/session$/);
  expect((await page.request.get("/assets/missing.js")).status()).toBe(404);
  expect((await page.request.get("/auth/missing")).status()).toBe(404);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/c/_/session/demo-chat");
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page.getByRole("menuitem", { name: "Sign out", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Connect", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(/\/c\/_\/session\/demo-chat$/);
});

test("profile sign out failures stay visible and can be retried", async ({
  page,
}) => {
  await signIn(page);
  await page.goto("/c/_/account");
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
  await page.goto("/setting/connection");
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

test("alias and UUID URLs open the same session and preserve its draft", async ({
  page,
}) => {
  await signIn(page);
  await page.goto("/c/_/session/demo-chat");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Same session, two readable locators");
  // The fixture uses these sixteen bytes as its resource ID.
  const id = "61626364-6566-6768-696a-6b6c6d6e6f70";
  await page.goto(`/c/_/session/${id}`);
  await expect(input).toHaveValue("Same session, two readable locators");
  await expect(page).toHaveURL(new RegExp(`/c/_/session/${id}$`));
  const link = page.getByRole("link", { name: /demo-chat/ });
  await expect(link).toHaveClass(/active/);
  await expect(link).toHaveAttribute("href", "/c/_/session/demo-chat");
  await link.click();
  await expect(input).toHaveValue("Same session, two readable locators");
});

test("legacy bookmarks redirect to singular scoped routes without losing conversation position", async ({
  page,
}) => {
  await signIn(page);
  await page.goto("/sessions/session?seq=2");
  await expect(page).toHaveURL(/\/c\/_\/session\/demo-chat\?seq=2$/);
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  await page.goto("/sessions/session?seq=%222%22");
  await expect(page).toHaveURL(/\/c\/_\/session\/demo-chat\?seq=2$/);
  await page.goto("/projects?project=demo");
  await expect(page).toHaveURL(/\/c\/_\/project\/demo$/);
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Demo project");
  await page.goto("/settings/connections");
  await expect(page).toHaveURL(/\/setting\/connection$/);
  await expect(
    page.getByRole("heading", { name: "Connections", exact: true }),
  ).toBeVisible();
});

test("an unknown connection never opens the local session with the same alias", async ({
  page,
}) => {
  await signIn(page);
  await page.goto("/c/other/session/demo-chat");
  await expect(
    page.getByRole("heading", { name: "Page not found", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("This connection is unavailable.", { exact: false }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toHaveCount(0);
});
