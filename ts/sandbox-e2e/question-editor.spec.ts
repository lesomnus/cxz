import { expect, test } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

test("option cards and Other share native selection and multiline paste editing", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  const question = page.locator(".approval");
  const group = question.locator("fieldset");
  await expect(group).toHaveCSS("border-width", "0px");
  await expect(group).toHaveCSS("padding", "0px");
  await expect(group).toHaveCSS("margin", "0px");
  await expect(question.locator(".question-option")).toHaveCount(2);
  await question
    .locator(".question-option")
    .filter({ hasText: "Development" })
    .click();
  const development = question.getByRole("radio", { name: /Development/ });
  await expect(development).toBeChecked();
  await development.focus();
  await development.press("ArrowDown");
  await expect(
    question.getByRole("radio", { name: /Production/ }),
  ).toBeChecked();
  await expect(development).not.toBeChecked();
  await expect(
    question.locator('.question-option[data-selected="true"]'),
  ).toContainText("Production");

  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  const other = question.getByRole("textbox", {
    name: "Other answer: Which environment?",
    exact: true,
  });
  const editor = question.locator(".composer-editor");
  await expect(other).toHaveJSProperty("tagName", "TEXTAREA");
  await expect(other).toHaveCSS("resize", "none");
  expect(
    await other.evaluate((el) => getComputedStyle(el).fontFamily),
  ).toContain("monospace");
  await composer.fill("Keep composer draft");
  await other.fill("prefix");
  await other.press("End");
  await other.press("Enter");
  await expect(other).toHaveValue("prefix\n");
  await expect(editor.locator(".editor-gutter > div > div")).toHaveText([
    "1",
    "2",
  ]);
  await other.press("Control+Enter");
  await expect(question).toHaveCount(1);
  await expect(composer).toHaveValue("Keep composer draft");

  const body = "first line\n第二行\r\nthird line\nfourth line";
  await page.evaluate((text) => navigator.clipboard.writeText(text), body);
  await other.focus();
  await other.press("Control+v");
  const chip = question.locator(".paste-chip");
  await expect(chip).toHaveCount(1);
  await expect(page.locator(".composer .paste-chip")).toHaveCount(0);
  const compact = await other.inputValue();
  await chip.click();
  const preview = page.getByRole("dialog", { name: "Paste source" });
  await expect(preview.locator("pre")).toHaveText(body);
  await expect(page.locator(".question-cards")).toHaveJSProperty("inert", true);
  await preview
    .getByRole("button", { name: "Expand source", exact: true })
    .click();
  await expect(preview).toHaveCount(0);
  // Native textarea values normalize CRLF; raw chip submission retains it.
  const expanded = "prefix\n" + body.replaceAll("\r\n", "\n");
  await expect(other).toHaveValue(expanded);
  await other.press("Control+z");
  await expect(other).toHaveValue(compact);
  await other.press("Control+Shift+z");
  await expect(other).toHaveValue(expanded);
  await other.press("Control+z");
  await expect(other).toHaveValue(compact);
  await expect(composer).toHaveValue("Keep composer draft");
  await page.screenshot({
    path: "test-results/question-option-editor.png",
    fullPage: true,
  });
  await question
    .getByRole("button", { name: "Submit answers", exact: true })
    .click();
  await expect(question).toHaveCount(0);
  const response = page
    .locator(".response .markdown")
    .filter({ hasText: "Your selection was recorded for this preview." });
  await expect(response).toBeVisible();
  const text = (await response.textContent())!;
  expect(JSON.parse(text.slice(text.indexOf("{")))).toEqual({
    environment: { selected: [], other: "prefix\n" + body },
  });
  await expect(composer).toHaveValue("Keep composer draft");
});
