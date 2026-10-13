import { expect, test } from "@playwright/test";

const snippet =
  "const greeting = '<script>alert(1)</script>';\nconsole.log(greeting);\n";

test("code controls reveal on hover, choose highlighting, and copy literal code", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=content-markdown--code-blocks&viewMode=story",
  );
  const block = page.locator(".markdown pre").first().locator("..");
  const chooser = block.getByRole("combobox", {
    name: "Language",
    exact: true,
  });
  const toolbar = chooser.locator("../..");
  await expect(block.locator("code")).toHaveText(snippet);
  await expect(toolbar).toHaveCSS("opacity", "0");
  await block.hover();
  await expect(toolbar).toHaveCSS("opacity", "1");
  const height = await block.evaluate(
    (node) => node.getBoundingClientRect().height,
  );
  await chooser.click();
  await page.mouse.move(0, 0);
  await expect(toolbar).toHaveCSS("opacity", "1");
  await page.getByRole("option", { name: "plaintext", exact: true }).click();
  await expect(block.locator("[data-language]")).toHaveAttribute(
    "data-language",
    "plaintext",
  );
  await expect(block.locator(".hljs-keyword")).toHaveCount(0);
  await block.hover();
  await block.getByRole("button", { name: "Copy", exact: true }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    snippet,
  );
  expect(
    await block.evaluate((node) => node.getBoundingClientRect().height),
  ).toBe(height);
  await chooser.click();
  await page.getByRole("option", { name: "javascript", exact: true }).click();
  await expect(block.locator(".hljs-keyword")).toHaveText("const");
  await expect(page.locator(".markdown script")).toHaveCount(0);
  await expect(page.locator(".markdown a")).toHaveAttribute("target", "_blank");
  await expect(page.locator(".markdown pre").nth(2)).toHaveText(
    "raw <tag> & text\n",
  );
  expect(errors).toEqual([]);
});

test("keyboard focus reveals controls and streaming preserves the selected language", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=content-markdown--streaming&viewMode=story");
  const block = page.locator(".markdown pre").first().locator("..");
  const chooser = block.getByRole("combobox", {
    name: "Language",
    exact: true,
  });
  await chooser.focus();
  await expect(chooser.locator("../..")).toHaveCSS("opacity", "1");
  await chooser.press("ArrowDown");
  await page.getByRole("option", { name: "python", exact: true }).click();
  await page
    .getByRole("button", { name: "Append response", exact: true })
    .click();
  await expect(block.locator("[data-language]")).toHaveAttribute(
    "data-language",
    "python",
  );
  await expect(block.locator("code")).toHaveText(snippet);
  await expect(page.locator(".markdown pre")).toHaveCount(4);
});
