import { expect, test, type Locator } from "@playwright/test";

const depth = (node: Locator) =>
  node.evaluate((element) => (element as SVGPathElement).getBBox().height);

for (const theme of ["dark", "light"]) {
  test(`quota keeps rounded air-pocket ends and a concave meniscus in ${theme} mode`, async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(
      `/iframe.html?id=accounts-quota--comparison&viewMode=story&globals=theme:${theme}`,
    );
    for (const value of [0, 31, 64, 90, 95, 98, 99, 100]) {
      const figure = page.getByRole("figure", {
        name: `${value}% remaining`,
        exact: true,
      });
      const meter = figure.getByRole("meter");
      await expect(meter).toHaveAttribute("aria-valuenow", String(value));
      const gauge = meter.locator("..");
      await expect(gauge).toHaveCSS("border-radius", "14px");
      const contour = gauge.locator("[data-liquid-contour]");
      await expect
        .poll(() => depth(contour))
        .toBeCloseTo((58 * (100 - value)) / 100, 2);
      await meter.evaluate((element) => {
        (element as HTMLElement).style.pointerEvents = "none";
      });
      if (value === 100) continue;
      // The whole closed outline must fit inside the tube, including its tips.
      const samples = await contour.evaluate((element) => {
        const path = element as SVGPathElement;
        const box = path.getBBox();
        const matrix = path.getScreenCTM()!;
        const length = path.getTotalLength();
        return Array.from({ length: 96 }, (_, index) => {
          const edge = path.getPointAtLength((length * index) / 96);
          const inside = new DOMPoint(
            edge.x * 0.97 + (box.x + box.width / 2) * 0.03,
            edge.y * 0.97 + (box.y + box.height / 2) * 0.03,
          ).matrixTransform(matrix);
          return document.elementFromPoint(inside.x, inside.y) === path;
        });
      });
      expect(samples.every(Boolean), `unclipped outline at ${value}%`).toBe(
        true,
      );
      if (value === 64 || value === 90) {
        const shape = await contour.evaluate((element) => {
          const tube = element.parentElement!.parentElement!;
          const box = tube.getBoundingClientRect();
          const path = element as SVGPathElement;
          const y = box.y + path.getBBox().height - 0.5;
          const left = document.elementFromPoint(box.x + 6, y);
          const right = document.elementFromPoint(box.right - 6, y);
          const liquid = (node: Element | null) =>
            node === tube || node === path.parentElement;
          return {
            centerEmpty:
              document.elementFromPoint(box.x + box.width / 2, y) === path,
            leftLiquid: liquid(left),
            rightLiquid: liquid(right),
          };
        });
        expect(shape).toEqual({
          centerEmpty: true,
          leftLiquid: true,
          rightLiquid: true,
        });
      }
    }
    const unknown = page.getByRole("figure", {
      name: "Not reported",
      exact: true,
    });
    await expect(unknown.getByRole("meter")).toHaveCount(0);
    await expect(
      unknown.getByRole("img", { name: "Not reported", exact: true }),
    ).toBeVisible();
    expect(await depth(unknown.locator("[data-liquid-contour]"))).toBe(0);
    await page.screenshot({ path: `/tmp/account-quota-soft-${theme}.png` });
    const nearFull = page.getByRole("figure", {
      name: "90% remaining",
      exact: true,
    });
    await nearFull.evaluate((element) => {
      (element as HTMLElement).style.cssText =
        "position:fixed;left:32px;top:32px;margin:0;transform:scale(6);transform-origin:top left;z-index:10";
    });
    await nearFull.screenshot({
      path: `/tmp/account-quota-soft-closeup-${theme}.png`,
    });
  });
}

test("changing quota morphs the same soft meniscus without changing the account layout", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-quota--water-level&viewMode=story");
  const quota = page.getByRole("region", {
    name: "Remaining agent usage",
    exact: true,
  });
  const meter = quota.getByRole("meter");
  await expect(meter).toHaveAttribute("aria-valuenow", "95");
  const contour = quota.locator("[data-liquid-contour]");
  await expect.poll(() => depth(contour)).toBeCloseTo(2.9, 2);
  const original = await contour.elementHandle();
  const before = await quota.boundingBox();
  await quota.evaluate((element) =>
    (element as HTMLElement).style.setProperty(
      "--account-quota-motion-duration",
      "1000ms",
    ),
  );
  const slider = page.getByRole("slider", {
    name: "Remaining usage",
    exact: true,
  });
  await slider.focus();
  await slider.press("Home");
  await expect(meter).toHaveAttribute("aria-valuenow", "0");
  await expect.poll(() => depth(contour)).toBeGreaterThan(3);
  expect(await depth(contour)).toBeLessThan(58);
  await expect.poll(() => depth(contour)).toBeCloseTo(58, 2);
  await slider.press("End");
  await expect(meter).toHaveAttribute("aria-valuenow", "100");
  await expect.poll(() => depth(contour)).toBeCloseTo(0, 2);
  expect(await original!.evaluate((element) => element.isConnected)).toBe(true);
  expect(await quota.boundingBox()).toEqual(before);
});

test("reduced motion updates liquid level immediately", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=accounts-quota--water-level&viewMode=story");
  const meter = page.getByRole("meter");
  await expect(meter).toHaveAttribute("aria-valuenow", "95");
  const contour = page.locator("[data-liquid-contour]");
  const slider = page.getByRole("slider", {
    name: "Remaining usage",
    exact: true,
  });
  await slider.focus();
  await slider.press("Home");
  await expect(meter).toHaveAttribute("aria-valuenow", "0");
  expect(await depth(contour)).toBe(58);
  expect(
    await contour.evaluate((element) => element.getAnimations().length),
  ).toBe(0);
});
