import { expect, test } from "@playwright/test";
const live =
  "/iframe.html?id=workspace-errors-rows--incoming-errors&viewMode=story";

for (const width of [1200, 390])
  test(`sidebar error dialog has compact header and full-row wide dismiss at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 850 });
    await page.goto(
      "/iframe.html?id=conversation-session-menu--always-open&viewMode=story",
    );
    const menu = page.locator(".action-menu-popup");
    const firstAction = menu
      .locator('[data-action-menu-layout="row"] button')
      .first();
    await expect(firstAction).toBeVisible();
    await page.waitForTimeout(400);
    const menuBox = (await menu.boundingBox())!;
    const actionBox = (await firstAction.boundingBox())!;
    const outerInset = actionBox.y - menuBox.y;
    const inset = await menu.evaluate((element) =>
      parseFloat(getComputedStyle(element).paddingTop),
    );
    await page.goto(
      "/iframe.html?id=workspace-errors--recovery&viewMode=story",
    );
    await page
      .getByRole("button", { name: "Fail request", exact: true })
      .click();
    await page.getByRole("button", { name: "Errors", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
    await expect(dialog).toBeVisible();
    await page.waitForTimeout(400);
    const box = (await dialog.boundingBox())!;
    expect(box.width).toBe(Math.min(420, width - 16));
    const header = dialog.locator(".card-heading");
    const close = header.getByRole("button", {
      name: "Close errors",
      exact: true,
    });
    expect((await header.boundingBox())!.height).toBe(
      (await close.boundingBox())!.height + 2 * inset,
    );
    await expect(header).toHaveCSS("padding-top", `${inset}px`);
    await expect(header).toHaveCSS("padding-bottom", `${inset}px`);
    await expect(header).toHaveCSS("padding-left", `${inset}px`);
    await expect(header).toHaveCSS("padding-right", `${inset}px`);
    const cardBox = (await dialog.locator(".floating-card").boundingBox())!;
    const closeBox = (await close.boundingBox())!;
    expect(closeBox.width).toBe(closeBox.height);
    expect(closeBox.y - cardBox.y).toBeCloseTo(outerInset, 0);
    expect(cardBox.x + cardBox.width - closeBox.x - closeBox.width).toBeCloseTo(
      outerInset,
      0,
    );
    const row = dialog.getByRole("row");
    await row.hover();
    const dismiss = row.getByRole("button", { name: "Dismiss", exact: true });
    const rowBox = (await row.boundingBox())!;
    const buttonBox = (await dismiss.boundingBox())!;
    expect(buttonBox.width).toBe(44);
    expect(buttonBox.height).toBeGreaterThanOrEqual(rowBox.height - 1);
    expect(buttonBox.y).toBeCloseTo(rowBox.y, 0);
    expect(buttonBox.x + buttonBox.width).toBeCloseTo(
      rowBox.x + rowBox.width,
      0,
    );
  });

test("only new error episodes slide down, not initial rows, count updates or tab changes", async ({
  page,
}) => {
  await page.goto(live);
  const table = page.getByRole("table", { name: "Errors", exact: true });
  const existing = table.getByRole("row").filter({ hasText: "Existing error" });
  await expect(existing).toBeVisible();
  await expect(existing).not.toHaveAttribute("data-animate-enter");
  await page.evaluate(() => {
    const table = document.querySelector<HTMLTableElement>(
      'table[aria-label="Errors"]',
    )!;
    table.dataset.entryAnimations = "0";
    table.addEventListener("animationstart", (event) => {
      if (
        !(event.target instanceof HTMLElement) ||
        !event.target.matches("tr[data-animate-enter]")
      )
        return;
      table.dataset.entryAnimations = String(
        Number(table.dataset.entryAnimations) + 1,
      );
    });
  });
  await page.getByRole("button", { name: "Add error", exact: true }).click();
  const first = table.getByRole("row").filter({ hasText: "New error 1" });
  await expect(first).toBeVisible();
  await expect(table).toHaveAttribute("data-entry-animations", "1");
  await expect(table.getByRole("row").first()).toContainText("New error 1");
  await expect(existing).not.toHaveAttribute("data-animate-enter");
  const frames = await first.evaluate((element) => {
    const animation = element
      .getAnimations()
      .find(
        (animation) =>
          animation instanceof CSSAnimation &&
          animation.animationName.includes("error-row-enter"),
      );
    const effect = animation?.effect;
    return effect instanceof KeyframeEffect
      ? effect.getKeyframes().map((frame) => frame.transform)
      : undefined;
  });
  // The event count is durable; keyframes may already have finished on a busy browser.
  if (frames) expect(frames[0]).toBe("translateY(-100%)");
  await page.waitForTimeout(300);
  await page
    .getByRole("button", { name: "Repeat latest error", exact: true })
    .click();
  await expect(
    first.getByTitle("2 occurrences", { exact: true }),
  ).toContainText("002");
  await page.waitForTimeout(300);
  await expect(table).toHaveAttribute("data-entry-animations", "1");
  await page.getByRole("button", { name: "Add error", exact: true }).click();
  await expect(table).toHaveAttribute("data-entry-animations", "2");
  const second = table.getByRole("row").filter({ hasText: "New error 2" });
  await second.hover();
  await second.getByRole("button", { name: "Dismiss", exact: true }).click();
  await page.getByRole("radio", { name: "History 1", exact: true }).click();
  await expect(table.getByRole("row")).not.toHaveAttribute(
    "data-animate-enter",
  );
  await page.waitForTimeout(300);
  await expect(table).toHaveAttribute("data-entry-animations", "2");
});

test("new error rows respect reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(live);
  await page.getByRole("button", { name: "Add error", exact: true }).click();
  const row = page.getByRole("row").filter({ hasText: "New error 1" });
  await expect(row).toBeVisible();
  await expect(row).toHaveCSS("animation-name", "none");
});
