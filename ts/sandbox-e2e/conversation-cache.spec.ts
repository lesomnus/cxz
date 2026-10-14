import { expect, test, devices } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("cached conversation restores its reading row and offset without keeping hidden transcript DOM", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/sessions/session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible({ timeout: 45000 });
  const pane = page.locator(".transcript");
  const anchor = () =>
    pane.evaluate((el) => {
      const top = el.getBoundingClientRect().top;
      const row = [...el.querySelectorAll<HTMLElement>("[data-row]")].find(
        (row) => row.getBoundingClientRect().bottom > top + 1,
      )!;
      return {
        id: row.dataset.row,
        offset: row.getBoundingClientRect().top - top,
      };
    });
  await pane.evaluate((el) => {
    el.scrollTop -= 700;
    el.dispatchEvent(new Event("reading-move"));
  });
  await expect(
    page.getByRole("button", { name: "Latest", exact: true }),
  ).toBeVisible();
  await page.waitForTimeout(250);
  const before = await anchor();
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(
    page.getByRole("heading", { name: "Preview ready", exact: true }),
  ).toBeVisible();
  expect(await page.locator(".transcript").count()).toBe(1);
  expect(await page.locator("[data-row]").count()).toBeLessThan(60);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(pane).toHaveAttribute("aria-busy", "false");
  await expect.poll(async () => (await anchor()).id).toBe(before.id);
  await expect
    .poll(async () => (await anchor()).offset)
    .toBeCloseTo(before.offset, 0);
  await expect(
    page.getByRole("button", { name: "Latest", exact: true }),
  ).toBeVisible();
});
