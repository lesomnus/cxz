import { expect, test, devices } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("session search reaches old history, keeps its overlay through jumps and outside clicks, and closes explicitly", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/sessions/session-3");
  const pane = page.locator(".transcript");
  await expect(pane).toContainText("History item 2100", { timeout: 45000 });
  await expect(pane).not.toContainText("History item 0001");
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  await editor.focus();
  await page.keyboard.press("Control+Shift+F");
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox", {
    name: "Search this conversation",
  });
  await expect(input).toBeFocused();
  const outer = (await page.locator("main.conversation").boundingBox())!,
    box = (await search.boundingBox())!;
  expect(box.y).toBeGreaterThan(outer.y);
  expect(box.x).toBeGreaterThanOrEqual(outer.x);
  expect(box.x + box.width).toBeLessThanOrEqual(outer.x + outer.width);
  await input.fill("HISTORY ITEM 0001");
  await search.getByRole("button", { name: /History item 0001/ }).click();
  await expect(pane).toContainText("History item 0001");
  await expect(search).toBeVisible();
  await pane.click({ position: { x: 10, y: 20 } });
  await expect(search).toBeVisible();
  await editor.focus();
  await expect(search).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(search).toHaveCount(0);
  await expect(editor).toBeFocused();
  await page.keyboard.press("Control+Shift+F");
  await expect(input).toHaveValue("HISTORY ITEM 0001");
  await page.keyboard.press("Control+Shift+F");
  await expect(input).toBeFocused();
  await search.getByRole("button", { name: "Close search" }).click();
  await expect(search).toHaveCount(0);
});

test("search pagination remains scoped and keyboard result navigation leaves search open", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/sessions/session-3");
  await expect(page.locator(".transcript")).toContainText("History item 2100", {
    timeout: 45000,
  });
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await page.keyboard.press("Control+Shift+F");
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  await input.fill("History item");
  await expect(search.getByRole("status")).toHaveText("100 matches");
  await search.getByRole("button", { name: "Load more results" }).click();
  await expect(search.getByRole("status")).toHaveText("200 matches");
  await input.focus();
  await input.press("ArrowDown");
  await expect(search.locator('[data-search-index="1"]')).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await input.press("Enter");
  await expect(search).toBeVisible();
  await input.fill("query that never appears in the sandbox");
  await expect(search.getByRole("status")).toHaveText("No matches");
  await search.getByRole("button", { name: "Close search" }).click();
});
