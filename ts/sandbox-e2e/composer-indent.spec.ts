import { expect, test } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

test("Markdown bullets continue indentation, undo atomically, and leave code and modified Enter alone", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("  \t- nested");
  await input.press("Enter");
  await expect(input).toHaveValue("  \t- nested\n  \t- ");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(17);
  await input.press("Control+z");
  await expect(input).toHaveValue("  \t- nested");
  await input.press("Control+Shift+z");
  await expect(input).toHaveValue("  \t- nested\n  \t- ");
  await input.press("Enter");
  await expect(input).toHaveValue("  \t- nested\n  \t");
  await input.fill("- one two");
  await input.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(5, 6));
  await input.press("Enter");
  await expect(input).toHaveValue("- one\n- two");
  await input.press("Shift+Enter");
  await expect(input).toHaveValue("- one\n- \ntwo");
  await input.fill("```\n- code\n```");
  await input.evaluate((el: HTMLTextAreaElement) =>
    el.setSelectionRange(10, 10),
  );
  await input.press("Enter");
  await expect(input).toHaveValue("```\n- code\n\n```");
  await input.fill("- first\n  - nested");
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    "- first\n  - nested",
  );
});

test("Tab indents, Shift+Tab outdents, native undo preserves selection and Ctrl+M releases keyboard focus", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("alpha");
  await input.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(2, 2));
  await input.press("Tab");
  await expect(input).toHaveValue("al  pha");
  await expect(input).toBeFocused();
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(4);
  await input.press("Control+z");
  await expect(input).toHaveValue("alpha");
  await input.fill("one\ntwo\nthree");
  await input.evaluate((el: HTMLTextAreaElement) =>
    el.setSelectionRange(0, 8, "backward"),
  );
  await input.press("Tab");
  await expect(input).toHaveValue("  one\n  two\nthree");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => [
      el.selectionStart,
      el.selectionEnd,
      el.selectionDirection,
    ]),
  ).toEqual([2, 12, "backward"]);
  await input.press("Shift+Tab");
  await expect(input).toHaveValue("one\ntwo\nthree");
  await input.press("Control+z");
  await expect(input).toHaveValue("  one\n  two\nthree");
  await input.press("Control+Shift+z");
  await expect(input).toHaveValue("one\ntwo\nthree");
  await input.fill("keep draft");
  await input.press("Control+m");
  await expect(page.getByRole("status")).toContainText("Move focus");
  await input.press("Tab");
  await expect(
    page.getByRole("combobox", { name: "Model", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(input).toBeFocused();
  await input.press("Control+m");
  await input.press("End");
  await input.press("Tab");
  await expect(input).toHaveValue("keep draft  ");
  await expect(input).toBeFocused();
  await expect(
    page.locator("article.input").filter({ hasText: "keep draft" }),
  ).toHaveCount(0);
  const pasted = "one\ntwo\nthree\nfour";
  await input.fill("");
  await page.evaluate((text) => navigator.clipboard.writeText(text), pasted);
  await input.press("Control+v");
  const token = await input.inputValue();
  await input.press("Home");
  await input.press("Tab");
  await expect(input).toHaveValue("  " + token);
  await input.press("ArrowRight");
  await input.press("Tab");
  await expect(input).toHaveValue("    " + token);
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await input.press("Shift+Tab");
  await expect(input).toHaveValue("  " + token);
  await input.press("Control+z");
  await expect(input).toHaveValue("    " + token);
  await page.locator(".paste-chip").click();
  await expect(
    page.getByRole("dialog", { name: "Paste source" }).locator("pre"),
  ).toHaveText(pasted);
  await page.keyboard.press("Escape");
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    /^ {4}\[Attached file: \/cxz\/assets\/session-1\/upload-\d+\/paste-[a-f0-9]+\.txt — read this file for the full content\]$/,
  );
});

test("code tokens use subdued colors and Tab inserts inside paired fences without moving the caret or line numbers", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.pressSequentially("```");
  await input.press("Tab");
  await expect(input).toHaveValue("```\n  \n```");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(6);
  await input.press("Shift+Tab");
  await expect(input).toHaveValue("```\n\n```");
  await input.fill(
    '```javascript\n// note\nconst name = "hello";\nconst n = 42;\n```',
  );
  await expect(page.locator(".editor-code-line .hljs-comment")).toHaveCSS(
    "color",
    "rgb(105, 123, 109)",
  );
  await expect(
    page.locator(".editor-code-line .hljs-keyword").first(),
  ).toHaveCSS("color", "rgb(152, 127, 168)");
  await expect(page.locator(".editor-code-line .hljs-string")).toHaveCSS(
    "color",
    "rgb(131, 156, 123)",
  );
  await expect(page.locator(".editor-code-line .hljs-number")).toHaveCSS(
    "color",
    "rgb(173, 141, 107)",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => {
        const lines = [...document.querySelectorAll(".editor-line")];
        const numbers = [
          ...document.querySelectorAll(".editor-gutter > div > div"),
        ];
        return Math.max(
          ...lines.map((line, index) =>
            Math.abs(
              line.getBoundingClientRect().top -
                numbers[index].getBoundingClientRect().top,
            ),
          ),
        );
      }),
    )
    .toBeLessThan(1);
  await page.screenshot({
    path: "test-results/composer-code-colors.png",
    fullPage: true,
  });
});

test("Question Other shares indentation and focus escape without touching the conversation draft", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  const question = page.locator(".approval");
  const input = question.getByRole("textbox", {
    name: "Other answer: Which environment?",
    exact: true,
  });
  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  await composer.fill("keep draft");
  await input.fill("answer");
  await input.press("Home");
  await input.press("Tab");
  await expect(input).toHaveValue("  answer");
  await input.press("Shift+Tab");
  await expect(input).toHaveValue("answer");
  await input.press("Control+m");
  await input.press("Tab");
  await expect(
    question.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(
    question.getByRole("button", { name: "Submit", exact: true }),
  ).toBeFocused();
  await expect(question).toHaveCount(1);
  await expect(composer).toHaveValue("keep draft");
});
