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
  const viewport = dialog.locator(".panel-scroll-content");
  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();

  await summaries.first().locator('[data-work-service="cxz_tasks"]').click();
  await expect(page.getByRole("dialog", dialogName)).toContainText(
    "No Tasks were created in this range.",
  );
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

test("created Tasks has loading, empty and failure states", async ({
  page,
}) => {
  await page.goto(story("loading-tasks"));
  await page.locator('[data-work-service="cxz_tasks"]').last().click();
  const dialog = page.getByRole("dialog", dialogName);
  await expect(dialog.getByRole("status")).toHaveText("Loading…");
  const loading = (await dialog.boundingBox())!;
  await expect(dialog.getByRole("link")).toHaveCount(15);
  expect((await dialog.boundingBox())!.height).toBe(loading.height);
  await page.keyboard.press("Escape");
  await page.locator('[data-work-service="cxz_tasks"]').last().click();
  await expect(dialog.getByRole("link")).toHaveCount(15);
  await expect(dialog.getByRole("status")).toHaveCount(0);

  await page.goto(story("no-created-tasks"));
  await page.locator('[data-work-service="cxz_tasks"]').last().click();
  await expect(page.getByRole("dialog", dialogName)).toContainText(
    "No Tasks were created in this range.",
  );

  await page.goto(story("unavailable-tasks"));
  await page.locator('[data-work-service="cxz_tasks"]').last().click();
  await expect(
    page.getByRole("dialog", dialogName).getByRole("alert"),
  ).toContainText("Update the Manager and project runtime");
  await expect(
    page.getByRole("dialog", dialogName).getByRole("button", { name: "Retry" }),
  ).toBeVisible();
});
