import { test, expect } from "@playwright/test";
test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

test("both pinned edges advance prompt phase and extend the background fade with tension", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  const pane = page.locator(".transcript");
  const thumb = page.getByRole("scrollbar", { name: "Conversation scroll" });
  // Start far from actual retained-history ends so both local edges can stream past.
  await pane.evaluate(
    (el) => (el.scrollTop = (el.scrollHeight - el.clientHeight) / 2),
  );
  const fade = () =>
    page
      .locator(".transcript-fade-bottom")
      .evaluate((el) => parseFloat(getComputedStyle(el).height));
  await expect.poll(fade).toBeGreaterThan(100);
  const baseline = await fade();
  for (const direction of [-1, 1]) {
    await expect
      .poll(() =>
        thumb.evaluate((el) =>
          Math.abs(
            parseFloat(getComputedStyle(el).top) - Number(el.dataset.targetTop),
          ),
        ),
      )
      .toBeLessThan(0.1);
    const bounds = (await thumb.boundingBox())!;
    const rail = (await page.locator(".scroll-track").boundingBox())!;
    await page.mouse.move(
      bounds.x + bounds.width / 2,
      bounds.y + bounds.height / 2,
    );
    await page.mouse.down();
    const x = bounds.x + bounds.width / 2;
    const edge =
      direction < 0
        ? rail.y + bounds.height / 2
        : rail.y + rail.height - bounds.height / 2;
    await page.mouse.move(x, edge + direction * 50);
    await expect
      .poll(async () => Number(await thumb.getAttribute("data-stretch")))
      .toBeGreaterThan(5);
    await expect.poll(fade).toBeGreaterThan(baseline + 20);
    const light = await fade();
    const topFade = () =>
      page
        .locator(".transcript-fade-top")
        .evaluate((el) => parseFloat(getComputedStyle(el).height));
    const topLight = await topFade();
    await page.mouse.move(x, edge + direction * 160);
    await expect
      .poll(async () => Number(await thumb.getAttribute("data-stretch")))
      .toBeGreaterThan(15);
    await expect.poll(fade).toBeGreaterThan(light + 15);
    const area = (await page.locator(".transcript-area").boundingBox())!;
    const stretched = (await thumb.boundingBox())!;
    expect(stretched.y).toBeGreaterThanOrEqual(area.y);
    expect(stretched.y + stretched.height).toBeLessThanOrEqual(
      area.y + area.height,
    );
    expect(
      await page
        .locator(".scroll-track")
        .evaluate((el) => Number(getComputedStyle(el).zIndex)),
    ).toBeGreaterThan(
      await page
        .locator(".transcript-fade-bottom")
        .evaluate((el) => Number(getComputedStyle(el).zIndex)),
    );
    if (direction > 0) {
      await expect.poll(topFade).toBeGreaterThan(topLight + 5);
      await expect
        .poll(async () => (await topFade()) - (await fade()))
        .toBeGreaterThan(15);
    }
    const phase = await page.evaluate(() => {
      const marker = document.querySelector<HTMLElement>(".scroll-marker")!;
      return {
        id: marker.dataset.prompt!,
        marker: parseFloat(marker.style.top),
        scroll: document.querySelector(".transcript")!.scrollTop,
      };
    });
    await expect
      .poll(() =>
        page
          .locator(`[data-prompt="${phase.id}"]`)
          .evaluate(
            (el, before) => parseFloat(el.style.top) - before,
            phase.marker,
          )
          .then((delta) => -direction * delta),
      )
      .toBeGreaterThan(5);
    const moved = await pane.evaluate((el) => el.scrollTop);
    expect(direction * (moved - phase.scroll)).toBeGreaterThan(50);
    await page.mouse.up();
    await expect(thumb).toHaveAttribute("data-stretch", "0.00");
    await expect.poll(fade).toBeCloseTo(baseline, 0);
  }
  await pane.evaluate((el) => (el.scrollTop -= 120));
  await page.getByRole("button", { name: "Latest", exact: true }).click();
  await expect.poll(fade).toBe(0);
});
