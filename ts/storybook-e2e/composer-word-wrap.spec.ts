import { expect, test } from "@playwright/test";

test("Alt+Z shares the menu's persistent wrap setting without changing the draft or selection", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 900 });
  await page.goto(
    "/iframe.html?id=conversation-composer--cursor-memory&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const editor = page.locator(".composer-editor");
  const text = "keep this draft " + "longword".repeat(40);
  await input.fill(text);
  await input.evaluate((node: HTMLTextAreaElement) =>
    node.setSelectionRange(2, 9, "backward"),
  );
  const element = await input.elementHandle();
  await input.press("Alt+z");
  await expect(editor).toHaveAttribute("data-word-wrap", "false");
  await expect(input).toHaveAttribute("wrap", "off");
  await expect(input).toBeFocused();
  await expect(input).toHaveValue(text);
  expect(
    await input.evaluate((node: HTMLTextAreaElement) => [
      node.selectionStart,
      node.selectionEnd,
      node.selectionDirection,
    ]),
  ).toEqual([2, 9, "backward"]);
  expect(
    await input.evaluate((node, original) => node === original, element),
  ).toBe(true);

  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  const options = page.getByRole("toolbar", {
    name: "Editor options",
    exact: true,
  });
  const wrap = options.getByRole("button", { name: "Word wrap", exact: true });
  await expect(wrap).toHaveAttribute("aria-keyshortcuts", "Alt+Z");
  await expect(wrap).toHaveAttribute("aria-pressed", "false");
  await expect(options.getByRole("button")).toHaveCount(2);
  for (const button of await options.getByRole("button").all()) {
    const bounds = (await button.boundingBox())!;
    expect(bounds.width).toBeCloseTo(24, 1);
    expect(bounds.height).toBeCloseTo(24, 1);
  }
  await wrap.click();
  await expect(wrap).toHaveAttribute("aria-pressed", "true");
  await expect(input).toHaveAttribute("wrap", "soft");
  await expect(options).toBeVisible();
  await page.keyboard.press("Escape");
  await input.focus();
  await input.press("Alt+z");
  await expect(input).toHaveAttribute("wrap", "off");
  await input.evaluate((node) => {
    for (const extra of [
      { repeat: true },
      { ctrlKey: true },
      { metaKey: true },
      { shiftKey: true },
      { isComposing: true },
    ])
      node.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "z",
          code: "KeyZ",
          altKey: true,
          bubbles: true,
          cancelable: true,
          ...extra,
        }),
      );
  });
  await expect(input).toHaveAttribute("wrap", "off");
  await expect(input).toHaveValue(text);
  await page.reload();
  await expect(input).toHaveValue(text);
  await expect(input).toHaveAttribute("wrap", "off");
  // A physical Alt+Z also works on layouts where Alt changes the key value.
  await input.evaluate((node) =>
    node.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "Ω",
        code: "KeyZ",
        altKey: true,
        bubbles: true,
        cancelable: true,
      }),
    ),
  );
  await expect(input).toHaveAttribute("wrap", "soft");
});

test("toggling wrapping preserves native Undo and Redo", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=conversation-composer--cursor-memory&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("keep");
  await input.press("End");
  await page.evaluate(() => navigator.clipboard.writeText(" appended"));
  await input.press("Control+v");
  await expect(input).toHaveValue("keep appended");
  await input.press("Alt+z");
  await input.press("Control+z");
  await expect(input).toHaveValue("keep");
  await input.press("Control+Shift+z");
  await expect(input).toHaveValue("keep appended");
});
