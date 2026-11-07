import { expect, test } from "@playwright/test";
const recovery = "/iframe.html?id=workspace-errors--recovery&viewMode=story";
const pageStory = "/iframe.html?id=workspace-errors--page&viewMode=story";

test("dialog and independent page share errors, tabs, dismiss and clear history", async ({
  page,
}) => {
  await page.goto(recovery);
  await page.getByRole("button", { name: "Fail request", exact: true }).click();
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  const link = dialog.getByRole("link", {
    name: "Open errors page",
    exact: true,
  });
  await expect(link).toHaveAttribute("href", "/c/_/errors");
  await link.click();
  const main = page.getByRole("main", { name: "Errors", exact: true });
  await expect(dialog).toHaveCount(0);
  await expect(main).toContainText("Account service is offline");
  await expect(main).toContainText("Account: work");
  await expect(
    main.getByRole("link", { name: "Open errors page", exact: true }),
  ).toHaveCount(0);
  await main.getByRole("button", { name: "Dismiss", exact: true }).click();
  await expect(trigger).not.toHaveAttribute("data-active");
  await main.getByRole("radio", { name: "History 1", exact: true }).click();
  await expect(main).toContainText("Dismissed");
  await trigger.click();
  await dialog.getByRole("radio", { name: "History 1", exact: true }).click();
  await expect(dialog).toContainText("Dismissed");
  await dialog
    .getByRole("button", { name: "Clear history", exact: true })
    .click();
  await expect(main).toContainText("No error history");
  await expect(
    main.getByRole("radio", { name: "History 0", exact: true }),
  ).toBeChecked();
});

test("one hour without recurrence becomes Stale while closed, persists and opens the History page", async ({
  page,
}) => {
  await page.clock.install({ time: new Date("2026-11-07T03:00:00Z") });
  await page.goto(recovery);
  await page.getByRole("button", { name: "Fail request", exact: true }).click();
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).toHaveAttribute("data-active", "true");
  await page.clock.fastForward(60 * 60_000 + 1);
  await expect(trigger).not.toHaveAttribute("data-active");
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  await expect(dialog).toContainText("No active errors");
  await dialog.getByRole("radio", { name: "History 1", exact: true }).click();
  await expect(dialog.getByText("Stale", { exact: true })).toBeVisible();
  await expect(dialog.getByText("Resolved", { exact: true })).toHaveCount(0);
  const link = dialog.getByRole("link", {
    name: "Open errors page",
    exact: true,
  });
  await expect(link).toHaveAttribute("href", "/c/_/errors?tab=history");
  await link.click();
  const main = page.getByRole("main", { name: "Errors", exact: true });
  await expect(
    main.getByRole("radio", { name: "History 1", exact: true }),
  ).toBeChecked();
  await expect(main.getByText("Stale", { exact: true })).toBeVisible();
  await page.goto(pageStory);
  await page
    .getByRole("main", { name: "Errors", exact: true })
    .getByRole("radio", { name: "History 1", exact: true })
    .click();
  await expect(
    page
      .getByRole("main", { name: "Errors", exact: true })
      .getByText("Stale", { exact: true }),
  ).toBeVisible();
});

for (const width of [1200, 390])
  test(`independent Errors page fits ${width}px and renders the same record details`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 800 });
    await page.goto(pageStory);
    const main = page.getByRole("main", { name: "Errors", exact: true });
    await expect(main).toContainText("Account service is offline");
    await expect(main).toContainText("cxz.AccountService/Get");
    await expect(main).toContainText("First seen");
    await expect(
      main.getByRole("button", { name: "Close errors", exact: true }),
    ).toHaveCount(0);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
    const box = (await main.boundingBox())!;
    const list = (await main
      .getByLabel("Error list", { exact: true })
      .boundingBox())!;
    expect(list.height).toBeGreaterThan(200);
    expect(list.y + list.height).toBeLessThanOrEqual(box.y + box.height);
  });
