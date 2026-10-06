import { expect, test, type Page } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

async function openEditor(page: Page) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  return page.getByRole("textbox", { name: "Message", exact: true });
}
async function paste(page: Page, text: string) {
  await page.evaluate((body) => navigator.clipboard.writeText(body), text);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.focus();
  await input.press("Control+v");
}

test("monospace line numbers follow soft wraps, native scrolling and mobile widths", async ({
  page,
}) => {
  const input = await openEditor(page);
  await expect(input).toHaveCSS("resize", "none");
  await expect(input).toHaveCSS("font-family", /monospace/);
  await expect(page.locator(".editor-gutter > div > div")).toHaveText(["1"]);
  await input.fill(`${"한글 text 😀 ".repeat(30)}\n\nthird line\n`);
  await expect(page.locator(".editor-gutter > div > div")).toHaveText([
    "1",
    "2",
    "3",
    "4",
  ]);
  const aligned = async () => {
    await expect
      .poll(() =>
        page.evaluate(() => {
          const numbers = [
            ...document.querySelectorAll(".editor-gutter > div > div"),
          ];
          const lines = [...document.querySelectorAll(".editor-line")];
          const textarea = document.querySelector(
            ".composer textarea",
          ) as HTMLTextAreaElement;
          const content = document.querySelector(".editor-mirror")!;
          return Math.max(
            ...numbers.map((number, index) =>
              Math.abs(
                number.getBoundingClientRect().top -
                  lines[index].getBoundingClientRect().top,
              ),
            ),
            Math.abs(
              textarea.scrollHeight -
                content.getBoundingClientRect().height -
                24,
            ),
          );
        }),
      )
      .toBeLessThan(1);
  };
  await aligned();
  await input.evaluate((el: HTMLTextAreaElement) => {
    el.scrollTop = 50;
    el.dispatchEvent(new Event("scroll"));
  });
  await aligned();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(input).toHaveCSS("font-size", "16px");
  await aligned();
  await input.fill("한글 조합 중");
  await input.dispatchEvent("compositionstart", { data: "중" });
  await input.dispatchEvent("keydown", {
    key: "Enter",
    ctrlKey: true,
    isComposing: true,
  });
  await expect(input).toHaveValue("한글 조합 중");
  await expect(
    page.locator("article.input").filter({ hasText: "한글 조합 중" }),
  ).toHaveCount(0);
  await input.dispatchEvent("compositionend", { data: "중" });
  await expect(page.locator(".editor-surface")).toBeVisible();
});

test("paste chips preserve session drafts and send exact original text inline", async ({
  page,
}) => {
  const input = await openEditor(page);
  await paste(page, "one\ntwo\nthree");
  await expect(input).toHaveValue("one\ntwo\nthree");
  await expect(page.locator(".paste-chip")).toHaveCount(0);
  await input.fill("앞 ");
  const body = "한글 😀\n  two\nthree\nfour\n";
  await paste(page, body);
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await input.press("End");
  await input.pressSequentially(" 뒤");
  const draft = await input.inputValue();
  expect(draft).toMatch(/^앞 \[Paste [0-9a-f]{8} · 5L · \d+B\] 뒤$/);
  await page.locator(".paste-chip").click();
  const preview = page.getByRole("dialog", { name: "붙여넣기 원문" });
  await expect(preview).toBeVisible();
  expect(await preview.locator("pre").textContent()).toBe(body);
  await page.keyboard.press("Escape");
  await expect(preview).not.toBeVisible();
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(input).toHaveValue("");
  await expect(page.locator(".paste-chip")).toHaveCount(0);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-1");
  await expect(input).toHaveValue(draft);
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: "test-results/composer-chip-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(input).toHaveValue("");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    `앞 ${body} 뒤`,
  );
  expect(
    await page.locator("article.input .message-body").last().textContent(),
  ).toBe(`앞 ${body} 뒤`);
});

test("chips are atomic, undoable, previewable and expandable one occurrence at a time", async ({
  page,
}) => {
  const input = await openEditor(page);
  const body = "one\ntwo\nthree\nfour";
  await paste(page, body);
  const token = await input.inputValue();
  await input.press("Backspace");
  await expect(input).toHaveValue("");
  await input.press("Control+z");
  await expect(input).toHaveValue(token);
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await input.press("Control+Shift+z");
  await expect(input).toHaveValue("");
  await input.press("Control+z");
  await input.press("End");
  await input.press("ArrowLeft");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => [
      el.selectionStart,
      el.selectionEnd,
    ]),
  ).toEqual([0, token.length]);
  await input.press("Enter");
  await expect(
    page.getByRole("dialog", { name: "붙여넣기 원문" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await input.fill(`${token}\n${token}`);
  await page.locator(".paste-chip").nth(1).click();
  await page.getByRole("button", { name: "원문 펼치기", exact: true }).click();
  await expect(input).toHaveValue(`${token}\n${body}`);
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(token.length + 1 + body.length);
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await input.press("Control+z");
  await expect(input).toHaveValue(`${token}\n${token}`);
  await page.locator(".paste-chip").first().click();
  await page.getByRole("button", { name: "삭제", exact: true }).click();
  await expect(input).toHaveValue(`\n${token}`);
  await input.evaluate((el: HTMLTextAreaElement) => {
    el.setSelectionRange(5, 10);
    el.dispatchEvent(new Event("select", { bubbles: true }));
  });
  await input.press("Backspace");
  await expect(input).toHaveValue("\n");
  await input.fill("keep draft");
  await paste(page, "x".repeat(1024 * 1024 + 1));
  await expect(page.getByRole("status")).toContainText("1 MiB");
  await expect(input).toHaveValue("keep draft");
});
