import { expect, test } from "@playwright/test";

test("session entry focuses the editor and restores its range after switching and refreshing", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-composer--cursor-memory&viewMode=story",
  );
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(editor).toBeFocused();
  await editor.fill("First session draft\nSecond line");
  await editor.evaluate((node: HTMLTextAreaElement) =>
    node.setSelectionRange(2, 8, "backward"),
  );
  await page.getByRole("tab", { name: "two", exact: true }).click();
  await expect(editor).toBeFocused();
  await editor.fill("Second session");
  await editor.press("ArrowLeft");
  const second = await editor.evaluate(
    (node: HTMLTextAreaElement) => node.selectionStart,
  );
  await page.getByRole("tab", { name: "one", exact: true }).click();
  await expect(editor).toBeFocused();
  await expect(editor).toHaveValue("First session draft\nSecond line");
  expect(
    await editor.evaluate((node: HTMLTextAreaElement) => [
      node.selectionStart,
      node.selectionEnd,
      node.selectionDirection,
    ]),
  ).toEqual([2, 8, "backward"]);
  await page.reload();
  await expect(editor).toBeFocused();
  expect(
    await editor.evaluate((node: HTMLTextAreaElement) => [
      node.selectionStart,
      node.selectionEnd,
      node.selectionDirection,
    ]),
  ).toEqual([2, 8, "backward"]);
  await page.getByRole("tab", { name: "two", exact: true }).click();
  await expect(editor).toBeFocused();
  expect(
    await editor.evaluate((node: HTMLTextAreaElement) => node.selectionStart),
  ).toBe(second);
  expect(errors).toEqual([]);
});
