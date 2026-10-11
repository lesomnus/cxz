import { expect, test } from "@playwright/test";

test("composer path hints use the shared suggestion overlay in serverless previews", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--path-hints&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Inspect `/workspace/s");
  const hints = page.getByRole("listbox", {
    name: "Path suggestions",
    exact: true,
  });
  await expect(
    hints.locator('[aria-selected="true"] .command-name'),
  ).toHaveText("/workspace/src/");
  await input.press("ArrowRight");
  await expect(input).toHaveValue("Inspect `/workspace/src/");
  await expect
    .poll(() => input.evaluate((el: HTMLTextAreaElement) => el.selectionStart))
    .toBe("Inspect `/workspace/src/".length);
  await expect(hints.getByRole("option", { name: /app.ts/ })).toBeVisible();
  await hints.getByRole("option", { name: /app.ts/ }).click();
  await expect(input).toHaveValue("Inspect `/workspace/src/app.ts");
  await expect(hints).toHaveCount(0);
});
