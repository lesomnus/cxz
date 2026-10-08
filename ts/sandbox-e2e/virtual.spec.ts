import { test, expect } from "@playwright/test";
test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

test("virtual messages preserve small tail scrolls and anchors across resize", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  const pane = page.locator(".transcript");
  const remaining = () =>
    pane.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight);
  await expect.poll(remaining).toBeLessThan(1);
  expect(await pane.locator("[data-seq]").count()).toBeLessThan(40);
  expect(
    Number(await page.locator(".virtual-messages").getAttribute("data-cached")),
  ).toBeLessThanOrEqual(1024);
  await pane.evaluate((el) => (el.scrollTop -= 8));
  // A near-bottom scroll must remain where the user put it, through row measurement.
  await page.waitForTimeout(400);
  expect(await remaining()).toBeCloseTo(8, 0);
  await pane.evaluate((el) => (el.scrollTop += 4));
  await page.waitForTimeout(400);
  expect(await remaining()).toBeCloseTo(4, 0);
  const fade = page.locator(".transcript-fade-bottom");
  expect(
    Number(await fade.evaluate((el) => getComputedStyle(el).opacity)),
  ).toBeLessThan(0.01);
  await pane.evaluate((el) => (el.scrollTop -= 1800));
  await expect
    .poll(async () =>
      Number(await fade.evaluate((el) => getComputedStyle(el).opacity)),
    )
    .toBe(1);
  const fadeBox = (await fade.boundingBox())!;
  const wrapperBox = (await page.locator(".composer-wrapper").boundingBox())!;
  expect(fadeBox.y + fadeBox.height).toBeLessThanOrEqual(wrapperBox.y);
  await expect(page.locator(".transcript-fade-top")).toHaveCSS("opacity", "1");
  const anchor = await pane.evaluate((el) => {
    const top = el.getBoundingClientRect().top;
    const row = [...el.querySelectorAll<HTMLElement>("[data-row]")].find(
      (node) => node.getBoundingClientRect().bottom > top,
    )!;
    return {
      id: row.dataset.row!,
      offset: row.getBoundingClientRect().top - top,
    };
  });
  await page.setViewportSize({ width: 900, height: 1000 });
  await expect
    .poll(async () =>
      pane.evaluate((el, saved) => {
        const row = el.querySelector(`[data-row="${saved.id}"]`)!;
        return Math.abs(
          row.getBoundingClientRect().top -
            el.getBoundingClientRect().top -
            saved.offset,
        );
      }, anchor),
    )
    .toBeLessThan(1);
  expect(await pane.locator("[data-seq]").count()).toBeLessThan(40);
  await page.getByRole("button", { name: "Latest", exact: true }).click();
  await expect.poll(remaining).toBeLessThan(1);
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("pages through an entire history in both directions with bounded DOM and cache", async ({
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
  const pane = page.locator(".transcript"),
    messages = page.locator(".virtual-messages");
  const first = async () =>
    BigInt((await messages.getAttribute("data-first"))!);
  const last = async () => BigInt((await messages.getAttribute("data-last"))!);
  const end = await last();
  for (let pageIndex = 0; pageIndex < 20 && (await first()) > 5n; pageIndex++) {
    const before = await first();
    await pane.evaluate((el) => (el.scrollTop = 0));
    await expect.poll(first).toBeLessThan(before);
    expect(
      Number(await messages.getAttribute("data-cached")),
    ).toBeLessThanOrEqual(8192);
    expect(await pane.locator("[data-seq]").count()).toBeLessThan(50);
  }
  await pane.evaluate((el) => (el.scrollTop = 0));
  await expect(
    page.getByText("History item 0001", { exact: false }),
  ).toBeVisible();
  await expect(page.locator(".transcript-fade-top")).toHaveCSS("opacity", "0");
  for (let pageIndex = 0; pageIndex < 20 && (await last()) < end; pageIndex++) {
    const before = await last();
    await pane.evaluate((el) => (el.scrollTop = el.scrollHeight));
    await expect.poll(last).toBeGreaterThan(before);
    expect(
      Number(await messages.getAttribute("data-cached")),
    ).toBeLessThanOrEqual(8192);
    expect(await pane.locator("[data-seq]").count()).toBeLessThan(50);
  }
  await pane.evaluate((el) => (el.scrollTop = el.scrollHeight));
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
});
