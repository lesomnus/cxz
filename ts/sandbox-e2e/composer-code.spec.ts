import { expect, test } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

test("fenced editor opens inline, detects syntax, closes to prose and sends resolved Markdown", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("before\n");
  await input.pressSequentially("```");
  await expect(input).toHaveValue("before\n```\n");
  const syntax = page.getByRole("combobox", {
    name: "Code syntax 1",
    exact: true,
  });
  await expect(syntax).toHaveValue("auto");
  await input.pressSequentially('{"hello": true, "count": 42}');
  await expect(syntax.locator("option:checked")).toHaveText("Auto · json");
  await expect(page.locator(".editor-code-line").first()).toHaveCSS(
    "background-color",
    "rgb(0, 0, 0)",
  );
  await expect(page.locator(".editor-code-line .hljs-attr")).toHaveCount(2);
  await expect(input).toHaveCSS("z-index", "1");
  await expect(page.locator(".editor-code-controls")).toHaveCSS("z-index", "2");
  await input.dispatchEvent("compositionstart", { data: "중" });
  await expect(page.locator(".editor-code-line").first()).toHaveCSS(
    "visibility",
    "visible",
  );
  await input.dispatchEvent("keydown", {
    key: "Enter",
    ctrlKey: true,
    isComposing: true,
  });
  await expect(input).toHaveValue('before\n```\n{"hello": true, "count": 42}');
  await input.dispatchEvent("compositionend", { data: "중" });
  await syntax.selectOption("javascript");
  await expect(input).toHaveValue(
    'before\n```javascript\n{"hello": true, "count": 42}',
  );
  await input.press("Control+z");
  await expect(input).toHaveValue('before\n```\n{"hello": true, "count": 42}');
  await syntax.selectOption("auto");
  await page
    .getByRole("button", { name: "Close code block 1", exact: true })
    .click();
  await expect(input).toBeFocused();
  await input.pressSequentially("after");
  await expect(input).toHaveValue(
    'before\n```\n{"hello": true, "count": 42}\n```\nafter',
  );
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(input).toHaveValue("");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-1");
  await expect(syntax).toHaveValue("auto");
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    'before\n```json\n{"hello": true, "count": 42}\n```\nafter',
  );
});

test("multiple blocks, paste chips, highlighting and native line geometry stay aligned at mobile widths", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const body = '{\n  "hello": true,\n  "count": 42\n}';
  await input.fill("```\n");
  await page.evaluate((text) => navigator.clipboard.writeText(text), body);
  await input.focus();
  await input.press("Control+v");
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await expect(
    page
      .getByRole("combobox", { name: "Code syntax 1" })
      .locator("option:checked"),
  ).toHaveText("Auto · json");
  await page.locator(".paste-chip").click();
  await expect(
    page.getByRole("dialog", { name: "붙여넣기 원문" }).locator("pre"),
  ).toHaveText(body);
  await page.keyboard.press("Escape");
  const draft = await input.inputValue();
  await input.fill(
    draft +
      "\n```\nplain\n```python\n" +
      "print('한글 😀 & <img src=x onerror=alert(1)>') ".repeat(15) +
      "\n```\nend",
  );
  await expect(
    page.getByRole("combobox", { name: "Code syntax 2" }),
  ).toHaveValue("python");
  await expect(page.locator(".editor-surface img")).toHaveCount(0);
  const aligned = async () => {
    await expect
      .poll(() =>
        page.evaluate(() => {
          const lines = [...document.querySelectorAll(".editor-line")];
          const numbers = [
            ...document.querySelectorAll(".editor-gutter > div > div"),
          ];
          const textarea = document.querySelector(
            ".composer textarea",
          ) as HTMLTextAreaElement;
          return Math.max(
            Math.abs(
              textarea.scrollHeight -
                document
                  .querySelector(".editor-mirror")!
                  .getBoundingClientRect().height -
                24,
            ),
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
  };
  await aligned();
  await page.setViewportSize({ width: 390, height: 844 });
  await aligned();
  await input.evaluate((el: HTMLTextAreaElement) => {
    el.scrollTop = 40;
    el.dispatchEvent(new Event("scroll"));
  });
  await aligned();
  await input.evaluate((el: HTMLTextAreaElement) => {
    el.scrollTop = 0;
    el.dispatchEvent(new Event("scroll"));
  });
  await page.screenshot({
    path: "test-results/composer-code-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.locator("article.input .message-body").last(),
  ).toContainText("```json\n" + body + "\n```");
});

test("unfinished code blocks are completed on send without changing draft source", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("```py\nprint('hello')");
  await expect(
    page.getByRole("combobox", { name: "Code syntax 1" }),
  ).toHaveValue("python");
  await input.press("Control+Enter");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    "```python\nprint('hello')\n```",
  );
});

test("Question Other uses the same code editor and sends fenced syntax without submitting the conversation", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  const question = page.locator(".approval");
  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  const other = question.getByRole("textbox", {
    name: "Other answer: Which environment?",
    exact: true,
  });
  await composer.fill("Keep my draft");
  await other.pressSequentially("```");
  await other.pressSequentially('{"environment": "test"}');
  await expect(
    question
      .getByRole("combobox", { name: "Code syntax 1" })
      .locator("option:checked"),
  ).toHaveText("Auto · json");
  await other.press("Control+Enter");
  await expect(question).toHaveCount(1);
  await expect(composer).toHaveValue("Keep my draft");
  await question
    .getByRole("button", { name: "Submit answers", exact: true })
    .click();
  await expect(question).toHaveCount(0);
  const response = page
    .locator(".response")
    .filter({ hasText: "Your selection was recorded for this preview." });
  await expect(response).toBeVisible();
  await response.hover();
  await response.getByRole("button", { name: "Copy", exact: true }).click();
  const text = await page.evaluate(() => navigator.clipboard.readText());
  expect(JSON.parse(text.slice(text.indexOf("{"))).environment.other).toBe(
    '```json\n{"environment": "test"}\n```',
  );
  await expect(composer).toHaveValue("Keep my draft");
});
