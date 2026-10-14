import { expect, test } from "@playwright/test";

test("standalone scroll area supplies its own style, centered rail, hover, drag and keyboard scrolling", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-scrollarea--short-overflow&viewMode=story",
  );
  const area = page.locator('[data-scroll-area="root"]'),
    viewport = area.locator('[data-scroll-area="viewport"]');
  const rail = area.locator('[data-scroll-area="rail"]'),
    handle = area.locator('[data-scroll-area="handle"]');
  const thumb = area.getByRole("scrollbar", { name: "Scroll entries" });
  await expect(thumb).toBeVisible();
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue("--space"),
    ),
  ).toBe("");
  const rootBox = (await area.boundingBox())!,
    railBox = (await rail.boundingBox())!,
    thumbBox = (await thumb.boundingBox())!;
  expect(railBox.height).toBeLessThan(rootBox.height);
  expect(railBox.y + railBox.height / 2).toBeCloseTo(
    rootBox.y + rootBox.height / 2,
    0,
  );
  expect(thumbBox.height).toBeLessThan(railBox.height / 2);
  await page.mouse.move(0, 0);
  await expect(handle).toHaveCSS("opacity", "0");
  await area.hover();
  await expect(handle).toHaveCSS("opacity", "0.7");
  await expect
    .poll(() => rail.evaluate((el) => getComputedStyle(el, "::before").opacity))
    .toBe("1");
  await page.mouse.move(
    thumbBox.x + thumbBox.width / 2,
    thumbBox.y + thumbBox.height / 2,
  );
  await expect(handle).toHaveCSS("opacity", "1");
  await page.mouse.down();
  await page.mouse.move(
    thumbBox.x + thumbBox.width / 2,
    railBox.y + railBox.height,
  );
  await page.mouse.up();
  await expect
    .poll(() =>
      viewport.evaluate(
        (el) => el.scrollHeight - el.clientHeight - el.scrollTop,
      ),
    )
    .toBeLessThan(1);
  await thumb.focus();
  await thumb.press("Home");
  await expect.poll(() => viewport.evaluate((el) => el.scrollTop)).toBe(0);
  await thumb.press("End");
  await expect
    .poll(() =>
      viewport.evaluate(
        (el) => el.scrollHeight - el.clientHeight - el.scrollTop,
      ),
    )
    .toBeLessThan(1);
  await page.goto(
    "/iframe.html?id=components-scrollarea--no-overflow&viewMode=story",
  );
  await expect(page.getByRole("scrollbar")).toHaveCount(0);
});

test("standalone content growth keeps the rail centered and viewportRef/onScroll work without app services", async ({
  page,
}) => {
  const api: string[] = [];
  page.on("request", (request) => {
    if (/\/cxz\./.test(request.url())) api.push(request.url());
  });
  await page.goto(
    "/iframe.html?id=components-scrollarea--growing-content&viewMode=story",
  );
  const area = page.locator('[data-scroll-area="root"]'),
    viewport = area.locator('[data-scroll-area="viewport"]'),
    rail = area.locator('[data-scroll-area="rail"]');
  const handle = area.locator('[data-scroll-area="handle"]');
  await expect(area.getByRole("scrollbar")).toBeVisible();
  const initial = (await rail.boundingBox())!.height;
  const add = page.getByRole("button", { name: "Add entries" });
  await add.hover();
  await expect(handle).toHaveCSS("opacity", "0");
  await add.click();
  await expect
    .poll(async () => (await rail.boundingBox())!.height)
    .toBeGreaterThan(initial);
  const outer = (await area.boundingBox())!,
    grown = (await rail.boundingBox())!;
  expect(grown.y + grown.height / 2).toBeCloseTo(outer.y + outer.height / 2, 0);
  await page.getByRole("button", { name: "Scroll to bottom" }).click();
  await expect
    .poll(() =>
      viewport.evaluate(
        (el) => el.scrollHeight - el.clientHeight - el.scrollTop,
      ),
    )
    .toBeLessThan(1);
  await expect
    .poll(() => page.getByLabel("Scroll position").textContent())
    .not.toBe("0");
  expect(api).toEqual([]);
});
