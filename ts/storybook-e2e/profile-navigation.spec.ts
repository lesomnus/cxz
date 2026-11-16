import { expect, test } from "@playwright/test";

test("Logs and Settings are reachable from Profile without a separate Logs sidebar button", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-sessions--default&viewMode=story");
  const sidebar = page.getByRole("navigation", {
    name: "Resources",
    exact: true,
  });
  await expect(
    sidebar.getByRole("link", { name: "Logs", exact: true }),
  ).toHaveCount(0);
  await expect(
    sidebar.getByRole("link", { name: "Projects view", exact: true }),
  ).toBeVisible();
  await expect(
    sidebar.getByRole("link", { name: "Tasks", exact: true }),
  ).toBeVisible();
  expect(
    await sidebar
      .getByRole("link")
      .evaluateAll((links) =>
        links.map((link) => link.getAttribute("aria-label")),
      ),
  ).toEqual(["Sessions view", "Search", "Tasks", "Memory", "Projects view"]);
  await expect(
    sidebar
      .locator(".sidebar-footer")
      .getByRole("link", { name: "Projects view", exact: true }),
  ).toBeVisible();
  const profile = sidebar.getByRole("button", { name: "Profile", exact: true });
  const menu = page.getByRole("menu", { name: "Profile", exact: true });
  await profile.click();
  await expect(menu.getByRole("menuitem")).toHaveText([
    "Accounts",
    "Settings",
    "Logs",
    "Sign out",
  ]);
  await menu.getByRole("menuitem", { name: "Logs", exact: true }).click();
  await expect(
    page.getByRole("searchbox", { name: "Search logs", exact: true }),
  ).toBeVisible();
  await expect(menu).toHaveCount(0);
  await profile.click();
  await menu.getByRole("menuitem", { name: "Settings", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "General", exact: true }),
  ).toBeVisible();
});
