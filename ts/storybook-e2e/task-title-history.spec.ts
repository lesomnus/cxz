import { expect, test } from "@playwright/test";

const story = (name: string) =>
  `/iframe.html?id=resources-tasks--${name}&viewMode=story`;
for (const width of [1280, 390]) {
  test(`readonly title expands downward at its existing width and copies from the expanded area at ${width}px`, async ({
    page,
    context,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(story("read-only-long-title"));
    const detail = page.getByRole("region", {
      name: "Task details",
      exact: true,
    });
    const title = detail.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toHaveAttribute("aria-readonly", "true");
    await page.evaluate(() => document.fonts.ready);
    await title.scrollIntoViewIfNeeded();
    const value = await title.textContent();
    const anchor = (await title.boundingBox())!;
    await title.hover();
    const popup = page.getByRole("dialog", { name: "Task title", exact: true });
    await expect(popup).toBeVisible();
    await expect
      .poll(async () => (await popup.boundingBox())?.width)
      .toBeCloseTo(anchor.width, 1);
    const expanded = (await popup.boundingBox())!;
    expect(expanded.x).toBeCloseTo(anchor.x, 1);
    expect(expanded.y).toBeCloseTo(anchor.y, 1);
    expect(expanded.height).toBeGreaterThan(anchor.height);
    const copy = popup.getByRole("button", {
      name: "Copy full title",
      exact: true,
    });
    await page.mouse.move(
      expanded.x + expanded.width / 2,
      expanded.y + expanded.height - 14,
    );
    await expect(popup).toBeVisible();
    await context.grantPermissions(["clipboard-read", "clipboard-write"], {
      origin: new URL(page.url()).origin,
    });
    await copy.click({
      position: {
        x: expanded.width / 3,
        y: (await copy.boundingBox())!.height - 4,
      },
    });
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(value);
    await expect(popup.getByRole("status")).toHaveText("Copied");
    await page.keyboard.press("Escape");
    await expect(popup).toHaveCount(0);
    await expect(title).toHaveCSS("opacity", "1");
    await expect(title).toBeFocused();
  });
}

test("task history loads retained audit rows in pages and refreshes after saving", async ({
  page,
}) => {
  await page.goto(story("change-history"));
  const detail = page.getByRole("region", {
    name: "Task details",
    exact: true,
  });
  const history = detail.getByRole("table", {
    name: "Task history",
    exact: true,
  });
  await expect(history.locator("tbody tr")).toHaveCount(20);
  await expect(history.locator("tbody tr").first()).toContainText("Updated");
  await expect(history.locator("tbody tr").first()).toContainText(
    "Title, Priority",
  );
  await detail
    .getByRole("button", { name: "Load more changes", exact: true })
    .click();
  await expect(history.locator("tbody tr")).toHaveCount(28);
  await expect(
    detail.getByRole("button", { name: "Load more changes", exact: true }),
  ).toHaveCount(0);
  await detail
    .getByRole("textbox", { name: "Description", exact: true })
    .fill("Recorded description update");
  await detail.getByRole("button", { name: "Save", exact: true }).click();
  await expect(history.locator("tbody tr").first()).toContainText(
    "Description",
  );
  await expect(history.locator("tbody tr")).toHaveCount(20);
});

test("older runtimes show an explicit history update requirement", async ({
  page,
}) => {
  await page.goto(story("history-unavailable"));
  const detail = page.getByRole("region", {
    name: "Task details",
    exact: true,
  });
  await expect(
    detail.getByRole("table", { name: "Task history", exact: true }),
  ).toContainText("History unavailable");
  await expect(
    detail.getByText(
      "Update the Manager and project runtime to view Task history.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    detail.getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveValue("Connect the task table");
});
