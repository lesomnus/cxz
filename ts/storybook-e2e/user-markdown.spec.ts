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

for (const theme of ["dark", "light"]) {
  test(`user code uses a separate surface palette in ${theme} mode`, async ({
    page,
    context,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.goto(
      `/iframe.html?id=conversation-eventcards--code-palette-comparison&viewMode=story&globals=theme:${theme}`,
    );
    const input = page.locator("article.input .markdown");
    const response = page.locator("article.response .markdown");
    await expect(input.locator("pre code")).toBeVisible();
    await expect(response.locator("pre code")).toBeVisible();
    const color = (selector: string, property: "color" | "backgroundColor") =>
      page
        .locator(selector)
        .first()
        .evaluate((el, property) => getComputedStyle(el)[property], property);
    expect(
      await color("article.input .markdown p code", "backgroundColor"),
    ).not.toBe(
      await color("article.response .markdown p code", "backgroundColor"),
    );
    expect(
      await color("article.input .markdown pre", "backgroundColor"),
    ).not.toBe(
      await color("article.response .markdown pre", "backgroundColor"),
    );
    expect(await color("article.input .hljs-keyword", "color")).not.toBe(
      await color("article.response .hljs-keyword", "color"),
    );
    expect(await input.locator("pre code").textContent()).toBe(
      await response.locator("pre code").textContent(),
    );
    const block = input.locator("pre").locator("..");
    await block.hover();
    await block.getByRole("button", { name: "Copy", exact: true }).click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      await input.locator("pre code").textContent(),
    );
  });
}
