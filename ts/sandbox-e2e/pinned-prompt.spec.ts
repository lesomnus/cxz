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

async function reveal(page: import("@playwright/test").Page) {
  const box = (await page.locator(".transcript").boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + 16);
  await expect
    .poll(() =>
      page
        .getByRole("button", { name: "Jump to user message" })
        .boundingBox()
        .then((pinned) => pinned!.y),
    )
    .toBeCloseTo(box.y, 0);
}

test("preceding input peeks below the title without following scroll and reveals on approach", async ({
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
  const area = (await pane.boundingBox())!;
  await page.mouse.move(area.x + area.width / 2, area.y + area.height / 2);
  await expect
    .poll(async () => {
      const box = (await pinned.boundingBox())!;
      return box.y + box.height - area.y;
    })
    .toBeCloseTo(8, 0);
  // Sample every animation frame during actual smoothed wheel motion. Neither
  // the reveal layer nor its visible bottom edge follows the virtual canvas.
  const motion = await pane.evaluate(async (el) => {
    const overlay = document.querySelector<HTMLElement>(".pinned-prompt")!;
    const button = overlay.querySelector("button")!;
    const samples: { overlay: number; bottom: number; scroll: number }[] = [];
    el.dispatchEvent(
      new WheelEvent("wheel", {
        bubbles: true,
        cancelable: true,
        deltaY: -200,
      }),
    );
    for (let i = 0; i < 20; i++) {
      await new Promise(requestAnimationFrame);
      samples.push({
        overlay: overlay.getBoundingClientRect().top,
        bottom: button.getBoundingClientRect().bottom,
        scroll: el.scrollTop,
      });
    }
    return samples;
  });
  expect(motion[0].scroll - motion.at(-1)!.scroll).toBeGreaterThan(20);
  for (const sample of motion) {
    expect(sample.overlay).toBeCloseTo(area.y, 1);
    expect(sample.bottom).toBeCloseTo(area.y + 8, 1);
  }
  expect(
    await pinned.evaluate((el) => {
      const box = el.getBoundingClientRect();
      return !el.contains(
        document.elementFromPoint(
          box.left + 25,
          document.querySelector(".transcript-area")!.getBoundingClientRect()
            .top - 2,
        ),
      );
    }),
  ).toBe(true);
  await reveal(page);
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
  await reveal(page);
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
  await page.mouse.move(10, 10);
  await pinned.focus();
  await expect
    .poll(() => pinned.boundingBox().then((box) => box!.y))
    .toBeCloseTo(area.y, 0);
  const previous = (await page
    .locator(".pinned-prompt")
    .getAttribute("data-pinned-seq"))!;
  await page.keyboard.press("Enter");
  await expect(
    pane.locator(`article.input[data-seq="${previous}"]`),
  ).toBeVisible();
  expect(await pane.locator("[data-row]").count()).toBeLessThan(40);
});

test("the preceding input only peeks after the nearest visible input clears the top", async ({
  page,
}) => {
  await openLong(page);
  const pane = page.locator(".transcript");
  const overlay = page.locator(".pinned-prompt");
  const seq = (await overlay.getAttribute("data-pinned-seq"))!;
  await reveal(page);
  await page.getByRole("button", { name: "Jump to user message" }).click();
  const input = pane.locator(`article.input[data-seq="${seq}"]`);
  await expect(input).toBeVisible();
  await page.mouse.move(10, 10);
  async function gap(pixels: number) {
    await input.evaluate((el, pixels) => {
      const pane = el.closest(".transcript")!;
      pane.scrollTop +=
        el.getBoundingClientRect().top -
        pane.getBoundingClientRect().top -
        pixels;
      pane.dispatchEvent(new Event("reading-move"));
    }, pixels);
    await expect
      .poll(
        async () =>
          (await input.boundingBox())!.y - (await pane.boundingBox())!.y,
      )
      .toBeCloseTo(pixels, 0);
  }
  await gap(12);
  await expect(overlay).toHaveAttribute("data-available", "false");
  await expect(overlay).toHaveCSS("opacity", "0");
  const area = (await pane.boundingBox())!;
  await page.mouse.move(area.x + area.width / 2, area.y + 16);
  expect(
    await overlay.evaluate(
      (el) =>
        !el.contains(
          document.elementFromPoint(
            el.getBoundingClientRect().left + 25,
            el.getBoundingClientRect().top + 16,
          ),
        ),
    ),
  ).toBe(true);
  await page.mouse.move(10, 10);
  // A large distance change must fade, rather than appearing in one frame.
  await gap(129);
  const samples = await overlay.evaluate(async (el) => {
    const opacity: number[] = [];
    for (let i = 0; i < 16; i++) {
      await new Promise(requestAnimationFrame);
      opacity.push(Number(getComputedStyle(el).opacity));
    }
    return opacity;
  });
  expect(samples.some((value) => value > 0 && value < 1)).toBe(true);
  await expect(overlay).toHaveCSS("opacity", "1");
  const button = overlay.locator("button");
  await expect
    .poll(async () => {
      const box = (await button.boundingBox())!;
      return box.y + box.height - area.y;
    })
    .toBeCloseTo(8, 0);
  await gap(112);
  await expect
    .poll(() => overlay.evaluate((el) => Number(getComputedStyle(el).opacity)))
    .toBeCloseTo(0.5, 1);
  await expect
    .poll(async () => {
      const box = (await button.boundingBox())!;
      return box.y + box.height - area.y;
    })
    .toBeCloseTo(4, 0);
  await gap(95);
  await expect(overlay).toHaveAttribute("inert", "");
  await expect(overlay).toHaveCSS("opacity", "0");
  // When the input still straddles the upper edge, its predecessor stays hidden.
  await gap(-10);
  await expect(overlay).toHaveAttribute("data-available", "false");
  await expect(overlay).toHaveCSS("opacity", "0");
  // Moving downward past the same input uses its bottom edge, with the same
  // 96..128px ramp as the next input's top edge. No next input is required.
  async function past(pixels: number) {
    await input.evaluate((el, pixels) => {
      const pane = el.closest(".transcript")!;
      pane.scrollTop +=
        el.getBoundingClientRect().bottom -
        pane.getBoundingClientRect().top +
        pixels;
      pane.dispatchEvent(new Event("reading-move"));
    }, pixels);
    await expect
      .poll(
        async () =>
          (await pane.boundingBox())!.y -
          (await input.boundingBox())!.y -
          (await input.boundingBox())!.height,
      )
      .toBeCloseTo(pixels, 0);
  }
  await past(95);
  await expect(overlay).toHaveAttribute("data-pinned-seq", seq);
  await expect(overlay).toHaveAttribute("inert", "");
  await expect(overlay).toHaveCSS("opacity", "0");
  await past(112);
  await expect
    .poll(() => overlay.evaluate((el) => Number(getComputedStyle(el).opacity)))
    .toBeCloseTo(0.5, 1);
  await expect
    .poll(async () => {
      const box = (await button.boundingBox())!;
      return box.y + box.height - area.y;
    })
    .toBeCloseTo(4, 0);
  await past(129);
  await expect(overlay).toHaveCSS("opacity", "1");
  await expect
    .poll(async () => {
      const box = (await button.boundingBox())!;
      return box.y + box.height - area.y;
    })
    .toBeCloseTo(8, 0);
  await past(95);
  await expect(overlay).toHaveAttribute("inert", "");
  await expect(overlay).toHaveCSS("opacity", "0");
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
  await reveal(page);
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
  // The tall input remains partly visible at the latest position in a tall
  // viewport. A shorter viewport fits inside its response, clearing the input.
  await page.setViewportSize({ width: 1440, height: 600 });
  await expect(pinned).toContainText("Pinned input line 100");
  await expect(page.locator(".conversation header small")).toContainText(
    "idle",
  );
  const pane = page.locator(".transcript");
  await reveal(page);
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
