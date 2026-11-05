import { expect, test, type Locator } from "@playwright/test";

const offset = (node: Locator) =>
  node.evaluate(
    (element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).m42,
  );

for (const theme of ["dark", "light"]) {
  test(`near-full quota keeps a complete fixed ellipse in ${theme} mode`, async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(
      `/iframe.html?id=accounts-quota--comparison&viewMode=story&globals=theme:${theme}`,
    );
    for (const value of [90, 95, 98, 99, 100]) {
      const figure = page.getByRole("figure", {
        name: `${value}% remaining`,
        exact: true,
      });
      await expect(figure.getByRole("meter")).toHaveAttribute(
        "aria-valuenow",
        String(value),
      );
      const liquid = figure.locator("[data-quota-liquid]");
      await expect
        .poll(() => offset(liquid))
        .toBeCloseTo((58 * (100 - value)) / 100, 2);
      const surface = liquid.locator("[data-quota-surface]");
      const box = (await surface.boundingBox())!;
      expect(box.width).toBe(30);
      expect(box.height).toBe(10);
      const visible = await surface.evaluate((element) => {
        const box = element.getBoundingClientRect();
        return [
          [0.5, 0.1],
          [0.1, 0.5],
          [0.9, 0.5],
          [0.5, 0.9],
        ].every(
          ([x, y]) =>
            document.elementFromPoint(
              box.x + box.width * x!,
              box.y + box.height * y!,
            ) === element,
        );
      });
      expect(visible).toBe(true);
    }
    const exhausted = page.getByRole("figure", {
      name: "0% remaining",
      exact: true,
    });
    await expect(exhausted.getByRole("meter")).toHaveAttribute(
      "aria-valuenow",
      "0",
    );
    await expect(
      exhausted.locator("[data-quota-surface]"),
    ).not.toBeInViewport();
    const unknown = page.getByRole("figure", {
      name: "Not reported",
      exact: true,
    });
    await expect(unknown.getByRole("meter")).toHaveCount(0);
    await expect(
      unknown.getByRole("img", { name: "Not reported", exact: true }),
    ).toBeVisible();
    await expect(unknown.locator("[data-quota-surface]")).not.toBeInViewport();
    await page.screenshot({ path: `/tmp/account-quota-liquid-${theme}.png` });
  });
}

test("changing quota slides the same liquid surface without resizing or remounting it", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-quota--water-level&viewMode=story");
  const quota = page.getByRole("region", {
    name: "Remaining agent usage",
    exact: true,
  });
  const meter = quota.getByRole("meter");
  await expect(meter).toHaveAttribute("aria-valuenow", "95");
  const liquid = quota.locator("[data-quota-liquid]");
  await expect.poll(() => offset(liquid)).toBeCloseTo(2.9, 2);
  const original = await liquid.elementHandle();
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
  await expect.poll(() => offset(liquid)).toBeGreaterThan(3);
  expect(await offset(liquid)).toBeLessThan(58);
  await expect.poll(() => offset(liquid)).toBeCloseTo(58, 2);
  await slider.press("End");
  await expect(meter).toHaveAttribute("aria-valuenow", "100");
  await expect.poll(() => offset(liquid)).toBeCloseTo(0, 2);
  expect(await original!.evaluate((element) => element.isConnected)).toBe(true);
  expect(await quota.boundingBox()).toEqual(before);
  expect(
    (await liquid.locator("[data-quota-surface]").boundingBox())!.height,
  ).toBe(10);
});

test("reduced motion updates liquid level immediately", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=accounts-quota--water-level&viewMode=story");
  const meter = page.getByRole("meter");
  await expect(meter).toHaveAttribute("aria-valuenow", "95");
  const liquid = page.locator("[data-quota-liquid]");
  const slider = page.getByRole("slider", {
    name: "Remaining usage",
    exact: true,
  });
  await slider.focus();
  await slider.press("Home");
  await expect(meter).toHaveAttribute("aria-valuenow", "0");
  expect(await offset(liquid)).toBe(58);
  expect(
    await liquid.evaluate((element) => element.getAnimations().length),
  ).toBe(0);
});
