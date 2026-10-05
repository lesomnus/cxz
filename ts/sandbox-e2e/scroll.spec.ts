import { test, expect } from "@playwright/test";
test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

test("grouped sessions, shortcut, stable fields and elastic local scrollbar", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const groups = page.locator(".session-tree-group");
  await expect(groups).toHaveCount(2);
  await expect(groups.first().locator(".tree-session")).toHaveCount(7);
  await expect(groups.last().locator(".tree-session")).toHaveCount(1);
  await expect(page.locator(".conversation header strong")).toHaveText(
    "session-1",
  );
  await expect(page.locator(".tree-session").first()).toContainText(
    "session-1",
  );
  await expect(page.locator(".tree-session").first()).toContainText(
    "Project checklist",
  );
  const effortX = (await page.locator(".effort-field").boundingBox())!.x;
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(page.locator(".model-field .meta-value")).toHaveText(
    "sandbox-codex",
  );
  expect((await page.locator(".effort-field").boundingBox())!.x).toBe(effortX);
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(send).toHaveText("");
  await expect(send).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await page.locator(".send-control").hover();
  await expect(page.getByRole("tooltip")).toHaveText("ctrl+enter");
  const message = page.getByRole("textbox", { name: "Message", exact: true });
  await message.fill("Keyboard sends once");
  await message.press("Control+Enter");
  await expect(
    page.locator("article.input").filter({ hasText: "Keyboard sends once" }),
  ).toHaveCount(1);
  await expect(message).toHaveValue("");

  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  const pane = page.locator(".transcript"),
    area = page.locator(".transcript-area");
  const handle = page.locator(".scroll-handle"),
    thumb = page.getByRole("scrollbar", { name: "Conversation scroll" });
  const areaBox = (await area.boundingBox())!;
  await page.mouse.move(100, 20);
  await expect(handle).toHaveCSS("opacity", "0");
  await page.mouse.move(
    areaBox.x + areaBox.width / 2,
    areaBox.y + areaBox.height / 2,
  );
  await expect(handle).toHaveCSS("opacity", "0.7");
  await expect(page.locator(".scroll-marker").first()).toBeVisible();
  const maximum = Number(await thumb.getAttribute("aria-valuemax"));
  const span = Number(await thumb.getAttribute("data-range"));
  expect(span).toBeLessThan(maximum);
  expect(span).toBeLessThanOrEqual(areaBox.height * 12);
  const bounds = (await thumb.boundingBox())!;
  await page.mouse.move(
    bounds.x + bounds.width / 2,
    bounds.y + bounds.height / 2,
  );
  await expect(handle).toHaveCSS("transform", "matrix(2, 0, 0, 1, 0, 0)");
  await page.mouse.down();
  const start = Number(await thumb.getAttribute("data-start"));
  const rail = (await page.locator(".scroll-track").boundingBox())!;
  await page.mouse.move(
    bounds.x + bounds.width / 2,
    rail.y + bounds.height / 2 - 120,
    { steps: 5 },
  );
  await expect
    .poll(async () => Number(await thumb.getAttribute("data-stretch")))
    .toBeGreaterThan(10);
  await expect
    .poll(async () => pane.evaluate((el) => el.scrollTop))
    .toBeLessThan(start - 100);
  expect(Number(await thumb.getAttribute("data-stretch"))).toBeLessThanOrEqual(
    20,
  );
  const animationStart = await page.evaluate(() => {
    const rail = document.querySelector(".scroll-track")!;
    const marker = [
      ...rail.querySelectorAll<HTMLElement>(".scroll-marker"),
    ].find(
      (el) =>
        parseFloat(getComputedStyle(el).top) > 0 &&
        parseFloat(getComputedStyle(el).top) < rail.clientHeight,
    )!;
    (window as any).savedPromptMarker = marker;
    return {
      id: marker.dataset.prompt!,
      marker: parseFloat(getComputedStyle(marker).top),
      thumb: parseFloat(
        getComputedStyle(document.querySelector(".scroll-thumb")!).top,
      ),
    };
  });
  await page.mouse.up();
  await expect(thumb).toHaveAttribute("data-stretch", "0.00");
  const animation = await page.evaluate(async (before) => {
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    );
    const marker = document.querySelector<HTMLElement>(
      `[data-prompt="${before.id}"]`,
    )!;
    const thumb = document.querySelector<HTMLElement>(".scroll-thumb")!;
    return {
      sameNode: marker === (window as any).savedPromptMarker,
      markerProgress:
        (parseFloat(getComputedStyle(marker).top) - before.marker) /
        (parseFloat(marker.style.top) - before.marker),
      thumbProgress:
        (parseFloat(getComputedStyle(thumb).top) - before.thumb) /
        (parseFloat(thumb.style.top) - before.thumb),
    };
  }, animationStart);
  expect(animation.sameNode).toBe(true);
  expect(animation.markerProgress).toBeGreaterThan(0);
  expect(
    Math.abs(animation.markerProgress - animation.thumbProgress),
  ).toBeLessThan(0.12);
  const first = await page
    .locator(".transcript-content [data-seq]")
    .first()
    .getAttribute("data-seq");
  await pane.evaluate((el) => (el.scrollTop = 0));
  await expect
    .poll(async () =>
      BigInt(
        (await page
          .locator(".transcript-content [data-seq]")
          .first()
          .getAttribute("data-seq"))!,
      ),
    )
    .toBeLessThan(BigInt(first!));
  expect(await page.locator(".transcript [data-seq]").count()).toBeLessThan(60);
  expect(
    Number(await page.locator(".virtual-messages").getAttribute("data-cached")),
  ).toBeLessThanOrEqual(512);
  await page.getByRole("button", { name: "↓ Latest" }).click();
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  expect(errors).toEqual([]);
  await page.screenshot({
    path: "test-results/sandbox-scroll.png",
    fullPage: true,
  });
});
