import { test, expect, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

async function ready(page: import("@playwright/test").Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
}

test("live additions glide existing rows and reveal new cards faster without changing virtual heights", async ({
  page,
}) => {
  await ready(page);
  expect(await page.locator("[data-live-arrival]").count()).toBe(0);
  await page.evaluate(() => {
    const result = { shifts: [] as any[], entrances: [] as any[] };
    (window as any).transcriptMotionProbe = result;
    const original = Element.prototype.animate;
    Element.prototype.animate = function (...args) {
      const animation = original.apply(this, args);
      if (this instanceof HTMLElement && this.dataset.row) {
        const frames = (animation.effect as KeyframeEffect).getKeyframes();
        const duration = Number(animation.effect!.getTiming().duration);
        const time = animation.currentTime;
        animation.pause();
        animation.currentTime = duration / 4;
        const initialY = new DOMMatrix(String(frames[0].transform)).m42;
        const quarterY = new DOMMatrix(getComputedStyle(this).transform).m42;
        result.shifts.push({
          frames,
          duration,
          quarterProgress: 1 - quarterY / initialY,
          height: this.offsetHeight,
          paintedHeight: this.getBoundingClientRect().height,
        });
        animation.currentTime = time;
        animation.play();
      }
      return animation;
    };
    new MutationObserver((records) => {
      for (const record of records) {
        const row = record.target as HTMLElement;
        if (!row.hasAttribute("data-live-arrival")) continue;
        const card = row.firstElementChild!;
        const animation = card
          .getAnimations()
          .find(
            (a) =>
              a instanceof CSSAnimation &&
              a.animationName === "transcript-enter",
          );
        if (!animation) continue;
        const time = animation.currentTime;
        animation.pause();
        animation.currentTime =
          Number(animation.effect!.getTiming().duration) / 4;
        const quarterOpacity = Number(getComputedStyle(card).opacity);
        animation.currentTime =
          Number(animation.effect!.getTiming().duration) / 2;
        const style = getComputedStyle(card),
          matrix = new DOMMatrix(style.transform);
        result.entrances.push({
          duration: animation.effect!.getTiming().duration,
          quarterOpacity,
          y: matrix.m42,
          scale: matrix.a,
          opacity: Number(style.opacity),
          height: row.offsetHeight,
          paintedHeight: row.getBoundingClientRect().height,
          event: card.className,
          blur: style.backdropFilter,
        });
        animation.currentTime = time;
        animation.play();
      }
    }).observe(document.querySelector(".virtual-messages")!, {
      subtree: true,
      attributes: true,
      attributeFilter: ["data-live-arrival"],
    });
  });
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .fill("Review this workspace");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).transcriptMotionProbe.entrances.filter((e: any) =>
            e.event.includes("event-detail"),
          ).length,
      ),
    )
    .toBeGreaterThan(0);
  const result = await page.evaluate(
    () => (window as any).transcriptMotionProbe,
  );
  expect(result.shifts.length).toBeGreaterThan(0);
  const entry = result.entrances.find((e: any) =>
    e.event.includes("event-detail"),
  );
  expect(entry.y).toBeGreaterThan(0);
  expect(entry.scale).toBeLessThan(1);
  expect(entry.opacity).toBeGreaterThan(0);
  expect(entry.opacity).toBeLessThan(1);
  expect(entry.blur).toBe("none");
  expect(entry.duration).toBe(300);
  expect(result.shifts[0].duration).toBe(500);
  // Both motions should still have most of their travel ahead after a quarter
  // of their duration, instead of rushing through it in the first frames.
  expect(entry.quarterOpacity).toBeGreaterThan(0.1);
  expect(entry.quarterOpacity).toBeLessThan(0.4);
  expect(result.shifts[0].quarterProgress).toBeGreaterThan(0.1);
  expect(result.shifts[0].quarterProgress).toBeLessThan(0.4);
  expect(entry.duration).toBeLessThan(result.shifts[0].duration);
  for (const sample of [...result.shifts, ...result.entrances])
    expect(sample.paintedHeight).toBeCloseTo(sample.height, 0);
  expect(
    result.shifts.some((s: any) => s.frames[0].transform !== "translateY(0px)"),
  ).toBe(true);
  await expect(page.locator("[data-live-arrival]")).toHaveCount(0);
  await expect
    .poll(() =>
      page
        .locator(".transcript")
        .evaluate((el) =>
          Math.abs(el.scrollHeight - el.clientHeight - el.scrollTop),
        ),
    )
    .toBeLessThan(1);
  const latest = page.locator('.transcript-row[data-latest-activity="true"]');
  await expect(latest).toHaveCount(1);
  await expect(latest.locator(".activity-card-face")).toHaveCSS(
    "transform",
    "none",
  );
  const previous = await latest.getAttribute("data-row");
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .fill("Continue the review");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(latest).not.toHaveAttribute("data-row", previous!);
  await expect(latest.locator(".activity-card-face")).toHaveCSS(
    "transform",
    "none",
  );
  const earlier = page.locator(`.transcript-row[data-row="${previous}"]`);
  await expect(earlier).toHaveAttribute("data-latest-activity", "false");
  await expect
    .poll(() =>
      earlier
        .locator(".activity-card-face")
        .evaluate((el) => new DOMMatrix(getComputedStyle(el).transform).m23),
    )
    .toBeLessThan(0);
});

test("reading history and reduced motion do not replay arrivals or displace the reading anchor", async ({
  page,
}) => {
  await ready(page);
  const pane = page.locator(".transcript");
  await pane.evaluate((el) => {
    el.scrollTop -= 800;
  });
  await expect(
    page.getByRole("button", { name: "Latest", exact: true }),
  ).toBeVisible();
  expect(await page.locator("[data-live-arrival]").count()).toBe(0);
  const position = await pane.evaluate((el) => el.scrollTop);
  await page.waitForTimeout(400);
  expect(await pane.evaluate((el) => el.scrollTop)).toBeCloseTo(position, 0);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.getByRole("button", { name: "Latest", exact: true }).click();
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .fill("Reduced motion arrival");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    "Reduced motion arrival",
  );
  expect(await page.locator("[data-live-arrival]").count()).toBe(0);
  expect(
    await pane.evaluate(
      (el) =>
        [...el.querySelectorAll("[data-row]")].flatMap((row) =>
          row.getAnimations(),
        ).length,
    ),
  ).toBe(0);
});
