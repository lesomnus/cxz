import { expect, test, type Locator, type Page } from "@playwright/test";

async function selection(input: Locator) {
  return input.evaluate((node: HTMLTextAreaElement) => ({
    start: node.selectionStart,
    end: node.selectionEnd,
    direction: node.selectionDirection,
  }));
}

async function pasteChip(page: Page, prefix = "alpha ") {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill(prefix);
  await input.evaluate((node) => {
    const data = new DataTransfer();
    data.setData(
      "text/plain",
      "sample text content long enough to become a chip",
    );
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        bubbles: true,
        cancelable: true,
        clipboardData: data,
      }),
    );
  });
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await input.press("Control+End");
  return input;
}

test("Ctrl+Left crosses a whole chip and continues to the preceding word; Ctrl+Right crosses back", async ({
  page,
}) => {
  const input = await pasteChip(page);
  const end = (await selection(input)).end;
  await input.press("Control+ArrowLeft");
  await expect.poll(() => selection(input)).toMatchObject({ start: 6, end: 6 });
  await input.press("Control+ArrowLeft");
  await expect.poll(() => selection(input)).toMatchObject({ start: 0, end: 0 });
  await input.evaluate((node: HTMLTextAreaElement) =>
    node.setSelectionRange(6, 6),
  );
  await input.press("Control+ArrowRight");
  await expect.poll(() => selection(input)).toMatchObject({ start: end, end });
});

test("Ctrl+Shift+Left selects the chip atomically and preserves the backward anchor", async ({
  page,
}) => {
  const input = await pasteChip(page);
  const end = (await selection(input)).end;
  await input.press("Control+Shift+ArrowLeft");
  await expect
    .poll(() => selection(input))
    .toEqual({ start: 6, end, direction: "backward" });
  await input.press("Control+Shift+ArrowLeft");
  await expect
    .poll(() => selection(input))
    .toEqual({ start: 0, end, direction: "backward" });
  await input.press("Control+Shift+ArrowRight");
  const contracted = await selection(input);
  expect(contracted.end).toBe(end);
  expect(contracted.direction).toBe("backward");
  expect(contracted.start).toBeGreaterThan(0);
  expect(contracted.start).toBeLessThanOrEqual(6);
  await input.evaluate(
    (node: HTMLTextAreaElement, end) =>
      node.setSelectionRange(6, end, "backward"),
    end,
  );
  await input.press("Control+Shift+ArrowRight");
  await expect.poll(() => selection(input)).toMatchObject({ start: end, end });
});

test("word movement across adjacent chips progresses one chip at a time without editing the draft", async ({
  page,
}) => {
  const input = await pasteChip(page, "");
  const firstEnd = (await selection(input)).end;
  await input.evaluate((node) => {
    const data = new DataTransfer();
    data.setData(
      "text/plain",
      "another sample long enough to become a second chip",
    );
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        bubbles: true,
        cancelable: true,
        clipboardData: data,
      }),
    );
  });
  await expect(page.locator(".paste-chip")).toHaveCount(2);
  await input.press("Control+End");
  const original = await input.inputValue();
  const end = (await selection(input)).end;
  await input.press("Control+ArrowLeft");
  await expect
    .poll(() => selection(input))
    .toMatchObject({ start: firstEnd, end: firstEnd });
  await input.press("Control+ArrowLeft");
  await expect.poll(() => selection(input)).toMatchObject({ start: 0, end: 0 });
  await input.press("Control+ArrowRight");
  await expect
    .poll(() => selection(input))
    .toMatchObject({ start: firstEnd, end: firstEnd });
  await input.press("Control+ArrowRight");
  await expect.poll(() => selection(input)).toMatchObject({ start: end, end });
  await expect(input).toHaveValue(original);
  await expect(page.locator(".paste-chip")).toHaveCount(2);
});
