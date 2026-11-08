import { expect, test } from "@playwright/test";
for (const position of ["adjacent", "space", "selected"] as const) {
  test(`Ctrl+Backspace deletes a text chip at ${position} and supports undo/redo`, async ({
    page,
  }) => {
    await page
      .context()
      .grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.goto(
      "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
    );
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    const chip = page.locator(".paste-chip");
    await input.fill("prefix ");
    await page.evaluate(() =>
      navigator.clipboard.writeText(
        "pasted text with multiple lines\n".repeat(3),
      ),
    );
    await input.press("Control+v");
    await expect(chip).toHaveAttribute("data-send-as", "text");
    if (position === "space") await input.press("Space");
    if (position === "selected")
      await input.evaluate((element: HTMLTextAreaElement) =>
        element.setSelectionRange(7, element.value.length),
      );
    await input.press("Control+Backspace");
    await expect(input).toHaveValue("prefix ");
    await expect(chip).toHaveCount(0);
    for (let repeat = 0; repeat < 2; repeat++) {
      await input.press("Control+z");
      await expect(chip).toHaveCount(1);
      await input.press("Control+Shift+z");
      await expect(input).toHaveValue("prefix ");
      await expect(chip).toHaveCount(0);
    }
  });
}
test("undo removes a newly pasted chip and redo restores its original contents", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const body = "original multiline paste\n".repeat(3);
  await page.evaluate((text) => navigator.clipboard.writeText(text), body);
  await input.press("Control+v");
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await input.press("Control+z");
  await expect(input).toHaveValue("");
  await expect(page.locator(".paste-chip")).toHaveCount(0);
  await input.press("Control+Shift+z");
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await input.press("Control+a");
  await input.press("Control+c");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(body);
  await input.press("End");
  await input.pressSequentially(" tail");
  await expect(input).toHaveValue(/ tail$/);
  await input.press("Control+z");
  await expect(input).not.toHaveValue(/ tail$/);
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await input.press("Control+Shift+z");
  await expect(input).toHaveValue(/ tail$/);
  await expect(page.locator(".paste-chip")).toHaveCount(1);
});
