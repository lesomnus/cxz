import { expect, test } from "@playwright/test";

test("shared panel items show selection separately from hover", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-panelitem--selection&viewMode=story",
  );
  const general = page.getByRole("button", { name: "General", exact: true });
  const editor = page.getByRole("button", { name: "Editor", exact: true });
  const background = (element: HTMLElement) =>
    getComputedStyle(element).backgroundColor;
  await expect(general).toHaveAttribute("aria-pressed", "true");
  const selected = await general.evaluate(background);
  const normal = await editor.evaluate(background);
  expect(selected).not.toBe(normal);
  await editor.hover();
  expect(await editor.evaluate(background)).not.toBe(normal);
  await editor.click();
  await page.mouse.move(600, 400);
  await expect(editor).toHaveAttribute("aria-pressed", "true");
  expect(await editor.evaluate(background)).toBe(selected);
  expect(await general.evaluate(background)).toBe(normal);
});
