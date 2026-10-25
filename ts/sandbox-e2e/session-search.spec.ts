import { BRAND_COLOR } from "../packages/nox/src/theme/colors";
import { expect, test, devices } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("local search includes loaded virtualized history, excludes unloaded history and closes only explicitly", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/session/session-3");
  const pane = page.locator(".transcript");
  await expect(pane).toContainText("History item 2100", { timeout: 45000 });
  await expect(pane).not.toContainText("History item 1800");
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  await editor.focus();
  await page.keyboard.press("Control+Shift+F");
  const search = page.getByRole("search", {
      name: "Search current conversation",
    }),
    input = search.getByRole("searchbox");
  await expect(input).toBeFocused();
  const outer = (await page.locator("main.conversation").boundingBox())!,
    box = (await search.boundingBox())!;
  expect(box.y - outer.y).toBeGreaterThanOrEqual(40);
  expect(box.x).toBeGreaterThanOrEqual(outer.x);
  expect(box.x + box.width).toBeLessThanOrEqual(outer.x + outer.width);
  await input.fill("HISTORY ITEM 0001");
  await expect(search.getByRole("status")).toHaveText("No matches");
  await expect(pane).not.toContainText("History item 0001");
  await input.fill("HISTORY ITEM 1800");
  await expect(search.getByRole("status")).toHaveText("1 matches");
  await search.getByRole("button", { name: /History item 1800/ }).click();
  await expect(pane).toContainText("History item 1800");
  await expect(input).toBeFocused();
  await pane.click({ position: { x: 10, y: 20 } });
  await expect(search).toBeVisible();
  await editor.focus();
  await page.keyboard.press("Escape");
  await expect(search).toHaveCount(0);
  await expect(editor).toBeFocused();
  await page.keyboard.press("Control+Shift+F");
  await expect(input).toHaveValue("HISTORY ITEM 1800");
  await search.getByRole("button", { name: "Close search" }).click();
  await expect(search).toHaveCount(0);
});

test("local search has no result page limit and Enter visits newest matches with focus and highlight cleanup", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/session/session-3");
  const pane = page.locator(".transcript");
  await expect(pane).toContainText("History item 2100", { timeout: 45000 });
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await page.keyboard.press("Control+Shift+F");
  const search = page.getByRole("search", {
      name: "Search current conversation",
    }),
    input = search.getByRole("searchbox");
  await input.fill("History item");
  const results = search.locator("[data-search-index]");
  await expect.poll(() => results.count()).toBeGreaterThan(100);
  await expect(
    search.getByRole("button", { name: "Load more results" }),
  ).toHaveCount(0);
  const seqs = await Promise.all(
    [0, 1, 2].map((index) =>
      search.locator(`[data-search-index="${index}"]`).getAttribute("data-seq"),
    ),
  );
  const active = () =>
    page.evaluate(() => {
      const range = [
        ...(CSS.highlights.get("conversation-search-current") ?? []),
      ][0] as Range | undefined;
      return range?.startContainer.parentElement?.closest<HTMLElement>(
        "[data-row]",
      )?.dataset.row;
    });
  for (const seq of seqs) {
    await input.press("Enter");
    await expect.poll(active).toBe(seq);
    await expect(input).toBeFocused();
  }
  await input.fill("HISTORY ITEM 1800");
  await expect(search.getByRole("status")).toHaveText("1 matches");
  const target = await search
    .locator('[data-search-index="0"]')
    .getAttribute("data-seq");
  await input.press("Enter");
  await expect.poll(active).toBe(target);
  await expect(pane).toContainText("History item 1800");
  await input.press("Enter");
  await expect.poll(active).toBe(target);
  await input.press("Escape");
  await expect
    .poll(() => page.evaluate(() => CSS.highlights.has("conversation-search")))
    .toBe(false);
});

test("green search ticks share existing marker motion and disappear when search clears or closes", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/session/session-3");
  const pane = page.locator(".transcript");
  await expect(pane).toContainText("History item 2100", { timeout: 45000 });
  await page.getByRole("textbox", { name: "Message", exact: true }).focus();
  await page.keyboard.press("Control+Shift+F");
  const input = page.getByRole("searchbox", {
      name: "Search this conversation",
    }),
    ticks = page.locator(".scroll-tick[data-search]");
  await input.fill("History item");
  await expect.poll(() => ticks.count()).toBeGreaterThan(0);
  await expect(ticks.first()).toHaveCSS(
    "background-color",
    `rgb(${BRAND_COLOR.slice(1)
      .match(/../g)!
      .map((part) => parseInt(part, 16))
      .join(", ")})`,
  );
  await expect(ticks.first()).toHaveCSS("opacity", "1");
  const aligned = () =>
    page.evaluate(() => {
      const ticks = [
        ...document.querySelectorAll<HTMLElement>(".scroll-tick[data-search]"),
      ];
      return (
        ticks.length > 0 &&
        ticks.every((tick) => {
          const original = document.querySelector<HTMLElement>(
            `.scroll-tick[data-event="${tick.dataset.search}"]`,
          );
          return (
            !!original &&
            Math.abs(
              parseFloat(getComputedStyle(tick).top) -
                parseFloat(getComputedStyle(original).top),
            ) < 0.5
          );
        })
      );
    });
  await expect.poll(aligned).toBe(true);
  await pane.hover({ position: { x: 40, y: 450 } });
  await page.mouse.wheel(0, -300);
  await expect.poll(aligned).toBe(true);
  const thumb = page.locator(".scroll-thumb"),
    bounds = (await thumb.boundingBox())!,
    rail = (await page.locator(".scroll-track").boundingBox())!;
  await page.mouse.move(
    bounds.x + bounds.width / 2,
    bounds.y + bounds.height / 2,
  );
  await page.mouse.down();
  try {
    await page.mouse.move(
      bounds.x + bounds.width / 2,
      rail.y + bounds.height / 2 - 100,
      { steps: 4 },
    );
    await expect
      .poll(() => thumb.getAttribute("data-stretch").then(Number))
      .toBeGreaterThan(1);
    await expect.poll(aligned).toBe(true);
  } finally {
    await page.mouse.up();
  }
  await expect.poll(aligned).toBe(true);
  await input.fill("No matching phrase at all");
  await expect(ticks).toHaveCount(0);
  await input.fill("History item");
  await expect.poll(() => ticks.count()).toBeGreaterThan(0);
  await input.press("Escape");
  await expect(ticks).toHaveCount(0);
});
