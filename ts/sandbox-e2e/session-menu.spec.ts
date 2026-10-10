import { test, expect, devices, type Page } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});
async function ready(page: Page) {
  await page.goto("/sandbox.html#/sessions/session-1");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
}
async function choose(page: Page, name: string) {
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  await page.getByRole("menuitem", { name, exact: true }).click();
}
async function runId(page: Page) {
  await choose(page, "Session details");
  const value = await page
    .locator(".session-details > div")
    .filter({ has: page.getByText("Run ID", { exact: true }) })
    .locator("dd")
    .innerText();
  await page
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  return value;
}
test("composer session menu stays in the viewport, supports keyboard navigation and shows session details", async ({
  page,
}) => {
  await ready(page);
  const trigger = page.getByRole("button", {
    name: "Session menu",
    exact: true,
  });
  const terminal = page.getByRole("button", { name: "Terminal", exact: true });
  expect((await trigger.boundingBox())!.x).toBeLessThan(
    (await terminal.boundingBox())!.x,
  );
  await trigger.focus();
  await trigger.press("ArrowDown");
  const menu = page.getByRole("menu", { name: "Session menu" });
  await expect(
    menu.getByRole("group", { name: "Session controls" }),
  ).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Stop session", exact: true }),
  ).toBeFocused();
  const bounds = (await menu.boundingBox())!;
  expect(bounds.y).toBeGreaterThanOrEqual(0);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(900);
  expect(bounds.y + bounds.height).toBeLessThan(
    (await trigger.boundingBox())!.y,
  );
  await page.screenshot({ path: "test-results/session-menu-desktop.png" });
  await page
    .getByRole("menuitem", { name: "Stop session", exact: true })
    .press("End");
  await expect(
    page.getByRole("menuitem", { name: "Copy session link", exact: true }),
  ).toBeFocused();
  await page
    .getByRole("menuitem", { name: "Copy session link", exact: true })
    .press("ArrowUp");
  await page
    .getByRole("menuitem", { name: "Session details", exact: true })
    .press("Enter");
  await expect(
    page.getByRole("dialog", { name: "Session details" }),
  ).toBeVisible();
  await expect(page.locator(".session-details")).toContainText("/workspace");
  await expect(page.locator(".session-details")).toContainText(
    "session-1-run-1",
  );
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await page.evaluate(() => {
    navigator.clipboard.writeText = async (value) => {
      (window as any).copiedSessionLink = value;
    };
  });
  await choose(page, "Copy session link");
  expect(await page.evaluate(() => (window as any).copiedSessionLink)).toBe(
    page.url(),
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await trigger.click();
  const mobile = (await menu.boundingBox())!;
  expect(mobile.x).toBeGreaterThanOrEqual(0);
  expect(mobile.x + mobile.width).toBeLessThanOrEqual(390);
  expect(mobile.y).toBeGreaterThanOrEqual(0);
  expect(mobile.y + mobile.height).toBeLessThanOrEqual(844);
  await page.screenshot({ path: "test-results/session-menu-mobile.png" });
});
test("Stop and Resume toggle, while Restart requires confirmation and preserves conversation history", async ({
  page,
}) => {
  await ready(page);
  const initial = await runId(page);
  const message = await page
    .locator("article.input .message-body")
    .last()
    .innerText();
  await choose(page, "Stop session");
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  await expect(
    page.getByRole("menuitem", { name: "Resume session", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("menuitem", { name: "Resume session", exact: true })
    .click();
  expect(await runId(page)).not.toBe(initial);
  const resumed = await runId(page);
  await choose(page, "Restart session");
  await expect(
    page.getByRole("dialog", { name: "Restart session" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await runId(page)).toBe(resumed);
  await choose(page, "Restart session");
  await page.getByRole("button", { name: "Restart", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await runId(page)).not.toBe(resumed);
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    message,
  );
});
test("Purge previews without deleting, cancels, then removes only the confirmed session", async ({
  page,
}) => {
  await ready(page);
  await choose(page, "Purge session");
  const dialog = page.getByRole("dialog", { name: "Purge session" });
  await expect(dialog).toContainText("/sandbox/session-1/journal");
  await expect(dialog).toContainText("Project workspace");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Session menu", exact: true }),
  ).toBeEnabled();
  await choose(page, "Purge session");
  await page
    .getByRole("button", { name: "Purge permanently", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Your workspace" }),
  ).toBeVisible();
  await expect(
    page
      .locator(".resource-panel .tree-session")
      .filter({ hasText: "session-1" }),
  ).toHaveCount(0);
  await expect(page.locator(".resource-panel .tree-session")).toHaveCount(7);
});
