import { expect, test } from "@playwright/test";

for (const mode of ["text", "file"] as const) {
  test(`${mode} pasted chip delivery reaches the session as selected`, async ({
    page,
  }) => {
    await page.goto("/sandbox.html");
    await expect(
      page.getByRole("heading", { name: "Current status" }),
    ).toBeVisible({ timeout: 45000 });
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    const body = "Original pasted text\nsecond\nthird\nfourth";
    await input.evaluate((node, body) => {
      node.focus();
      const clipboardData = new DataTransfer();
      clipboardData.setData("text/plain", body);
      node.dispatchEvent(
        new ClipboardEvent("paste", {
          clipboardData,
          bubbles: true,
          cancelable: true,
        }),
      );
    }, body);
    const chip = page.locator(
      ".composer-editor:not(.composer-send-ghost) .paste-chip",
    );
    await chip.getByRole("radio", { name: "Text", exact: true }).check();
    if (mode === "file") {
      await chip.getByRole("radio", { name: "File", exact: true }).check();
      await expect(chip).toHaveAttribute("data-upload-state", "ready");
    }
    await page.getByRole("button", { name: "Send", exact: true }).click();
    const message = page.locator("article.input .message-body").last();
    if (mode === "text") await expect(message).toHaveText(body);
    else {
      await expect(message.locator("p")).toHaveText(
        /^\[Attached file: \/cxz\/assets\/.+\/paste-[a-f0-9]+\.txt — read this file for the full content\]$/,
      );
      await expect(message).not.toContainText("Original pasted text");
    }
  });
}
