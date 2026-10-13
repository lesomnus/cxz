import { expect, test } from "@playwright/test";

test("turn work collapses into one line and its dialog never covers adjacent panes", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-workdialog--contained&viewMode=story",
  );
  const summaries = page.locator("[data-work-summary]");
  await expect(summaries).toHaveCount(2);
  await expect(summaries.first().getByRole("button")).toHaveText("4");
  const icon = summaries.first().getByRole("button").locator("svg");
  await expect(icon).toBeVisible();
  await expect(icon.locator("path")).toBeVisible();
  await expect(icon).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  expect(await icon.evaluate((node) => node.getAttribute("fill"))).toBe(
    "currentColor",
  );
  const tooltip = summaries.first().getByRole("tooltip");
  await expect(tooltip).toBeHidden();
  await summaries.first().getByRole("button").hover();
  await expect(tooltip).toContainText("4 tasks performed · View details");
  await expect(tooltip).toBeVisible();
  await expect(page.locator(".transcript .tool-activity")).toHaveCount(0);
  const pane = page.locator(".transcript");
  const before = await pane.evaluate((node) => ({
    top: node.scrollTop,
    height: node.scrollHeight,
  }));
  await summaries.first().getByRole("button").click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await expect(dialog.locator(".tool-activity")).toHaveCount(4);
  await dialog.evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished),
    );
  });
  const bounds = (await dialog.locator(".floating-card").boundingBox())!;
  const area = (await page.locator(".conversation").boundingBox())!;
  const panel = (await page
    .getByRole("complementary", { name: "Sessions panel", exact: true })
    .boundingBox())!;
  const files = (await page
    .getByRole("complementary", { name: "File explorer", exact: true })
    .boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(panel.x + panel.width);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(files.x);
  expect(area.x + area.width - bounds.x - bounds.width).toBeGreaterThan(0);
  expect(bounds.y - area.y).toBeGreaterThan(0);
  expect(area.y + area.height - bounds.y - bounds.height).toBeGreaterThan(0);
  expect(bounds.height / area.height).toBeGreaterThan(0.9);
  expect(
    await pane.evaluate((node) => ({
      top: node.scrollTop,
      height: node.scrollHeight,
    })),
  ).toEqual(before);
  await dialog
    .getByRole("button", { name: "Close task dialog", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(summaries.first().getByRole("button")).toBeFocused();
  await summaries.nth(1).getByRole("button").click();
  await expect(dialog.locator(".tool-activity")).toHaveCount(1);
  await dialog
    .getByRole("button", { name: "Close task dialog", exact: true })
    .press("Escape");
  await expect(dialog).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("long task lists remain virtualized and scroll independently", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-workdialog--long-turn&viewMode=story",
  );
  await expect(
    page.locator("[data-work-summary]").getByRole("button"),
  ).toHaveText("600");
  await page.getByRole("button", { name: "View details", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await expect(dialog.locator(".tool-activity").first()).toBeVisible();
  expect(await dialog.locator(".tool-activity").count()).toBeLessThan(40);
  const before = await page
    .locator(".transcript")
    .evaluate((node) => node.scrollTop);
  await dialog
    .getByRole("scrollbar", { name: "Panel scroll", exact: true })
    .focus();
  await page.keyboard.press("End");
  await expect(dialog.locator('.tool-activity[data-seq="1200"]')).toBeVisible();
  expect(await dialog.locator(".tool-activity").count()).toBeLessThan(40);
  expect(
    await page.locator(".transcript").evaluate((node) => node.scrollTop),
  ).toBe(before);
  await dialog
    .getByRole("button", { name: "Close task dialog", exact: true })
    .click();
  await page.setViewportSize({ width: 390, height: 620 });
  await page.getByRole("button", { name: "View details", exact: true }).click();
  await dialog.evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished),
    );
  });
  const bounds = (await dialog.locator(".floating-card").boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
});

test("paging earlier tasks preserves the open execution and its detail editor", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-workdialog--paged-turn&viewMode=story",
  );
  await expect(
    page.locator("[data-work-summary]").getByRole("button"),
  ).toHaveText("1+");
  await page.getByRole("button", { name: "View details", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Bash · Details", exact: true })
    .click();
  await dialog.getByRole("tab", { name: "Output", exact: true }).click();
  await expect(dialog.getByRole("tabpanel")).toContainText("main");
  await dialog
    .getByRole("button", { name: "Load earlier tasks", exact: true })
    .click();
  await expect(
    page.locator("[data-work-summary]").getByRole("button"),
  ).toHaveText("2");
  await expect(dialog.locator(".tool-activity")).toHaveCount(2);
  await expect(dialog.getByRole("tabpanel")).toContainText("main");
  await expect(
    dialog.getByRole("button", { name: "Load earlier tasks", exact: true }),
  ).toHaveCount(0);
});

test("work batches split at assistant replies and summaries align right", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-workdialog--between-responses&viewMode=story",
  );
  const summaries = page.locator("[data-work-summary]");
  await expect(summaries).toHaveCount(2);
  await expect(summaries.first().getByRole("button")).toHaveText("1");
  await expect(summaries.nth(1).getByRole("button")).toHaveText("2");
  await expect(summaries.first()).toHaveCSS("justify-content", "flex-end");
  await summaries.first().getByRole("button").click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await expect(dialog.locator(".tool-activity")).toHaveCount(1);
  await dialog
    .getByRole("button", { name: "Close task dialog", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await summaries.nth(1).getByRole("button").click();
  await expect(dialog.locator(".tool-activity")).toHaveCount(2);
  expect(
    await dialog.evaluate(
      (node) => getComputedStyle(node, "::backdrop").backgroundColor,
    ),
  ).toBe("rgba(0, 0, 0, 0)");
  await expect(dialog.locator("[data-work-backdrop]")).not.toHaveCSS(
    "background-color",
    "rgba(0, 0, 0, 0)",
  );
});
