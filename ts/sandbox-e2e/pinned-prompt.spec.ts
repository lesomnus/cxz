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

test("latest preceding input remains pinned above fades and jumps to its virtual row", async ({
  page,
}) => {
  await openLong(page);
  const pane = page.locator(".transcript");
  const pinned = page.getByRole("button", { name: "Jump to user message" });
  await expect(pinned).toBeVisible();
  await expect(pinned).toContainText("Review history batch 53.");
  const seq = (await page
    .locator(".pinned-prompt")
    .getAttribute("data-pinned-seq"))!;
  await expect(pane.locator(`[data-row="${seq}"]`)).toHaveCount(0);
  const areaTop = (await pane.boundingBox())!.y;
  expect((await pinned.boundingBox())!.y).toBeCloseTo(areaTop, 0);
  await pane.evaluate((el) => (el.scrollTop -= 200));
  await expect
    .poll(() => pinned.boundingBox().then((box) => box!.y))
    .toBeCloseTo(areaTop, 0);
  const belowFade = await pinned.evaluate((el) => {
    const fade = document.querySelector<HTMLElement>(".transcript-fade-top")!;
    fade.style.pointerEvents = "auto";
    const box = el.getBoundingClientRect();
    const above = el.contains(
      document.elementFromPoint(box.left + 25, box.top + 25),
    );
    fade.style.pointerEvents = "none";
    return above;
  });
  expect(belowFade).toBe(true);
  await pinned.click();
  const original = pane.locator(`article.input[data-seq="${seq}"]`);
  await expect(original).toBeVisible();
  await expect
    .poll(async () =>
      Math.abs(
        (await original.boundingBox())!.y - (await pane.boundingBox())!.y,
      ),
    )
    .toBeLessThan(1);
  await pane.evaluate((el) => (el.scrollTop += 200));
  await expect(pinned).toContainText("Review history batch 53.");
  await pane.evaluate((el) => (el.scrollTop -= 1200));
  await expect
    .poll(async () =>
      BigInt(
        (await page.locator(".pinned-prompt").getAttribute("data-pinned-seq"))!,
      ),
    )
    .toBeLessThan(BigInt(seq));
  await pinned.focus();
  const previous = (await page
    .locator(".pinned-prompt")
    .getAttribute("data-pinned-seq"))!;
  await page.keyboard.press("Enter");
  await expect(
    pane.locator(`article.input[data-seq="${previous}"]`),
  ).toBeVisible();
  expect(await pane.locator("[data-row]").count()).toBeLessThan(40);
});

test("an input preceding the cached window is found and can be loaded on demand", async ({
  page,
}) => {
  await openLong(page);
  const pane = page.locator(".transcript");
  // Set up a cache-head viewport before ordinary edge paging. Only suppress the
  // setup's scroll event; the lookup and click then use the real WASM service.
  await pane.evaluate(async (el) => {
    const pause = (event: Event) => {
      if (event.target === el) event.stopImmediatePropagation();
    };
    document.addEventListener("scroll", pause, true);
    el.dispatchEvent(
      new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: -1 }),
    );
    el.scrollTop = 12;
    el.dispatchEvent(new Event("reading-move"));
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    );
    document.removeEventListener("scroll", pause, true);
  });
  const first = BigInt(
    (await page.locator(".virtual-messages").getAttribute("data-first"))!,
  );
  const pinned = page.getByRole("button", { name: "Jump to user message" });
  await expect(pinned).toBeVisible();
  const seq = BigInt(
    (await page.locator(".pinned-prompt").getAttribute("data-pinned-seq"))!,
  );
  expect(seq).toBeLessThan(first);
  await pinned.click();
  await expect(pane.locator(`article.input[data-seq="${seq}"]`)).toBeVisible();
  expect(
    Number(await page.locator(".virtual-messages").getAttribute("data-cached")),
  ).toBeLessThanOrEqual(512);
});

test("a long pinned input stays bounded and its own wheel does not move the transcript", async ({
  page,
}) => {
  await openLong(page);
  const text = Array.from(
    { length: 100 },
    (_, i) => `Pinned input line ${i + 1}`,
  ).join("\n");
  const message = page.getByRole("textbox", { name: "Message", exact: true });
  await message.fill(text);
  await message.press("Control+Enter");
  const pinned = page.getByRole("button", { name: "Jump to user message" });
  await expect(pinned).toContainText("Pinned input line 100");
  await expect(page.locator(".conversation header small")).toContainText(
    "idle",
  );
  const pane = page.locator(".transcript");
  const box = (await pinned.boundingBox())!,
    area = (await pane.boundingBox())!;
  expect(box.height).toBeLessThanOrEqual(Math.min(180, area.height * 0.25) + 1);
  const top = await pane.evaluate((el) => el.scrollTop);
  await pinned.hover();
  await page.mouse.wheel(0, 200);
  await expect
    .poll(() => pinned.evaluate((el) => el.scrollTop))
    .toBeGreaterThan(50);
  expect(await pane.evaluate((el) => el.scrollTop)).toBe(top);
  await page.screenshot({
    path: "test-results/sandbox-pinned-input.png",
    fullPage: true,
  });
});
