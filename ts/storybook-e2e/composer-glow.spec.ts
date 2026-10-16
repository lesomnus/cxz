import { test, expect } from "@playwright/test";
test("glow stays inside the toolbar while continuing behind the rounded editor shoulders", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(
    "/iframe.html?id=conversation-composer--working&viewMode=story",
  );
  const glow = page.locator(".composer-aurora");
  await expect(glow).toHaveAttribute("data-active", "true");
  await expect(page.locator(".composer-aurora-viewport")).toHaveCSS(
    "overflow",
    "clip",
  );
  const wrapper = (await page.locator(".composer-wrapper").boundingBox())!;
  const input = (await page.locator(".composer-input").boundingBox())!;
  const shoulder = { x: wrapper.x + 1, y: input.y + 1, width: 3, height: 4 };
  const above = {
    x: wrapper.x + 20,
    y: wrapper.y - 20,
    width: wrapper.width - 40,
    height: 15,
  };
  const lit = await page.screenshot({ clip: shoulder });
  const aboveLit = await page.screenshot({ clip: above });
  await glow.evaluate((el) => (el.style.opacity = "0"));
  expect(lit.equals(await page.screenshot({ clip: shoulder }))).toBe(false);
  expect(aboveLit.equals(await page.screenshot({ clip: above }))).toBe(true);
  expect(await page.locator(".composer-input").boundingBox()).toEqual(input);
  await glow.evaluate((el) => el.style.removeProperty("opacity"));
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  await editor.fill("Still editable");
  await expect(editor).toHaveValue("Still editable");
});
