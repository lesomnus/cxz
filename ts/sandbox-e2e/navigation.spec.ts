import { test, expect } from "@playwright/test";
test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

async function openLong(page: import("@playwright/test").Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
}

test("first elastic drag returns to the rail at the reading position", async ({
  page,
}) => {
  await openLong(page);
  const pane = page.locator(".transcript");
  const visibleSequence = () =>
    pane.evaluate((el) => {
      const top = el.getBoundingClientRect().top;
      const row = [...el.querySelectorAll<HTMLElement>("[data-row]")].find(
        (node) => node.getBoundingClientRect().bottom > top,
      )!;
      return Number(row.dataset.row);
    });
  const initial = await visibleSequence();
  const thumb = page.getByRole("scrollbar", { name: "Conversation scroll" });
  const bounds = (await thumb.boundingBox())!;
  const rail = (await page.locator(".scroll-track").boundingBox())!;
  const x = bounds.x + bounds.width / 2;
  await page.mouse.move(x, bounds.y + bounds.height / 2);
  await page.mouse.down();
  await page.mouse.move(x, rail.y + bounds.height / 2 - 180);
  await expect
    .poll(async () => Number(await thumb.getAttribute("data-stretch")))
    .toBeGreaterThan(15);
  await expect.poll(visibleSequence).toBeLessThan(initial - 200);
  const before = await visibleSequence();
  await page.mouse.move(x, rail.y + bounds.height / 2 + 4);
  await expect(thumb).toHaveAttribute("data-stretch", "0.00");
  const resumed = await visibleSequence();
  expect(Math.abs(resumed - before)).toBeLessThan(30);
  await page.mouse.move(x, rail.y + bounds.height / 2 + 12);
  await page.waitForTimeout(150);
  expect(Math.abs((await visibleSequence()) - resumed)).toBeLessThan(30);
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toHaveCount(0);
  await page.mouse.up();
});

test("wheel movement is quick and monotonic, lands exactly and respects reduced motion", async ({
  page,
}) => {
  await openLong(page);
  const pane = page.locator(".transcript");
  await pane.evaluate((el) => (el.scrollTop = 5000));
  await page.waitForTimeout(150);
  const motion = await pane.evaluate(async (el) => {
    const before = el.scrollTop;
    let shift = 0;
    const shifted = (event: Event) => {
      shift += (event as CustomEvent<number>).detail;
    };
    el.addEventListener("history-shift", shifted);
    el.dispatchEvent(
      new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: 300 }),
    );
    const positions: number[] = [];
    for (let frame = 0; frame < 16; frame++) {
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
      positions.push(el.scrollTop - before - shift);
    }
    el.removeEventListener("history-shift", shifted);
    return positions;
  });
  expect(motion.some((n) => n > 0 && n < 300)).toBe(true);
  expect(motion[3]).toBeGreaterThan(200);
  for (let i = 1; i < motion.length; i++)
    expect(motion[i]).toBeGreaterThanOrEqual(motion[i - 1]);
  expect(motion.at(-1)).toBe(300);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const handled = await pane.evaluate((el) => {
    const event = new WheelEvent("wheel", {
      bubbles: true,
      cancelable: true,
      deltaY: 300,
    });
    el.dispatchEvent(event);
    return event.defaultPrevented;
  });
  expect(handled).toBe(false);
});
