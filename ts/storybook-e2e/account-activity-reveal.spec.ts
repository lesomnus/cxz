import { expect, test, type Page } from "@playwright/test";
async function open(page: Page) {
  await page.clock.setFixedTime(new Date("2026-11-01T18:00:00Z"));
  await page.addInitScript(() => {
    Reflect.set(window, "activityReveals", []);
    const animate = Element.prototype.animate;
    Element.prototype.animate = function (frames, options) {
      if (this.matches("[data-date] > button[data-level]")) {
        const root = this.closest('[aria-label="Daily activity"]')!;
        const origin = root
          .querySelector('[data-date="2026-11-01"]')!
          .getBoundingClientRect();
        const bounds = this.getBoundingClientRect();
        Reflect.get(window, "activityReveals").push({
          date: this.parentElement!.getAttribute("data-date"),
          account: this.closest("[data-account]")?.getAttribute("data-account"),
          level: Number(this.getAttribute("data-level")),
          distance: Math.hypot(
            bounds.x + bounds.width / 2 - origin.x - origin.width / 2,
            bounds.y + bounds.height / 2 - origin.y - origin.height / 2,
          ),
          delay: typeof options === "object" ? Number(options?.delay) : 0,
        });
      }
      return animate.call(this, frames, options);
    };
  });
  await page.goto(
    "/iframe.html?id=accounts-cache--slow-refresh&viewMode=story",
  );
  const activity = page
    .locator('[data-account="work"]')
    .getByRole("region", { name: "Daily activity", exact: true });
  await expect(activity).toHaveAttribute("aria-busy", "false");
  return activity;
}
const traces = (page: Page) =>
  page.evaluate(
    () =>
      Reflect.get(window, "activityReveals") as {
        date: string;
        account: string;
        level: number;
        distance: number;
        delay: number;
      }[],
  );
test("account values reveal by radial distance from today, while cached and unchanged values stay stable", async ({
  page,
}) => {
  const activity = await open(page);
  const rows = (await traces(page)).filter((row) => row.account === "work");
  expect(rows.length).toBeGreaterThan(3);
  expect(rows.every((row) => row.level > 0)).toBe(true);
  expect(rows.find((row) => row.date === "2026-11-01")!.delay).toBe(0);
  const outward = [...rows].sort((a, b) => a.distance - b.distance);
  for (let index = 1; index < outward.length; index++) {
    expect(outward[index].delay).toBeGreaterThan(0);
    expect(outward[index].delay).toBeGreaterThanOrEqual(
      outward[index - 1].delay,
    );
  }
  const count = (await traces(page)).length;
  await page
    .getByRole("button", { name: "Leave accounts", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Open accounts", exact: true })
    .click();
  await expect(activity).toHaveAttribute("aria-busy", "false");
  expect((await traces(page)).length).toBe(count);
  await page
    .locator('[data-account="work"]')
    .getByRole("button", { name: /^Refresh account / })
    .click();
  await expect(activity).toHaveAttribute("aria-busy", "true");
  await expect(activity).toHaveAttribute("aria-busy", "false");
  expect((await traces(page)).length).toBe(count);
});
test("reduced motion shows loaded account values immediately", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const activity = await open(page);
  expect((await traces(page)).length).toBe(0);
  await expect(
    activity.locator('button[data-level="4"]').first(),
  ).toBeVisible();
});
