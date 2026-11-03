import { test, expect, devices } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1844, height: 1000 },
});

test("expanded code ends at the session panel and editor, and tracks split resizing", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/session/session-1");
  const panel = page.getByRole("complementary", { name: "Session list" });
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  const pre = page.locator(".transcript pre").first(),
    block = pre.locator("..");
  await expect(pre).toBeVisible({ timeout: 45000 });
  await expect(editor).toBeVisible();
  const pane = page.locator(".transcript");
  await block.hover();
  const expand = block.getByRole("button", {
    name: "Expand code block",
    exact: true,
  });
  const toolbar = block
    .getByRole("button", { name: "Copy", exact: true })
    .locator("../..");
  const before = (await toolbar.boundingBox())!;
  await expand.click();
  const check = async () => {
    await expect
      .poll(async () => {
        const code = (await pre.boundingBox())!,
          view = (await pane.boundingBox())!;
        return Math.max(
          Math.abs(code.x - view.x),
          Math.abs(code.x + code.width - view.x - view.width),
        );
      })
      .toBeLessThan(1);
  };
  await check();
  const after = (await toolbar.boundingBox())!;
  expect(after.x).toBeCloseTo(before.x, 1);
  expect(after.y).toBeCloseTo(before.y, 1);
  const code = (await pre.boundingBox())!,
    left = (await panel.boundingBox())!,
    right = (await editor.boundingBox())!;
  expect(code.x).toBeCloseTo(left.x + left.width, 1);
  expect(code.x + code.width).toBeLessThanOrEqual(right.x + 1);
  expect(right.x - code.x - code.width).toBeLessThan(12);
  expect(
    await pre.evaluate((el) => {
      const b = el.getBoundingClientRect();
      return el.contains(document.elementFromPoint(b.left + 3, b.top + 4));
    }),
  ).toBe(true);
  const divider = page.getByRole("separator", {
    name: "Resize workspace editor",
  });
  const d = (await divider.boundingBox())!;
  await page.mouse.move(d.x + d.width / 2, d.y + 200);
  await page.mouse.down();
  await page.mouse.move(d.x + 160, d.y + 200, { steps: 8 });
  await page.mouse.up();
  await check();
  await page.setViewportSize({ width: 1280, height: 900 });
  await check();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    1280,
  );
  await block
    .getByRole("button", { name: "Restore code block width", exact: true })
    .click();
  await expect(expand).toHaveAttribute("aria-pressed", "false");
});
