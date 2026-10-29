import { expect, test } from "@playwright/test";

test("files entering anywhere on screen reveal the editor target without changing the draft", async ({
  page,
}, testInfo) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--file-uploads&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const editor = page.locator(".composer-editor");
  const overlay = page.getByRole("status").filter({
    hasText: "Drop files or folders here",
  });
  await input.fill("Before  after");
  await input.evaluate((node: HTMLTextAreaElement) => {
    node.focus();
    node.setSelectionRange(7, 7);
  });
  const data = await page.evaluateHandle(() => {
    const data = new DataTransfer();
    data.items.add(new File(["hello"], "sample.txt"));
    return data;
  });
  await page.dispatchEvent("body", "dragenter", { dataTransfer: data });
  await expect(overlay).toBeVisible();
  await expect(overlay).toHaveCSS("pointer-events", "none");
  await expect(editor).toHaveAttribute("data-file-drag", "true");
  await expect(input).toBeFocused();
  expect(
    await input.evaluate((node: HTMLTextAreaElement) => node.selectionStart),
  ).toBe(7);
  await expect(input).toHaveValue("Before  after");
  const box = (await editor.boundingBox())!;
  expect(await overlay.boundingBox()).toEqual(box);
  await page.screenshot({ path: testInfo.outputPath("file-drop-dark.png") });
  await page.evaluate(() => (document.documentElement.dataset.theme = "light"));
  await page.screenshot({ path: testInfo.outputPath("file-drop-light.png") });

  // Browser ordering: enter the next child before leaving the previous node.
  await input.dispatchEvent("dragenter", { dataTransfer: data });
  await page.dispatchEvent("body", "dragleave", { dataTransfer: data });
  await expect(overlay).toBeVisible();
  const gutter = editor.locator(".editor-gutter");
  await gutter.dispatchEvent("dragenter", { dataTransfer: data });
  await input.dispatchEvent("dragleave", { dataTransfer: data });
  await expect(overlay).toBeVisible();
  await gutter.dispatchEvent("dragleave", { dataTransfer: data });
  await expect(overlay).toHaveCount(0);

  // A late-mounted editor can detect a drag already in progress from dragover.
  await page.dispatchEvent("body", "dragover", { dataTransfer: data });
  await expect(overlay).toBeVisible();
  await input.dispatchEvent("drop", { dataTransfer: data });
  await expect(overlay).toHaveCount(0);
  await expect(page.locator(".paste-chip")).toHaveAttribute(
    "data-upload-state",
    "ready",
  );
  await expect(page.locator(".paste-chip")).toHaveAccessibleName(
    "sample.txt · 0005B",
  );
  await data.dispose();
});

test("text drags are ignored and canceling or dropping outside always clears the file target", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--file-uploads&viewMode=story",
  );
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  const overlay = page.locator(".composer-file-drop-overlay");
  const textPrevented = await page.evaluate(() => {
    const data = new DataTransfer();
    data.setData("text/plain", "move this text");
    for (const type of ["dragenter", "dragover", "drop"])
      if (
        !document.body.dispatchEvent(
          new DragEvent(type, {
            bubbles: true,
            cancelable: true,
            dataTransfer: data,
          }),
        )
      )
        return true;
    return false;
  });
  expect(textPrevented).toBe(false);
  await expect(overlay).toHaveCount(0);
  const data = await page.evaluateHandle(() => {
    const data = new DataTransfer();
    data.items.add(new File(["hello"], "outside.txt"));
    return data;
  });
  for (const end of ["escape", "blur", "dragend", "drop"]) {
    await page.dispatchEvent("body", "dragenter", { dataTransfer: data });
    await expect(overlay).toBeVisible();
    if (end === "escape") await page.keyboard.press("Escape");
    else if (end === "blur")
      await page.evaluate(() => window.dispatchEvent(new Event("blur")));
    else if (end === "dragend")
      await page.dispatchEvent("body", end, { dataTransfer: data });
    else {
      expect(
        await page.evaluate(
          (data) =>
            !document.body.dispatchEvent(
              new DragEvent("drop", {
                bubbles: true,
                cancelable: true,
                dataTransfer: data,
              }),
            ),
          data,
        ),
      ).toBe(true);
    }
    await expect(overlay).toHaveCount(0);
  }
  await expect(page.locator(".paste-chip")).toHaveCount(0);
  await data.dispose();
});

test("read-only and unavailable upload editors do not offer a file target", async ({
  page,
}) => {
  for (const story of ["empty", "pending-send&args=uploadMode:ready"]) {
    await page.goto(
      `/iframe.html?id=conversation-composer--${story}&viewMode=story`,
    );
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await expect(input).toBeVisible();
    if (story.startsWith("pending-send"))
      await expect(input).toHaveAttribute("readonly", "");
    await page.evaluate(() => {
      const data = new DataTransfer();
      data.items.add(new File(["hello"], "sample.txt"));
      document.body.dispatchEvent(
        new DragEvent("dragenter", {
          bubbles: true,
          dataTransfer: data,
        }),
      );
    });
    await expect(page.locator(".composer-file-drop-overlay")).toHaveCount(0);
  }
});
