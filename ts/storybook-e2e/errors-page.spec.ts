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
  await expect(main).toContainText("Read Account");
  await expect(main).toContainText("work");
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
  await expect(dialog.getByText(/RPC · Stale/)).toBeVisible();
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
  await expect(main.getByText("Stale", { exact: true }).first()).toBeVisible();
  await page.goto(pageStory);
  await page
    .getByRole("main", { name: "Errors", exact: true })
    .getByRole("radio", { name: "History 1", exact: true })
    .click();
  await expect(
    page
      .getByRole("main", { name: "Errors", exact: true })
      .getByText("Stale", { exact: true })
      .first(),
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
    await expect(main).toContainText("Read Account");
    await expect(main).not.toContainText("cxz.AccountService/Get");
    await expect(main.locator(".floating-card")).toHaveCount(0);
    const table = main.getByRole("table", { name: "Errors", exact: true });
    await expect(table).toBeVisible();
    const toolbar = main.locator("[data-resource-toolbar-content]");
    expect((await table.boundingBox())!.x).toBeCloseTo(
      (await toolbar.boundingBox())!.x,
      0,
    );
    expect((await table.boundingBox())!.width).toBeCloseTo(
      (await toolbar.boundingBox())!.width,
      0,
    );
    await main.getByRole("button", { name: "Details", exact: true }).click();
    const messageWidth = (await table
      .getByRole("columnheader", { name: "Message", exact: true })
      .boundingBox())!.width;
    expect(messageWidth / (await table.boundingBox())!.width).toBeGreaterThan(
      width < 600 ? 0.6 : 0.2,
    );
    await expect(
      main.getByRole("region", { name: "Error details", exact: true }),
    ).toContainText("First seen");
    await expect(
      main.getByRole("region", { name: "Error details", exact: true }),
    ).toContainText("cxz.AccountService/Get");
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

test("resource table handles a populated collection and expands complete per-error details", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=workspace-errors--overview&viewMode=story");
  const main = page.getByRole("main", { name: "Errors", exact: true });
  const table = main.getByRole("table", { name: "Errors", exact: true });
  await expect(
    main.getByRole("radio", { name: "Active 6", exact: true }),
  ).toBeChecked();
  await expect(table.getByRole("row")).toHaveCount(7);
  const row = table
    .getByRole("row")
    .filter({ hasText: "Memory revision changed" });
  const details = row.getByRole("button", { name: "Details", exact: true });
  await details.focus();
  await page.keyboard.press("Enter");
  await expect(details).toHaveAttribute("aria-expanded", "true");
  const expanded = main.getByRole("region", {
    name: "Error details",
    exact: true,
  });
  await expect(expanded).toContainText("cxz.MemoryService/Patch");
  await expect(expanded).toContainText("https://workspace.example.com");
  await expect(expanded).toContainText("Request ID");
  await main.getByRole("radio", { name: "History 3", exact: true }).click();
  await expect(table.getByRole("row")).toHaveCount(4);
  await expect(
    main.getByRole("region", { name: "Error details", exact: true }),
  ).toHaveCount(0);
  await main
    .getByRole("button", { name: "Clear history", exact: true })
    .click();
  await expect(
    main.getByText("No error history", { exact: true }),
  ).toBeVisible();
  await main.getByRole("radio", { name: "Active 6", exact: true }).click();
  await expect(table.getByRole("row")).toHaveCount(7);
});

test("normalizes mixed error kinds in the sidebar and page, leaving raw transport fields in details", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=workspace-errors--overview&viewMode=story");
  const main = page.getByRole("main", { name: "Errors", exact: true });
  const table = main.getByRole("table", { name: "Errors", exact: true });
  await expect(
    table.getByRole("columnheader", { name: "Kind", exact: true }),
  ).toBeVisible();
  await expect(
    table
      .getByRole("row")
      .filter({ hasText: "Conversation stream disconnected" }),
  ).toContainText("Release review");
  await expect(table).not.toContainText("cxz.SessionService/Events");
  const upload = table
    .getByRole("row")
    .filter({ hasText: "File upload failed" });
  await expect(upload).toContainText("HTTP");
  await expect(upload).toContainText("HTTP request");
  await upload.getByRole("button", { name: "Details", exact: true }).click();
  const details = main.getByRole("region", {
    name: "Error details",
    exact: true,
  });
  await expect(details).toContainText("HTTP status");
  await expect(details).toContainText("503");
  await expect(details).toContainText("/upload");
  await page.getByRole("button", { name: "Errors", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  const compact = dialog.getByRole("table", { name: "Errors", exact: true });
  await expect(compact).toContainText("Watch conversation · Release review");
  await expect(compact).not.toContainText("89502035e952a2c227802d29");
  await dialog.getByRole("radio", { name: "History 3", exact: true }).click();
  const login = compact
    .getByRole("row")
    .filter({ hasText: "Provider login was interrupted" });
  await expect(login).toContainText("Connection");
  await expect(login).toContainText("Provider login · personal");
  await login.getByRole("button", { name: "Details", exact: true }).click();
  await expect(
    dialog.getByRole("region", { name: "Error details", exact: true }),
  ).toContainText("1006");
});
