import { expect, test } from "@playwright/test";

test("paste chips use icon modes and a ten-character flattened hover preview without opening a dialog", async ({
  page,
}, testInfo) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.evaluate((node) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData(
      "text/plain",
      "first\nline\r\nthird\nfourth\n".repeat(60),
    );
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  const chip = page.locator(".paste-chip");
  await expect(
    chip.getByRole("radio", { name: "File", exact: true }),
  ).toBeChecked();
  await expect(chip.getByText("Text", { exact: true })).toHaveCount(0);
  await expect(chip.getByText("File", { exact: true })).toHaveCount(0);
  await chip.hover();
  await expect(page.getByRole("tooltip")).toHaveText("first line…");
  expect(
    await page.getByRole("tooltip").evaluate((el) => el.childElementCount),
  ).toBe(0);
  await page.screenshot({ path: testInfo.outputPath("paste-chip-hint.png") });
  await chip.locator(".paste-chip-label").click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const bounds = (await chip.boundingBox())!;
  await chip.getByRole("radio", { name: "Text", exact: true }).check();
  await expect(chip).toHaveAttribute("data-send-as", "text");
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeEnabled();
  await expect(chip.locator(".paste-chip-label")).not.toContainText("%");
  await chip.getByRole("radio", { name: "File", exact: true }).check();
  await expect(chip).toHaveAttribute("data-send-as", "file");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  const after = (await chip.boundingBox())!;
  expect(Math.abs(after.width - bounds.width)).toBeLessThan(0.125);
  await chip.getByRole("radio", { name: "File", exact: true }).focus();
  await page.keyboard.press("ArrowLeft");
  await expect(chip).toHaveAttribute("data-send-as", "text");
});
