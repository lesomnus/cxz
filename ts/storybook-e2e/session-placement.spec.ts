import { expect, test } from "@playwright/test";

for (const width of [1280, 390]) {
  test(`session creation defaults to a centered modal at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/iframe.html?id=accounts-workflows--session-creation-requires-login&viewMode=story",
    );
    const add = page.getByRole("button", { name: "Add session", exact: true });
    await add.click();
    const dialog = page.getByRole("dialog", {
      name: "Add session",
      exact: true,
    });
    await expect
      .poll(() =>
        dialog.evaluate((node) => node.contains(document.activeElement)),
      )
      .toBe(true);
    const rect = (await dialog.boundingBox())!;
    expect(rect.y).toBeGreaterThan(64);
    expect(rect.x + rect.width / 2).toBeCloseTo(width / 2, 0);
    expect(rect.y + rect.height / 2).toBeCloseTo(450, 0);
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(add).toBeFocused();
  });
}

test("unanchored account forms use the same centered fallback", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add account", exact: true });
  await expect(dialog).toBeVisible();
  const rect = (await dialog.boundingBox())!;
  expect(rect.y).toBeGreaterThan(64);
  expect(rect.y + rect.height / 2).toBeCloseTo(
    page.viewportSize()!.height / 2,
    0,
  );
});

for (const [story, sectionLabel] of [
  ["resources-views--accounts-long-sessions", "Sessions using this account"],
  ["pages-projects--long-sessions", "Project sessions"],
]) {
  test(`${sectionLabel} scrollbar sits on the section edge and remains usable`, async ({
    page,
  }) => {
    await page.goto(`/iframe.html?id=${story}&viewMode=story`);
    const section = page
      .getByRole("region", { name: sectionLabel, exact: true })
      .first();
    const area = section.getByRole("region", {
      name: "Session list",
      exact: true,
    });
    const thumb = area.getByRole("scrollbar", {
      name: "Session list",
      exact: true,
    });
    await expect(thumb).toBeVisible();
    await area.hover();
    const handle = area.locator('[data-scroll-area="handle"]');
    const box = (await handle.boundingBox())!;
    const edge =
      (await section.boundingBox())!.x + (await section.boundingBox())!.width;
    expect(box.x + box.width / 2).toBeCloseTo(edge, 0);
    expect(
      await page.evaluate(
        ({ x, y }) =>
          !!document.elementFromPoint(x, y)?.closest('[role="scrollbar"]'),
        { x: edge + 1, y: box.y + box.height / 2 },
      ),
    ).toBe(true);
    await thumb.focus();
    await page.keyboard.press("End");
    await expect
      .poll(() =>
        area
          .locator('[data-scroll-area="viewport"]')
          .evaluate((node) => node.scrollTop),
      )
      .toBeGreaterThan(0);
    await expect(section.locator("tbody tr").last()).toBeInViewport();
    await page.keyboard.press("Home");
    await expect
      .poll(() =>
        area
          .locator('[data-scroll-area="viewport"]')
          .evaluate((node) => node.scrollTop),
      )
      .toBe(0);
    const drag = (await thumb.boundingBox())!;
    await page.mouse.move(edge + 1, drag.y + drag.height / 2);
    await page.mouse.down();
    await page.mouse.move(edge + 1, drag.y + drag.height / 2 + 40, {
      steps: 4,
    });
    await page.mouse.up();
    await expect
      .poll(() =>
        area
          .locator('[data-scroll-area="viewport"]')
          .evaluate((node) => node.scrollTop),
      )
      .toBeGreaterThan(0);
  });
}
