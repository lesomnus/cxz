import { expect, test } from "@playwright/test";

test("pending uploads block send without moving the caret and deletion releases the blocker", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--file-uploads&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const send = page.getByRole("button", { name: "Send", exact: true });
  await input.fill("Before  after");
  await input.evaluate((node: HTMLTextAreaElement) => {
    node.focus();
    node.setSelectionRange(7, 7);
  });
  await input.evaluate((node) => {
    const data = new DataTransfer();
    data.items.add(new File(["hello"], "sample.txt"));
    node.dispatchEvent(
      new DragEvent("drop", {
        dataTransfer: data,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  const chip = page.locator(".composer .paste-chip");
  await expect(chip).toHaveAttribute("data-upload-state", "uploading");
  await expect(send).toBeDisabled();
  await input.pressSequentially(" typing");
  const position = await input.evaluate(
    (node: HTMLTextAreaElement) => node.selectionStart,
  );
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  expect(
    await input.evaluate((node: HTMLTextAreaElement) => node.selectionStart),
  ).toBe(position);
  await expect(send).toBeEnabled();
  await expect(input).toHaveValue(
    /^Before \[File .*sample.txt\] typing after$/,
  );
  await chip.click();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(input).toHaveValue("Before  typing after");
  await expect(send).toBeEnabled();
});

test("failed uploads retry in place and deleting a pending chip cancels it", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-composer--failed-upload&viewMode=story",
  );
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  const choosing = page.waitForEvent("filechooser");
  await page
    .getByRole("menuitem", { name: "Upload files", exact: true })
    .click();
  await (
    await choosing
  ).setFiles({
    name: "retry.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("retry"),
  });
  const chip = page.locator(".composer .paste-chip");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(chip).toHaveAttribute("data-upload-state", "error");
  await expect(send).toBeDisabled();
  const before = await input.inputValue();
  await chip.click();
  await page.getByRole("button", { name: "Retry upload", exact: true }).click();
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(
    page.getByRole("dialog", { name: "Attachment", exact: true }),
  ).toContainText("/cxz/assets/storybook/upload/retry.txt");
  await expect(input).toHaveValue(before);
  await expect(send).toBeEnabled();
  await page.keyboard.press("Escape");
  await input.fill("keep");
  await input.evaluate((node) => {
    const data = new DataTransfer();
    data.items.add(new File(["cancel"], "cancel.txt"));
    node.dispatchEvent(
      new DragEvent("drop", {
        dataTransfer: data,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await expect(chip).toHaveAttribute("data-upload-state", "uploading");
  await input.fill("keep");
  await expect(send).toBeEnabled();
  await page.waitForTimeout(600);
  await expect(chip).toHaveCount(0);
  expect(errors).toEqual([]);
});
