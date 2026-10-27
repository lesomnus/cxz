import { expect, test, type Locator } from "@playwright/test";

test("full ID hints escape clipped containers and remain inside the viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 600 });
  await page.goto(
    "/iframe.html?id=components-overflowhint--default&viewMode=story",
  );
  const trigger = page.locator("[tabindex='0']");
  await trigger.hover();
  const hint = page.getByRole("tooltip");
  await expect(hint).toHaveText("1a2b3c4d-5e6f-7788-9900-aabbccddeeff");
  const card = (await trigger.locator("..").boundingBox())!;
  const bounds = (await hint.boundingBox())!;
  expect(bounds.x + bounds.width).toBeGreaterThan(card.x + card.width);
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(600);
  expect(await hint.evaluate((el) => el.parentElement === document.body)).toBe(
    true,
  );
  await page.mouse.move(0, 0);
  await expect(hint).toHaveCount(0);
});

async function expectAlignedText(trigger: Locator) {
  const boxes = await trigger.evaluate((element) => {
    function firstCharacters(target: Element) {
      const walker = document.createTreeWalker(target, NodeFilter.SHOW_TEXT);
      const text = walker.nextNode()!;
      const range = document.createRange();
      range.setStart(text, 0);
      range.setEnd(text, Math.min(7, text.textContent!.length));
      const rect = range.getBoundingClientRect();
      return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
    }
    const hint = document.getElementById(
      element.getAttribute("aria-describedby")!,
    )!;
    return {
      original: firstCharacters(element),
      expanded: firstCharacters(hint),
    };
  });
  for (const key of ["x", "y", "width", "height"] as const)
    expect(Math.abs(boxes.original[key] - boxes.expanded[key])).toBeLessThan(
      0.25,
    );
}

test("in-place ID overlays keep the text origin and baseline with inherited typography", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 600 });
  await page.goto(
    "/iframe.html?id=components-overflowhint--in-place&viewMode=story",
  );
  const trigger = page.locator("[tabindex='0']");
  await trigger.hover();
  const hint = page.getByRole("tooltip");
  await expect(hint).toBeVisible();
  await expectAlignedText(trigger);
  await expect(hint).toHaveCSS("font-size", "18px");
  await expect(hint).toHaveCSS("line-height", "36px");
  await page.mouse.move(0, 0);
  await expect(hint).toHaveCount(0);
  await trigger.hover();
  await expect(hint).toBeVisible();
  await expectAlignedText(trigger);
  const frame = (await hint.boundingBox())!;
  expect(frame.x + frame.width).toBeLessThanOrEqual(390);
  expect(await hint.evaluate((el) => el.parentElement === document.body)).toBe(
    true,
  );
});
