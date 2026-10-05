import { test, expect } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});
test("compact monochrome workspace, aligned composer and release-triggered buttons", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await expect(
    page.getByRole("navigation", { name: "Resources" }),
  ).toBeVisible();
  const composer = page.locator(".composer-input");
  const box = (await composer.boundingBox())!;
  const main = (await page.locator(".conversation").boundingBox())!;
  expect(box.width).toBeLessThanOrEqual(620);
  expect((await page.locator(".composer").boundingBox())!.width).toBe(600);
  expect(
    Math.abs(box.x + box.width / 2 - main.x - main.width / 2),
  ).toBeLessThan(1);
  for (const content of await page
    .locator(".transcript-row > article, .transcript-row > details")
    .all()) {
    const bounds = (await content.boundingBox())!;
    expect(bounds.x).toBeGreaterThanOrEqual(box.x + 10 - 1);
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(
      box.x + box.width - 10 + 1,
    );
  }
  const row = page.locator(".tree-project").first();
  await expect(row).toHaveCSS("border-top-width", "0px");
  await expect(row).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await row.hover();
  await expect(row).toHaveCSS("border-top-width", "0px");
  await expect(row).toHaveCSS("background-color", "rgb(40, 40, 40)");
  await expect(page.locator(".composer-meta")).toContainText("sandbox-claude");
  await expect(page.locator(".effort-field .meta-value")).toHaveText("high");
  await expect(
    page.getByRole("img", { name: "Remaining quota 70%" }),
  ).toHaveText("⣿⣿⣿⣿⣿⣤⣀⣀");
  await page.locator(".context-popover").hover();
  await expect(
    page.getByRole("tooltip", { name: "24K/200K", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".context-donut .context-fill")).toHaveAttribute(
    "stroke-dasharray",
    "0.12 1",
  );
  const send = page.getByRole("button", { name: "Send", exact: true });
  const sendBox = (await send.boundingBox())!;
  expect(sendBox.x + sendBox.width).toBeLessThan(box.x + box.width);
  expect(sendBox.y + sendBox.height).toBeLessThan(box.y);
  const wrapperBox = (await page.locator(".composer-wrapper").boundingBox())!;
  expect(box.width).toBe(
    (await page.locator(".composer").boundingBox())!.width + 20,
  );
  expect(wrapperBox.x - box.x).toBe(4);
  expect(box.x + box.width - wrapperBox.x - wrapperBox.width).toBe(4);
  const toolbarBox = (await page.locator(".composer-toolbar").boundingBox())!;
  expect(toolbarBox.height).toBe(28);
  expect(sendBox.width).toBe(32);
  expect(sendBox.height).toBe(24);
  expect(sendBox.y - toolbarBox.y).toBe(2);
  expect(toolbarBox.y + toolbarBox.height - sendBox.y - sendBox.height).toBe(2);
  expect(toolbarBox.x + toolbarBox.width - sendBox.x - sendBox.width).toBe(2);
  expect(box.y + box.height).toBe(wrapperBox.y + wrapperBox.height);
  await expect(page.locator(".composer-wrapper")).toHaveCSS("padding", "0px");
  await expect(page.locator(".composer-wrapper")).toHaveCSS(
    "background-color",
    "rgb(25, 25, 25)",
  );
  await expect(send).toHaveCSS("border-top-width", "0px");
  await expect(page.locator(".composer-wrapper")).toHaveCSS(
    "border-radius",
    "5px",
  );
  await expect(composer).toHaveCSS("border-radius", "5px");
  await expect(send).toHaveCSS("border-radius", "2px");
  const message = page.getByRole("textbox", { name: "Message", exact: true });
  await message.fill("Release to send");
  await send.hover();
  await page.mouse.down();
  await expect(send.locator(".button-content")).toHaveCSS(
    "transform",
    "matrix(0.95, 0, 0, 0.95, 0, 0)",
  );
  await expect(
    page.locator("article.input").filter({ hasText: "Release to send" }),
  ).toHaveCount(0);
  // Releasing outside the button cancels the action using native click behavior.
  await page.mouse.move(box.x - 20, box.y - 20);
  await page.mouse.up();
  await expect(message).toHaveValue("Release to send");
  await send.hover();
  await page.mouse.down();
  await page.mouse.up();
  await expect(
    page.locator("article.input").filter({ hasText: "Release to send" }),
  ).toHaveCount(1);
  await expect(send.locator(".button-content")).toHaveCSS(
    "transform",
    "matrix(1, 0, 0, 1, 0, 0)",
  );
  await page.getByRole("button", { name: "Projects view" }).click();
  await expect(page.locator(".resource-view")).toBeVisible();
  await page
    .locator(".project-row")
    .filter({ hasText: "Secondary project" })
    .click();
  await expect(page.locator(".session-tree-group")).toHaveCount(2);
  await expect(page.locator(".tree-session")).toHaveCount(8);
  await page.locator(".tree-session").filter({ hasText: "session-8" }).click();
  await expect(message).toBeVisible();
  await page.screenshot({
    path: "test-results/sandbox-desktop.png",
    fullPage: true,
  });
});
