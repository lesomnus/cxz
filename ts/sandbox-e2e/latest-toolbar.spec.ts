import { test, expect } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

test("Latest slides into the toolbar overlay and switches after 96px of actual reading movement", async ({
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
  const slot = page.locator(".latest-slot");
  const button = slot.locator("button");
  const latest = page.getByRole("button", { name: "Latest", exact: true });
  const send = page.getByRole("button", { name: "Send", exact: true });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Keep this draft");
  const toolbar = (await page.locator(".composer-toolbar").boundingBox())!;
  const inputBox = (await input.boundingBox())!;
  await expect(slot).toHaveAttribute("inert", "");
  await expect(latest).toHaveCount(0);
  expect(await button.textContent()).toBe("");
  const area = (await pane.boundingBox())!;
  await page.mouse.move(area.x + area.width / 2, area.y + area.height / 2);
  const before = await pane.evaluate((el) => el.scrollTop);
  await page.mouse.wheel(0, -80);
  // A wheel of eighty moves the pane by eighty, give or take what the browser
  // rounds a fractional device pixel to -- it arrives as seventy-eight often
  // enough to matter, and the exact conversion is not what is under test here.
  // What is under test is that real wheel movement scrolls the pane and leaves
  // the slot hidden; the reading-movement thresholds below are driven by
  // scrollTop directly, so they do not inherit this.
  await expect
    .poll(async () => before - (await pane.evaluate((el) => el.scrollTop)))
    .toBeCloseTo(80, -1);
  await expect(slot).toHaveAttribute("data-visible", "false");
  const frames = await pane.evaluate(async (el) => {
    const button = document.querySelector<HTMLElement>(".latest-button")!;
    el.scrollTop -= 20;
    const frames: { opacity: number; top: number }[] = [];
    for (let i = 0; i < 20; i++) {
      await new Promise(requestAnimationFrame);
      frames.push({
        opacity: Number(getComputedStyle(button).opacity),
        top: button.getBoundingClientRect().top,
      });
    }
    return frames;
  });
  expect(frames.some((frame) => frame.opacity > 0 && frame.opacity < 1)).toBe(
    true,
  );
  expect(
    frames.some(
      (frame) => frame.top > toolbar.y + 3 && frame.top < toolbar.y + 26,
    ),
  ).toBe(true);
  await expect(slot).toHaveAttribute("data-visible", "true");
  await expect(button).toHaveCSS("opacity", "1");
  const latestBox = (await latest.boundingBox())!;
  const sendBox = (await send.boundingBox())!;
  expect(latestBox.width).toBe(48);
  expect(latestBox.height).toBe(24);
  expect(latestBox.y).toBe(sendBox.y);
  expect(latestBox.x + latestBox.width / 2).toBe(toolbar.x + toolbar.width / 2);
  expect(sendBox.width).toBe(48);
  expect(sendBox.height).toBe(latestBox.height);
  await latest.hover();
  await expect(latest).toHaveCSS("background-color", "rgb(54, 54, 54)");
  await expect(latest).toHaveCSS("border-radius", "7px");
  await send.hover();
  await expect(send).toHaveCSS("background-color", "rgb(54, 54, 54)");
  // Occupy the same toolbar position with an ordinary future control. Latest
  // keeps its own layer without shifting either the input or Send.
  await page.locator(".composer-toolbar").evaluate((el) => {
    const other = document.createElement("button");
    other.className = "future-toolbar-control";
    other.textContent = "Other";
    other.style.cssText =
      "position:absolute;left:50%;top:2px;transform:translateX(-50%);width:64px;height:24px;min-height:24px";
    el.append(other);
  });
  expect(
    await latest.evaluate((el) => {
      const box = el.getBoundingClientRect();
      return el.contains(
        document.elementFromPoint(
          box.left + box.width / 2,
          box.top + box.height / 2,
        ),
      );
    }),
  ).toBe(true);
  expect(await input.boundingBox()).toEqual(inputBox);
  expect(await send.boundingBox()).toEqual(sendBox);
  await page.locator(".future-toolbar-control").evaluate((el) => el.remove());
  async function move(delta: number) {
    await pane.evaluate(async (el, delta) => {
      el.scrollTop += delta;
      for (let i = 0; i < 4; i++) await new Promise(requestAnimationFrame);
    }, delta);
  }
  await move(-300);
  await move(80);
  await expect(slot).toHaveAttribute("data-visible", "true");
  await move(-20);
  await move(35);
  await expect(slot).toHaveAttribute("data-visible", "true");
  await move(2);
  await expect(slot).toHaveAttribute("data-visible", "false");
  await expect(slot).toHaveAttribute("inert", "");
  expect(
    await pane.evaluate(
      (el) => el.scrollHeight - el.clientHeight - el.scrollTop,
    ),
  ).toBeGreaterThan(100);
  await move(-80);
  await expect(slot).toHaveAttribute("data-visible", "false");
  await move(-20);
  await expect(slot).toHaveAttribute("data-visible", "true");
  await expect(button).toHaveCSS("opacity", "1");
  await page.screenshot({
    path: "test-results/sandbox-latest-toolbar.png",
    fullPage: true,
  });
  await latest.click();
  await expect
    .poll(() =>
      pane.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(1);
  await expect(slot).toHaveAttribute("data-visible", "false");
  await expect(input).toHaveValue("Keep this draft");
});
