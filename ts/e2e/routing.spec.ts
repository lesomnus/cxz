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
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
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
  await expect(page.locator(".project-row")).toBeVisible();
  await page.locator(".project-row").click();
  await expect(page).toHaveURL(/\/sessions$/);
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
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Connect", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(/\/sessions\/session$/);
});
