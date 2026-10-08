import { expect, test } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

test("resource navigation shares catalog subscriptions instead of opening a list/watch per project", async ({
  page,
}) => {
  const calls = {
    projectList: 0,
    projectWatch: 0,
    sessionList: 0,
    sessionWatch: 0,
  };
  const requests = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    const name = {
      "/cxz.ProjectService/List": "projectList",
      "/cxz.ProjectService/Watch": "projectWatch",
      "/cxz.SessionService/List": "sessionList",
      "/cxz.SessionService/Watch": "sessionWatch",
    }[path];
    if (!name) return;
    calls[name]++;
    const body = request.postDataBuffer();
    // Server-streaming Connect requests have an envelope before their JSON.
    const json = JSON.parse(
      (name.endsWith("Watch") ? body.subarray(5) : body).toString(),
    );
    requests.push(json.filters);
  });
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.goto("/");
  await expect(page.locator(".tree-session").first()).toBeVisible();
  await expect
    .poll(() => calls)
    .toEqual({
      projectList: 1,
      projectWatch: 1,
      sessionList: 1,
      sessionWatch: 1,
    });
  await page.locator(".tree-project").first().click();
  await page.locator(".tree-project").first().click();
  await page.getByRole("link", { name: "Projects view", exact: true }).click();
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await page.getByRole("link", { name: /demo-chat/ }).click();
  await expect(page.locator(".composer")).toBeVisible();
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await page.waitForTimeout(300);
  expect(calls).toEqual({
    projectList: 1,
    projectWatch: 1,
    sessionList: 1,
    sessionWatch: 1,
  });
  expect(requests).toEqual(Array.from({ length: 4 }, () => [{ listed: true }]));
});
