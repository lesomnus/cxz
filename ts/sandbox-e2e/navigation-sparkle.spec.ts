import { expect, test, devices, type Locator } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
  launchOptions: {
    args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"],
  },
});

async function visibleInk(link: Locator) {
  const ink = link.locator("canvas[data-ink-sparkle]");
  await expect(ink).toHaveAttribute("data-renderer", "webgl2");
  await expect(ink).toHaveAttribute("data-pressed", "true");
  // Check rendered pixels, rather than merely the presence of a canvas.
  await expect
    .poll(() =>
      ink.evaluate((node: HTMLCanvasElement) => {
        const gl = node.getContext("webgl2")!;
        const pixels = new Uint8Array(node.width * node.height * 4);
        gl.readPixels(
          0,
          0,
          node.width,
          node.height,
          gl.RGBA,
          gl.UNSIGNED_BYTE,
          pixels,
        );
        return pixels.some((v, i) => i % 4 === 3 && v > 0);
      }),
    )
    .toBe(true);
}

test("sidebar and session links render ink while preserving routing and new tabs", async ({
  page,
  context,
}) => {
  await page.goto("/sandbox.html#/sessions/session-1");
  const projects = page.getByRole("link", {
    name: "Projects view",
    exact: true,
  });
  await expect(projects).toBeVisible({ timeout: 45000 });
  await expect(projects).toHaveAttribute("data-effect", "sparkle");
  const box = (await projects.boundingBox())!;
  const icon = (await projects.locator("svg").first().boundingBox())!;
  expect(
    Math.abs(icon.x + icon.width / 2 - box.x - box.width / 2),
  ).toBeLessThan(1);
  expect(
    Math.abs(icon.y + icon.height / 2 - box.y - box.height / 2),
  ).toBeLessThan(1);
  await projects.hover();
  await page.mouse.down();
  await visibleInk(projects);
  expect(await projects.boundingBox()).toEqual(box);
  await page.mouse.up();
  await expect(page).toHaveURL(/#\/projects$/);
  await expect(projects.locator("canvas[data-ink-sparkle]")).toHaveCount(0);
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  const panel = page.getByRole("complementary", { name: "Session list" });
  const session = panel.locator('.tree-session[href$="/session-2"]');
  await expect(session).toBeVisible();
  await expect(session).toHaveAttribute("data-effect", "sparkle");
  await session.hover();
  await page.mouse.down();
  await visibleInk(session);
  await page.mouse.up();
  await expect(page).toHaveURL(/#\/sessions\/session-2$/);
  await expect(session.locator("canvas[data-ink-sparkle]")).toHaveCount(0);

  const next = panel.locator('.tree-session[href$="/session-3"]');
  const opened = context.waitForEvent("page");
  await next.click({ modifiers: ["Control"] });
  const tab = await opened;
  await tab.waitForURL(/#\/sessions\/session-3$/, { waitUntil: "commit" });
  expect(tab.url()).toMatch(/#\/sessions\/session-3$/);
  await expect(page).toHaveURL(/#\/sessions\/session-2$/);
  await tab.close();
});
