import { test, expect, devices, type Page } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});
async function ready(page: Page) {
  await page.goto("/sandbox.html#/c/_/session/session-1");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
}
async function choose(page: Page, name: string) {
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  await page.getByRole("menuitem", { name, exact: true }).click();
}
async function runId(page: Page) {
  await choose(page, "Details");
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
  await expect(
    page.getByRole("button", { name: "Terminal", exact: true }),
  ).toHaveCount(0);
  await trigger.focus();
  await trigger.press("ArrowDown");
  const menu = page.getByRole("menu", { name: "Session menu" });
  await expect(menu.getByRole("group", { name: "Controls" })).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Stop", exact: true }),
  ).toBeFocused();
  await expect(menu.getByRole("menuitem")).toHaveText([
    "Stop",
    "Restart",
    "Fork",
    "Purge",
    "Insert secret",
    "Shared memory",
    "Auxiliary AI",
    "Conversation usage",
    "Background tasks",
    "Log in",
    "Upload files",
    "Upload folder",
    "Details",
  ]);
  await expect(
    menu
      .getByRole("menuitem")
      .or(menu.getByRole("menuitemcheckbox"))
      .locator("svg"),
  ).toHaveCount(8);
  await expect(menu.getByText("Information", { exact: true })).toHaveCount(0);
  await expect(
    menu.getByRole("menuitem", { name: "Copy session link" }),
  ).toHaveCount(0);
  await menu.evaluate(async (el) => {
    await Promise.all(
      el.getAnimations({ subtree: true }).map((a) => a.finished),
    );
  });
  const bounds = (await menu.boundingBox())!;

  const triggerBounds = (await trigger.boundingBox())!;
  const toolbarBounds = (await page
    .locator(".composer-toolbar")
    .boundingBox())!;
  const inset = triggerBounds.y - toolbarBounds.y;
  expect(toolbarBounds.y - (bounds.y + bounds.height)).toBeCloseTo(inset, 0);
  // Measure in the same frame, before ResizeObserver can repair stale coordinates.
  expect(
    await menu.evaluate((el) => {
      const trigger = document.querySelector(
        '[aria-label="Session menu"][aria-haspopup="menu"]',
      )!;
      const previous = el.style.maxHeight;
      el.style.maxHeight = "180px";
      const gap =
        trigger.getBoundingClientRect().top - el.getBoundingClientRect().bottom;
      el.style.maxHeight = previous;
      return gap;
    }),
  ).toBeCloseTo(2 * inset, 0);

  expect(bounds.y).toBeGreaterThanOrEqual(0);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(900);
  expect(bounds.y + bounds.height).toBeLessThan(
    (await trigger.boundingBox())!.y,
  );
  await page.screenshot({ path: "test-results/session-menu-desktop.png" });
  await page.getByRole("menuitem", { name: "Stop", exact: true }).press("End");
  await expect(
    page.getByRole("menuitem", { name: "Details", exact: true }),
  ).toBeFocused();
  await page
    .getByRole("menuitem", { name: "Details", exact: true })
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
  await page.setViewportSize({ width: 390, height: 844 });
  await trigger.click();
  const mobile = (await menu.boundingBox())!;
  expect(mobile.x).toBeGreaterThanOrEqual(0);
  expect(mobile.x + mobile.width).toBeLessThanOrEqual(390);
  expect(mobile.y).toBeGreaterThanOrEqual(0);
  expect(mobile.y + mobile.height).toBeLessThanOrEqual(844);
  await page.screenshot({ path: "test-results/session-menu-mobile.png" });
});
test("Fork preserves the conversation and new input stays in the child session", async ({
  page,
}) => {
  await ready(page);
  const original = page.url();
  const messages = page.locator(
    ".transcript-content article.input .message-body",
  );
  const previous = await messages.allTextContents();
  await choose(page, "Fork");
  await expect(page).toHaveURL(/#\/c\/_\/session\/session-fork-/);
  await expect(messages).toHaveText(previous);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Explore the second direction in this fork.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(messages.last()).toHaveText(
    "Explore the second direction in this fork.",
  );
  await page.goto(original);
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible();
  await expect(messages).toHaveText(previous);
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
  await choose(page, "Stop");
  const stop = page.getByRole("dialog", { name: "Stop session" });
  await expect(stop).toBeVisible();
  await expect(
    stop.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(stop).toHaveCount(0);
  expect(await runId(page)).toBe(initial);
  await choose(page, "Stop");
  await page
    .getByRole("dialog", { name: "Stop session" })
    .getByRole("button", { name: "Stop", exact: true })
    .click();
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  await expect(
    page.getByRole("menuitem", { name: "Resume", exact: true }),
  ).toBeVisible();
  await page.getByRole("menuitem", { name: "Resume", exact: true }).click();
  expect(await runId(page)).not.toBe(initial);
  const resumed = await runId(page);
  await choose(page, "Restart");
  await expect(
    page.getByRole("dialog", { name: "Restart session" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await runId(page)).toBe(resumed);
  await choose(page, "Restart");
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
  // Counted rather than hardcoded: what this verifies is that exactly one
  // session goes, not how many the fixture happens to seed.
  const listed = page.locator(".resource-panel .tree-session");
  const before = await listed.count();
  expect(before).toBeGreaterThan(1);
  await choose(page, "Purge");
  const dialog = page.getByRole("dialog", { name: "Purge session" });
  await expect(dialog).toContainText("/sandbox/session-1/journal");
  await expect(dialog).toContainText("Project workspace");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Session menu", exact: true }),
  ).toBeEnabled();
  await choose(page, "Purge");
  await page
    .getByRole("button", { name: "Purge permanently", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Sessions", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .locator(".resource-panel .tree-session")
      .filter({ hasText: "session-1" }),
  ).toHaveCount(0);
  await expect(listed).toHaveCount(before - 1);
});

test("Details edits title and alias while keeping conversation identity and updating the alias address", async ({
  page,
}) => {
  await ready(page);
  const originalRun = await runId(page);
  const originalMessage = await page
    .locator("article.input .message-body")
    .last()
    .innerText();
  await choose(page, "Details");
  const details = page.getByRole("dialog", {
    name: "Session details",
    exact: true,
  });
  await details.getByRole("button", { name: "Edit Title" }).click();
  const titleEditor = page.getByRole("dialog", {
    name: "Edit Title",
    exact: true,
  });
  await titleEditor
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Updated project review");
  await titleEditor
    .getByRole("textbox", { name: "Title", exact: true })
    .press("Enter");
  await expect(titleEditor).toHaveCount(0);
  await expect(details.getByRole("button", { name: "Edit Title" })).toHaveText(
    "Updated project review",
  );
  await expect(
    page
      .locator(".resource-panel .tree-session")
      .filter({ hasText: "session-1" }),
  ).toContainText("Updated project review");
  await details.getByRole("button", { name: "Edit Alias" }).click();
  const aliasEditor = page.getByRole("dialog", {
    name: "Edit Alias",
    exact: true,
  });
  await aliasEditor
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("oak-tree");
  await aliasEditor
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(aliasEditor).toHaveCount(0);
  await expect(details.getByRole("button", { name: "Edit Alias" })).toHaveText(
    "oak-tree",
  );
  await expect(
    page
      .locator(".resource-panel .tree-session")
      .filter({ hasText: "Updated project review" }),
  ).toContainText("oak-tree");
  await details.getByRole("button", { name: "Edit Alias" }).click();
  await aliasEditor
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("session-2");
  await aliasEditor
    .getByRole("textbox", { name: "Alias", exact: true })
    .press("Enter");
  await expect(aliasEditor.getByRole("alert")).toContainText(
    "alias is already in use",
  );
  await page.screenshot({
    path: "test-results/session-details-edit-error.png",
  });
  await page.keyboard.press("Escape");
  await expect(aliasEditor).toHaveCount(0);
  await expect(details).toBeVisible();
  await expect(
    details.getByRole("button", { name: "Edit Alias" }),
  ).toBeFocused();
  await expect(details.getByRole("button", { name: "Edit Alias" })).toHaveText(
    "oak-tree",
  );
  await page
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  expect(await runId(page)).toBe(originalRun);
  await expect(page).toHaveURL(/#\/c\/_\/session\/oak-tree$/);
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    originalMessage,
  );
  await choose(page, "Details");
  await expect(details.getByRole("button", { name: "Edit Title" })).toHaveText(
    "Updated project review",
  );
  await expect(details.getByRole("button", { name: "Edit Alias" })).toHaveText(
    "oak-tree",
  );
});
