import { expect, test } from "@playwright/test";

test("selecting activity dates fades daily statistic decimal roles instead of snapping", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-cache--default&viewMode=story");
  const work = page.locator('[data-account="work"]');
  const graph = work.getByRole("region", {
    name: "Daily activity",
    exact: true,
  });
  await expect(graph).toHaveAttribute("aria-busy", "false");
  await expect(work.locator("[data-changing]")).toHaveCount(0);
  await page.clock.install();
  const now = await page.evaluate(() => Date.now());
  await page.clock.pauseAt(new Date(now + 50));
  const daily = work.getByRole("region", {
    name: "Daily statistics",
    exact: true,
  });
  const input = daily.locator("dd").first();
  const cell = input.locator("[data-motion][aria-label] > span > span").nth(1);
  const select = async (date: string) =>
    graph
      .locator(`[data-date="${date}"] button`)
      .evaluate((node: HTMLButtonElement) => node.click());
  await select("2026-10-24");
  await page.clock.runFor(2000);
  await expect(
    input.getByRole("img", { name: "1600", exact: true }),
  ).toHaveText("1.60K");
  for (const [date, raw, formatted] of [
    ["2026-10-31", "800", "0800·"],
    ["2026-10-24", "1600", "1.60K"],
  ]) {
    const before = await cell.evaluate((node) => getComputedStyle(node).color);
    await select(date);
    await expect(
      input.getByRole("img", { name: raw, exact: true }),
    ).toBeAttached();
    const samples = await cell.evaluate((node) => {
      const animation = node
        .getAnimations()
        .find((item) =>
          (item.effect as KeyframeEffect)
            .getKeyframes()
            .some((frame) => frame.color !== undefined),
        )!;
      animation.pause();
      const timing = animation.effect!.getTiming();
      return [0, 0.5, 1].map((fraction) => {
        animation.currentTime = fraction
          ? Number(timing.delay) + Number(timing.duration) * fraction
          : 0;
        return getComputedStyle(node).color;
      });
    });
    expect(samples[0]).toBe(before);
    expect(samples[1]).not.toBe(samples[0]);
    expect(samples[1]).not.toBe(samples[2]);
    await page.clock.runFor(2000);
    await expect(input.getByRole("img", { name: raw, exact: true })).toHaveText(
      formatted,
    );
    await expect(cell).toHaveCSS("color", samples[2]);
  }
});
