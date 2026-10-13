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
  await expect(chip).toHaveAccessibleName("sample.txt · Uploading…");
  await expect(send).toBeDisabled();
  await input.pressSequentially(" typing");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(chip).toHaveAccessibleName("sample.txt · 5 B");
  expect(
    await input.evaluate((node: HTMLTextAreaElement) =>
      node.value.slice(0, node.selectionStart).endsWith(" typing"),
    ),
  ).toBe(true);
  await expect(send).toBeEnabled();
  await expect(input).toHaveValue(/^Before sample.txt · 5 B typing after$/);
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
  await expect(chip).toHaveAccessibleName("retry.txt · Upload failed.");
  await expect(send).toBeDisabled();
  await chip.click();
  await page.getByRole("button", { name: "Retry upload", exact: true }).click();
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(chip).toHaveAccessibleName("retry.txt · 5 B");
  await expect(
    page.getByRole("dialog", { name: "Attachment", exact: true }),
  ).toContainText("/cxz/assets/storybook/upload/retry.txt");
  await expect(input).toHaveValue("retry.txt · 5 B");
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

test("large pasted text shows upload status then UTF-8 bytes without internal names", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--file-uploads&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const text = "한글😀\n".repeat(3000) + "x".repeat(1500);
  const bytes = Buffer.byteLength(text, "utf8");
  await input.fill("Before  after");
  await input.evaluate((node: HTMLTextAreaElement, text) => {
    node.focus();
    node.setSelectionRange(7, 7);
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", text);
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  }, text);
  const chip = page.locator(".composer .paste-chip");
  const label = chip.locator(".paste-chip-label");
  await expect(chip).toHaveAccessibleName("[Paste 000%]");
  await expect(label).toHaveText("[Paste 000%]");
  await expect(chip.locator(".paste-chip-token")).toHaveCount(0);
  await expect(label).toHaveText("[Paste 012%]");
  await expect(label.locator("[data-paste-leading]")).toHaveText("0");
  expect(
    await label
      .locator("[data-paste-leading]")
      .evaluate((node) => getComputedStyle(node).color),
  ).not.toBe(await label.evaluate((node) => getComputedStyle(node).color));
  await input.pressSequentially(" typing");
  const before = await input.inputValue();
  const selection = await input.evaluate(
    (node: HTMLTextAreaElement) => node.selectionStart,
  );
  const bounds = await chip.boundingBox();
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  expect(bytes).toBe(34500);
  await expect(label).toHaveText("[Text 34.5K]");
  await expect(chip).toHaveAccessibleName("[Text 34.5K]");
  await expect(chip).toHaveAttribute("title", /34,500 B/);
  await expect(input).toHaveValue(
    before.replace(/\[Paste \d{3}%\]/, "[Text 34.5K]"),
  );
  expect(
    await input.evaluate((node: HTMLTextAreaElement) => node.selectionStart),
  ).toBe(selection);
  expect(await chip.boundingBox()).toEqual(bounds);
  await chip.click();
  await expect(page.getByRole("dialog")).toContainText(text.trim());
});

test("upload acknowledgement during IME does not interrupt composition or expose references", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--file-uploads&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.evaluate((node) => {
    node.focus();
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", "x".repeat(900));
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await expect(input).toHaveValue("[Paste 000%]");
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Input.imeSetComposition", {
    text: "하",
    selectionStart: 1,
    selectionEnd: 1,
  });
  await expect(page.locator(".paste-chip")).toHaveAttribute(
    "data-upload-state",
    "ready",
  );
  await expect(input).toHaveValue("[Paste 000%]하");
  await expect(page.locator(".paste-chip-label")).toHaveText("[Paste 000%]");
  await cdp.send("Input.insertText", { text: "한" });
  await expect(input).toHaveValue("[Text 0900B]한");
  await expect(page.locator("[data-paste-leading]")).toHaveText("0");
});
