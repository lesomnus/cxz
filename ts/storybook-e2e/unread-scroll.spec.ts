import { expect, test } from "@playwright/test";

test("unread sessions tint only the scroll edges hiding them", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=sessions-panel--unread-beyond-scroll&viewMode=story",
  );
  const area = page.locator(".panel-scroll-region");
  const pane = page.locator(".panel-scroll-content");
  await expect(area).toHaveAttribute("data-unread-below", "true");
  await expect(area).not.toHaveAttribute("data-unread-above");
  await expect
    .poll(() =>
      page
        .locator(".panel-scroll-fade-bottom")
        .evaluate((el) => getComputedStyle(el, "::after").opacity),
    )
    .toBe("1");
  await pane.evaluate((el) => {
    el.scrollTop = (el.scrollHeight - el.clientHeight) / 2;
  });
  await expect(area).toHaveAttribute("data-unread-above", "true");
  await expect(area).toHaveAttribute("data-unread-below", "true");
  await pane.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await expect(area).toHaveAttribute("data-unread-above", "true");
  await expect(area).not.toHaveAttribute("data-unread-below");
  // Clearing unread markers must remove the tint without requiring another scroll.
  await pane
    .locator('[data-unread="true"]')
    .evaluateAll((rows) =>
      rows.forEach((row) => row.removeAttribute("data-unread")),
    );
  await expect(area).not.toHaveAttribute("data-unread-above");
  await expect(area).not.toHaveAttribute("data-unread-below");
  await expect
    .poll(() =>
      page
        .locator(".panel-scroll-fade-top")
        .evaluate((el) => getComputedStyle(el, "::after").opacity),
    )
    .toBe("0");
});

test("ordinary overflow without unread sessions keeps its neutral shadow", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=sessions-panel--long-list&viewMode=story");
  const area = page.locator(".panel-scroll-region");
  await expect(page.locator(".tree-session")).toHaveCount(24);
  await expect(area).not.toHaveAttribute("data-unread-below");
  await page.locator(".panel-scroll-content").evaluate((el) => {
    el.scrollTop = 300;
  });
  await expect(area).not.toHaveAttribute("data-unread-above");
});
