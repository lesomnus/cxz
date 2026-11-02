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

  for (const width of [1280, 390]) {
    test(`${sectionLabel} stays still when wheeling past the table boundary at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 700 });
      await page.goto(`/iframe.html?id=${story}&viewMode=story`);
      const area = page
        .getByRole("region", { name: sectionLabel, exact: true })
        .first()
        .getByRole("region", { name: "Session list", exact: true });
      const viewport = area.locator('[data-scroll-area="viewport"]');
      await expect(viewport).toHaveCSS("border-radius", "0px");
      await expect(viewport).toHaveCSS("overscroll-behavior", "none");
      const content = area.locator('[data-scroll-area="content"]');
      await expect(content).toHaveCSS("padding-right", "0px");
      const edges = await content.evaluate((node) => ({
        content: node.getBoundingClientRect().right,
        table: node.querySelector("table")!.getBoundingClientRect().right,
      }));
      expect(edges.table).toBeCloseTo(edges.content, 1);
      const thumb = area.getByRole("scrollbar", {
        name: "Session list",
        exact: true,
      });
      for (const [key, delta] of [
        ["End", 320],
        ["Home", -320],
      ] as const) {
        await thumb.press(key);
        const before = (await viewport.boundingBox())!;
        const grab = (await thumb.boundingBox())!;
        await page.mouse.move(
          grab.x + grab.width / 2 + 1,
          grab.y + grab.height / 2,
        );
        await page.mouse.wheel(0, delta);
        await page.mouse.wheel(0, delta);
        await page.waitForTimeout(150);
        const after = (await viewport.boundingBox())!;
        expect(after.y).toBeCloseTo(before.y, 1);
        expect(after.height).toBeCloseTo(before.height, 1);
        const samples = await viewport.evaluate(async (node) => {
          const values: number[] = [];
          for (let frame = 0; frame < 12; frame++) {
            await new Promise(requestAnimationFrame);
            values.push(node.querySelector("table")!.getBoundingClientRect().y);
          }
          return values;
        });
        expect(Math.max(...samples) - Math.min(...samples)).toBeLessThan(0.1);
      }
      // Enter the content while a free-spinning wheel keeps delivering input,
      // then continue with a fractional tail after reaching the native end.
      await viewport.evaluate((node) => {
        node.scrollTop = node.scrollHeight - node.clientHeight - 40;
        node.dataset.blockedWheels = "0";
        node.addEventListener("wheel", (event) => {
          if (event.defaultPrevented)
            node.dataset.blockedWheels = String(
              Number(node.dataset.blockedWheels) + 1,
            );
        });
      });
      const contentBounds = (await viewport.boundingBox())!;
      await page.mouse.move(
        contentBounds.x + contentBounds.width / 2,
        contentBounds.y + contentBounds.height / 2,
      );
      for (const delta of [24, 24, 24, 1, 0.5, 0.25, 0.125, 0.125, 0.125])
        await page.mouse.wheel(0, delta);
      await expect
        .poll(() =>
          viewport.evaluate((node) => Number(node.dataset.blockedWheels)),
        )
        .toBeGreaterThan(0);
      const final = (await viewport.boundingBox())!;
      expect(final.y).toBeCloseTo(contentBounds.y, 1);
      await expect(area.locator('[data-scroll-area="bottom-fade"]')).toHaveCSS(
        "opacity",
        "0",
      );
      // A sequence that starts inside must be consumed from its first event;
      // waiting until the end can leave subsequent events non-cancelable.
      await viewport.evaluate((node) => {
        node.scrollTop = 0;
        node.addEventListener(
          "wheel",
          (event) => {
            node.dataset.firstInsideWheel = String(event.defaultPrevented);
          },
          { once: true },
        );
      });
      await page.mouse.move(
        contentBounds.x + contentBounds.width / 2,
        contentBounds.y + contentBounds.height / 2,
      );
      await page.mouse.wheel(0, 32);
      await expect(viewport).toHaveAttribute("data-first-inside-wheel", "true");
      await expect
        .poll(() => viewport.evaluate((node) => node.scrollTop))
        .toBe(32);
      for (const delta of [120, 120, 120, 120, 120, 120, 1, 0.5, 0.25, 0.125])
        await page.mouse.wheel(0, delta);
      await expect
        .poll(() =>
          viewport.evaluate((node) =>
            Math.abs(node.scrollHeight - node.clientHeight - node.scrollTop),
          ),
        )
        .toBeLessThan(1);
      expect((await viewport.boundingBox())!.y).toBeCloseTo(contentBounds.y, 1);
    });
  }
}
