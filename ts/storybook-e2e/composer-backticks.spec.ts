import { expect, test } from "@playwright/test";

test("backticks pair only before whitespace, wrap selections and preserve undo", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--cursor-memory&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Use word");
  await input.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(4, 4));
  await input.press("`");
  await expect(input).toHaveValue("Use `word");

  await input.fill("Use  word");
  await input.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(4, 4));
  await input.press("`");
  await expect(input).toHaveValue("Use `` word");
  await input.pressSequentially("code`");
  await expect(input).toHaveValue("Use `code` word");

  const draft = "Use 한글 code \t next";
  await input.fill(draft);
  await input.evaluate((el: HTMLTextAreaElement) =>
    el.setSelectionRange(4, el.value.indexOf("next"), "backward"),
  );
  await input.press("`");
  await expect(input).toHaveValue("Use `한글 code` \t next");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) =>
      el.value.slice(el.selectionStart, el.selectionEnd),
    ),
  ).toBe("한글 code");
  await input.press("Control+z");
  await expect(input).toHaveValue(draft);
  await input.press("Control+Shift+z");
  await expect(input).toHaveValue("Use `한글 code` \t next");

  await input.fill("");
  await input.pressSequentially("```");
  await expect(input).toHaveValue("```\n\n```");
});
