import { expect, test } from "@playwright/test";

const story = (name = "created-tasks") =>
  `/iframe.html?id=conversation-worksummary--${name}&viewMode=story`;
const dialogName = { name: "Created tasks", exact: true };

test("WorkSummary opens created Tasks above the trigger without shifting the conversation", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(story());
  const summaries = page.locator("[data-work-summary]");
  await expect(summaries).toHaveCount(2);
  const trigger = summaries.last().locator('[data-work-service="cxz_tasks"]');
  await expect(trigger).toHaveText("015");
  await expect(trigger).toHaveAccessibleName("Created tasks: 15");
  await expect(
    summaries.first().locator('[data-work-service="cxz_tasks"]'),
  ).toBeDisabled();
  await expect(
    summaries.last().locator('[data-work-service="cxz_memory"]'),
  ).toBeDisabled();
  const before = (await trigger.boundingBox())!;
  await trigger.click();
  const dialog = page.getByRole("dialog", dialogName);
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("link")).toHaveCount(15);
  await expect(
    dialog.getByRole("link", { name: /Implement Task MCP tools/ }),
  ).toHaveCount(1);
  await expect(
    dialog.getByRole("link", { name: /^Implement Task MCP tools task-1 / }),
  ).toHaveCount(0);
  const box = (await dialog.boundingBox())!;
  expect(box.y + box.height).toBeLessThanOrEqual(before.y);
  expect(Math.abs((await trigger.boundingBox())!.y - before.y)).toBeLessThan(1);
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(trigger).not.toBeFocused();
  await expect(trigger).toHaveCSS("outline-style", "solid");
  await expect(trigger).toHaveCSS("outline-width", "2px");
  const viewport = dialog.locator(".panel-scroll-content");
  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();

  await trigger.click();
  await page.locator("textarea").click();
  await expect(page.getByRole("dialog", dialogName)).toHaveCount(0);

  await trigger.click();
  const link = page.getByRole("dialog", dialogName).getByRole("link").first();
  await expect(link).toHaveAttribute(
    "href",
    "/c/_/project/workspace/task/task-4",
  );
  await link.click();
  await expect(page.getByRole("dialog", dialogName)).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveText("Wait for CI");
  expect(errors).toEqual([]);
});

test("narrow viewport keeps the created Tasks popup inside the screen", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 740 });
  await page.goto(story());
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await trigger.click();
  const dialog = page.getByRole("dialog", dialogName);
  await expect(dialog.getByRole("link")).toHaveCount(15);
  const box = (await dialog.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(360);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.y + box.height).toBeLessThanOrEqual(740);
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(trigger).toBeFocused();
});

test("zero Task counts stay disabled while loading and after read-only calls", async ({
  page,
}) => {
  await page.goto(story("loading-tasks"));
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await expect(trigger).toHaveAttribute("aria-busy", "true");
  await expect(trigger).toHaveText("000");
  await expect(trigger).toBeDisabled();
  const before = (await trigger.boundingBox())!;
  await expect(trigger).toHaveText("015");
  await expect(trigger).toBeEnabled();
  const after = (await trigger.boundingBox())!;
  expect(after.width).toBe(before.width);
  expect(after.height).toBe(before.height);
  await page.goto(story("no-created-tasks"));
  for (const button of await page
    .locator('[data-work-service="cxz_tasks"]')
    .all()) {
    await expect(button).toHaveText("000");
    await expect(button).toBeDisabled();
  }
  await expect(page.getByRole("dialog", dialogName)).toHaveCount(0);
});

test("cached Task counts and open lists update together and preserve trigger styling", async ({
  page,
}) => {
  await page.goto(story("refreshing-tasks"));
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await expect(trigger).toHaveText("001");
  await trigger.click();
  const dialog = page.getByRole("dialog", dialogName);
  await expect(dialog.getByRole("status")).toHaveText("Loading…");
  await expect(dialog.getByRole("link")).toHaveCount(1);
  const loading = (await dialog.boundingBox())!;
  await expect(dialog.getByRole("link")).toHaveCount(15);
  await expect(trigger).toHaveText("015");
  expect((await dialog.boundingBox())!.height).toBe(loading.height);
  await expect(trigger).toHaveCSS("outline-style", "solid");
  await page.keyboard.press("Escape");
  await page.locator('[data-work-service="cxz_tasks"]').last().click();
  await expect(dialog.getByRole("link")).toHaveCount(15);
  await expect(dialog.getByRole("status")).toHaveCount(0);
});

test("cached Tasks remain accessible after a refresh fails", async ({
  page,
}) => {
  await page.goto(story("unavailable-tasks"));
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await expect(trigger).toHaveText("001");
  await trigger.click();
  await expect(
    page.getByRole("dialog", dialogName).getByRole("link"),
  ).toHaveCount(1);
  await expect(
    page.getByRole("dialog", dialogName).getByRole("alert"),
  ).toContainText("Update the Manager and project runtime");
  await expect(
    page.getByRole("dialog", dialogName).getByRole("button", { name: "Retry" }),
  ).toBeVisible();
  await expect(trigger).toHaveCSS("outline-style", "solid");
});
