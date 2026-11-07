import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("sidebar highlights only the current resource on project-scoped pages", async ({
  page,
  baseURL,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: new URL(baseURL!).origin },
    data: { token: "a".repeat(32) },
  });
  const sidebar = page.getByRole("navigation", {
    name: "Resources",
    exact: true,
  });
  for (const [path, label] of [
    ["/c/_/project/demo/task", "Tasks"],
    ["/c/_/project/demo/task/example", "Tasks"],
    ["/c/_/project/demo/memory", "Memory"],
    ["/c/_/project/demo/memory/example", "Memory"],
    ["/c/_/project/demo", "Projects view"],
    ["/c/_/project", "Projects view"],
  ]) {
    await page.goto(path);
    const current = sidebar.getByRole("link", { name: label, exact: true });
    await expect(current).toHaveClass(/\bactive\b/);
    await expect(sidebar.locator("a.active")).toHaveCount(1);
    await expect(sidebar.locator('a[aria-current="page"]')).toHaveCount(1);
    if (label !== "Projects view") {
      await expect(
        sidebar.getByRole("link", { name: "Projects view", exact: true }),
      ).not.toHaveClass(/\bactive\b/);
    }
  }
});
