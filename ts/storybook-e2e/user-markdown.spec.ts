import { expect, test } from "@playwright/test";

test("user messages render Markdown without extra container padding", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-eventcards--user-input&viewMode=story",
  );
  const body = page.locator("article.input .message-body");
  await expect(body.locator("strong")).toHaveText("conversation cards");
  await expect(body.locator("li")).toHaveText([
    "Use the shared controls",
    "Include model · effort metadata",
  ]);
  await expect(body.locator("code")).toHaveText("model · effort");
  await expect(body.locator(".markdown")).toHaveCSS("padding-left", "0px");
  await expect(body.locator(".markdown")).toHaveCSS("padding-right", "0px");
});
