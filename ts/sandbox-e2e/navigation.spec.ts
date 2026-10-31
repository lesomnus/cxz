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

test("outward wheel gestures never flash Latest without moving", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const pane = page.locator(".transcript");
  expect(await pane.evaluate((el) => el.scrollHeight - el.clientHeight)).toBe(
    0,
  );
  const flashed = async (deltas: number[]) =>
    pane.evaluate(async (el, deltas) => {
      let flashes = 0;
      const slot = document.querySelector(".latest-slot")!;
      const observer = new MutationObserver((records) => {
        for (const record of records)
          if (
            record.oldValue === "true" ||
            slot.getAttribute("data-visible") === "true"
          )
            flashes++;
      });
      observer.observe(slot, {
        attributes: true,
        attributeFilter: ["data-visible"],
        attributeOldValue: true,
      });
      for (const deltaY of deltas) {
        el.dispatchEvent(
          new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY }),
        );
        for (let frame = 0; frame < 8; frame++)
          await new Promise<void>((resolve) =>
            requestAnimationFrame(() => resolve()),
          );
      }
      observer.disconnect();
      return flashes;
    }, deltas);
  expect(await flashed([300, -300, 8, -8])).toBe(0);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  expect(await flashed([300, 300, 8, 3000])).toBe(0);
  await expect(
    page.getByRole("button", { name: "Latest", exact: true }),
  ).toHaveCount(0);
});

test("ordinary movement extends the fade in the direction content leaves", async ({
  page,
}) => {
  await openLong(page);
  const pane = page.locator(".transcript");
  await pane.evaluate(
    (el) => (el.scrollTop = (el.scrollHeight - el.clientHeight) / 2),
  );
  await page.waitForTimeout(900);
  for (const direction of [-1, 1]) {
    const fade = page.locator(
      direction < 0 ? ".transcript-fade-bottom" : ".transcript-fade-top",
    );
    const baseline = await fade.evaluate(
      (el) => el.getBoundingClientRect().height,
    );
    await pane.evaluate(
      (el, deltaY) =>
        el.dispatchEvent(
          new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY }),
        ),
      direction * 300,
    );
    await expect
      .poll(() => fade.evaluate((el) => el.getBoundingClientRect().height), {
        intervals: [16, 32, 50],
      })
      .toBeGreaterThan(baseline + 10);
    await expect
      .poll(() => fade.evaluate((el) => el.getBoundingClientRect().height))
      .toBeLessThan(baseline + 1);
  }
});

test("prompt ticks never reverse through first measurements and scrolling a larger cached window", async ({
  page,
}) => {
  await openLong(page);
  const pane = page.locator(".transcript");
  const result = await pane.evaluate(async (el) => {
    const rail = document.querySelector<HTMLElement>(".scroll-track")!;
    const snapshot = () => ({
      top: el.scrollTop,
      start: Number(
        document.querySelector<HTMLElement>(".scroll-thumb")!.dataset.start,
      ),
      thumb: Number(
        document.querySelector<HTMLElement>(".scroll-thumb")!.dataset.targetTop,
      ),
      first:
        document.querySelector<HTMLElement>(".virtual-messages")!.dataset
          .first!,
      last: document.querySelector<HTMLElement>(".virtual-messages")!.dataset
        .last!,
      markers: new Map(
        [...rail.querySelectorAll<HTMLElement>(".scroll-marker")].map(
          (node) => [
            node.dataset.prompt!,
            {
              target: Number(node.dataset.targetTop),
              displayed: parseFloat(getComputedStyle(node).top),
            },
          ],
        ),
      ),
    });
    const initial = snapshot();
    let worst: unknown = null;
    let reverse = 0,
      checked = 0,
      pages = 0;
    // Cache measurement may start a rebase near the end of a wheel sample.
    // Compare completed positions instead of assuming 200ms finishes it.
    const waitForRebase = async () => {
      const deadline = performance.now() + 2000;
      let settledFrames = 0;
      while (settledFrames < 3) {
        await new Promise(requestAnimationFrame);
        const nodes = [
          document.querySelector<HTMLElement>(".scroll-thumb")!,
          ...rail.querySelectorAll<HTMLElement>(".scroll-marker"),
        ];
        const settled = nodes.every(
          (node) =>
            Math.abs(
              parseFloat(getComputedStyle(node).top) -
                Number(node.dataset.targetTop),
            ) < 0.1,
        );
        settledFrames = settled ? settledFrames + 1 : 0;
        if (performance.now() > deadline)
          throw new Error(
            "Scrollbar rebase did not settle after wheel movement",
          );
      }
    };
    for (const direction of [-1, 1]) {
      for (let step = 0; step < 80; step++) {
        const before = snapshot();
        el.dispatchEvent(
          new WheelEvent("wheel", {
            bubbles: true,
            cancelable: true,
            deltaY: direction * 1200,
          }),
        );
        await new Promise((resolve) => setTimeout(resolve, 200));
        await waitForRebase();
        const after = snapshot();
        if (after.first !== before.first || after.last !== before.last) pages++;
        for (const [id, old] of before.markers) {
          const next = after.markers.get(id);
          if (
            !next ||
            old.target < 0 ||
            old.target > rail.clientHeight ||
            next.target < 0 ||
            next.target > rail.clientHeight
          )
            continue;
          checked++;
          const delta = Math.max(
            direction * (next.target - old.target),
            direction * (next.displayed - old.displayed),
          );
          if (delta > reverse) {
            reverse = delta;
            worst = {
              direction,
              step,
              id,
              old,
              next,
              before: {
                top: before.top,
                start: before.start,
                thumb: before.thumb,
                first: before.first,
                last: before.last,
              },
              after: {
                top: after.top,
                start: after.start,
                thumb: after.thumb,
                first: after.first,
                last: after.last,
              },
            };
          }
        }
      }
    }
    return {
      reverse,
      checked,
      pages,
      initial: initial.first,
      final: snapshot().first,
      worst,
    };
  });
  expect(result.checked).toBeGreaterThan(40);
  // Larger pages can hold this whole scrolling range. Page request counts are
  // not a prerequisite for checking the thumb and prompt phase directions.
  expect(result.reverse, JSON.stringify(result)).toBeLessThan(1);
});
