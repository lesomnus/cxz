import { expect, test, type Locator } from "@playwright/test";

const offset = (node: Locator) =>
  node.evaluate(
    (element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).m42,
  );

for (const theme of ["dark", "light"]) {
  test(`quota preserves the original tube and a concave meniscus in ${theme} mode`, async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(
      `/iframe.html?id=accounts-quota--comparison&viewMode=story&globals=theme:${theme}`,
    );
    for (const value of [64, 90, 95, 98, 99, 100]) {
      const figure = page.getByRole("figure", {
        name: `${value}% remaining`,
        exact: true,
      });
      const meter = figure.getByRole("meter");
      await expect(meter).toHaveAttribute("aria-valuenow", String(value));
      const gauge = meter.locator("..");
      await expect(gauge).toHaveCSS("border-radius", "14px");
      const vessel = gauge.locator("[data-quota-vessel]");
      const cap = vessel.locator("[data-quota-empty]");
      await expect.poll(() => offset(cap)).toBeCloseTo((-58 * value) / 100, 2);
      await expect(cap).toHaveCSS("border-radius", "13px");
      const box = (await cap.boundingBox())!;
      expect(box.width).toBe(30);
      expect(box.height).toBe(58);
      if (value === 64 || value === 90) {
        await meter.evaluate((element) => {
          (element as HTMLElement).style.pointerEvents = "none";
        });
        const shape = await cap.evaluate((element) => {
          const box = element.getBoundingClientRect();
          const y = box.bottom - 0.5;
          const center = document.elementFromPoint(box.x + box.width / 2, y);
          const left = document.elementFromPoint(box.x + 6, y);
          const right = document.elementFromPoint(box.right - 6, y);
          const vessel = element.parentElement!;
          return {
            centerEmpty: center === element,
            leftLiquid: left === vessel,
            rightLiquid: right === vessel,
          };
        });
        expect(shape).toEqual({
          centerEmpty: true,
          leftLiquid: true,
          rightLiquid: true,
        });
      }
    }
    const exhausted = page.getByRole("figure", {
      name: "0% remaining",
      exact: true,
    });
    await expect(exhausted.getByRole("meter")).toHaveAttribute(
      "aria-valuenow",
      "0",
    );
    expect(await offset(exhausted.locator("[data-quota-empty]"))).toBe(0);
    const unknown = page.getByRole("figure", {
      name: "Not reported",
      exact: true,
    });
    await expect(unknown.getByRole("meter")).toHaveCount(0);
    await expect(
      unknown.getByRole("img", { name: "Not reported", exact: true }),
    ).toBeVisible();
    expect(await offset(unknown.locator("[data-quota-empty]"))).toBe(-58);
    await page.screenshot({ path: `/tmp/account-quota-meniscus-${theme}.png` });
  });
}

test("changing quota translates the same empty cap without deforming the meniscus", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-quota--water-level&viewMode=story");
  const quota = page.getByRole("region", {
    name: "Remaining agent usage",
    exact: true,
  });
  const meter = quota.getByRole("meter");
  await expect(meter).toHaveAttribute("aria-valuenow", "95");
  const cap = quota.locator("[data-quota-empty]");
  await expect.poll(() => offset(cap)).toBeCloseTo(-55.1, 2);
  const original = await cap.elementHandle();
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
  await expect.poll(() => offset(cap)).toBeGreaterThan(-55);
  expect(await offset(cap)).toBeLessThan(0);
  await expect.poll(() => offset(cap)).toBeCloseTo(0, 2);
  await slider.press("End");
  await expect(meter).toHaveAttribute("aria-valuenow", "100");
  await expect.poll(() => offset(cap)).toBeCloseTo(-58, 2);
  expect(await original!.evaluate((element) => element.isConnected)).toBe(true);
  expect(await quota.boundingBox()).toEqual(before);
  expect((await cap.boundingBox())!.height).toBe(58);
});

test("reduced motion updates liquid level immediately", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=accounts-quota--water-level&viewMode=story");
  const meter = page.getByRole("meter");
  await expect(meter).toHaveAttribute("aria-valuenow", "95");
  const cap = page.locator("[data-quota-empty]");
  const slider = page.getByRole("slider", {
    name: "Remaining usage",
    exact: true,
  });
  await slider.focus();
  await slider.press("Home");
  await expect(meter).toHaveAttribute("aria-valuenow", "0");
  expect(await offset(cap)).toBe(0);
  expect(await cap.evaluate((element) => element.getAnimations().length)).toBe(
    0,
  );
});
