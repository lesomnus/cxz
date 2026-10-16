import { test, expect } from "@playwright/test";

test("working glow paints above the toolbar without shifting controls or intercepting input", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(
    "/iframe.html?id=conversation-composer--working&viewMode=story",
  );
  const glow = page.locator(".composer-aurora");
  await expect(glow).toHaveAttribute("data-active", "true");
  const viewport = page.locator(".composer-aurora-viewport");
  await expect(viewport).toHaveCSS("overflow", "visible");
  await expect(viewport).toHaveCSS("contain", "none");
  await expect(viewport).toHaveCSS("pointer-events", "none");
  const toolbar = page.locator(".composer-toolbar");
  const before = (await toolbar.boundingBox())!;
  const clip = {
    x: before.x + 20,
    y: Math.max(0, before.y - 50),
    width: before.width - 40,
    height: Math.min(46, before.y - 4),
  };
  expect(clip.height).toBeGreaterThan(10);
  const lit = await page.screenshot({ clip });
  await glow.evaluate((el) => (el.style.opacity = "0"));
  const unlit = await page.screenshot({ clip });
  expect(lit.equals(unlit)).toBe(false);
  expect(await toolbar.boundingBox()).toEqual(before);
  await glow.evaluate((el) => el.style.removeProperty("opacity"));
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Still editable while working");
  await expect(input).toHaveValue("Still editable while working");
});
