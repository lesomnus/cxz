import { expect, test } from "@playwright/test";

test("rapid activity-date changes retain partially visible statistic digits", async ({
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
  const input = work
    .getByRole("region", { name: "Daily statistics", exact: true })
    .locator("dd")
    .first();
  const select = async (date: string) =>
    graph
      .locator(`[data-date="${date}"] button`)
      .evaluate((node: HTMLButtonElement) => node.click());
  const seek = async (fraction: number) =>
    input.evaluate((node, fraction) => {
      for (const animation of node.getAnimations({ subtree: true })) {
        if (!(animation instanceof CSSAnimation)) continue;
        animation.pause();
        const timing = animation.effect!.getTiming();
        animation.currentTime =
          Number(timing.delay) + Number(timing.duration) * fraction;
      }
    }, fraction);
  const sample = async () =>
    input
      .locator("[data-motion][aria-label] > span > span")
      .evaluateAll((cells) =>
        cells.map((cell) => {
          const bounds = cell.getBoundingClientRect();
          const frames = Array.from(
            cell.querySelectorAll("[data-slide-frame]"),
          );
          return frames.length
            ? frames
                .map((frame) => ({
                  text: frame.textContent,
                  offset:
                    (frame.getBoundingClientRect().top - bounds.top) /
                    bounds.height,
                }))
                .filter(
                  (frame) => frame.offset > -0.9999 && frame.offset < 0.9999,
                )
                .sort((a, b) => a.offset - b.offset)
            : [{ text: cell.firstElementChild!.textContent, offset: 0 }];
        }),
      );
  await select("2026-10-24");
  await page.clock.runFor(2000);
  await select("2026-10-31");
  for (const [date, raw, fraction] of [
    ["2026-10-24", "1600", 0.23],
    ["2026-10-31", "800", 0.31],
    ["2026-10-24", "1600", 0.43],
  ] as const) {
    await seek(fraction);
    const before = await sample();
    expect(before.some((frames) => frames.length === 2)).toBe(true);
    await select(date);
    await expect(
      input.getByRole("img", { name: raw, exact: true }),
    ).toBeAttached();
    await seek(0);
    const after = await sample();
    expect(after.map((frames) => frames.map((frame) => frame.text))).toEqual(
      before.map((frames) => frames.map((frame) => frame.text)),
    );
    after.forEach((frames, index) =>
      frames.forEach((frame, j) =>
        expect(frame.offset).toBeCloseTo(before[index][j].offset, 3),
      ),
    );
  }
  await page.clock.runFor(2000);
  await expect(
    input.getByRole("img", { name: "1600", exact: true }),
  ).toHaveText("1.60K");
  await expect(input.locator("[data-changing]")).toHaveCount(0);
});

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
