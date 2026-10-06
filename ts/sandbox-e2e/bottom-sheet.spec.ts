import { expect, test, type Page } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

async function open(page: Page) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  return page.getByRole("textbox", { name: "Message", exact: true });
}
async function paste(page: Page, body: string) {
  await page.evaluate((text) => navigator.clipboard.writeText(text), body);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.focus();
  await input.press("Control+v");
}

test("response copy appears on hover and keyboard focus without changing layout", async ({
  page,
}) => {
  await open(page);
  const response = page.locator(".response").first();
  const copy = response.locator(".copy-control");
  const before = await response.boundingBox();
  await page.mouse.move(0, 0);
  await expect(copy).toHaveCSS("opacity", "0");
  await response.hover();
  await expect(copy).toHaveCSS("opacity", "1");
  await page.mouse.move(0, 0);
  await expect(copy).toHaveCSS("opacity", "0");
  await response.getByRole("button", { name: "Copy", exact: true }).focus();
  await expect(copy).toHaveCSS("opacity", "1");
  await expect(response.getByRole("tooltip")).toHaveText("copy");
  expect(await response.boundingBox()).toEqual(before);
});

test("event details and paste previews stack above the composer without moving the transcript", async ({
  page,
}) => {
  const input = await open(page);
  await paste(page, "first\nsecond\nthird\nfourth");
  await input.press("End");
  await input.pressSequentially(" ");
  await paste(page, "another\npaste\nbody\nhere");
  const draft = await input.inputValue();
  const trigger = page.locator(".transcript-row > .event-detail").last();
  await trigger.scrollIntoViewIfNeeded();
  const before = await page
    .locator(".transcript")
    .evaluate((el) => ({ top: el.scrollTop, height: el.scrollHeight }));
  await trigger.click();
  const event = page.locator('.bottom-sheet[data-active="true"]');
  await expect(event).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  const eventId = await event.getAttribute("data-sheet");
  const wrapper = (await page.locator(".composer-wrapper").boundingBox())!;
  const card = (await event.boundingBox())!;
  expect(Math.abs(card.y + card.height - wrapper.y)).toBeLessThan(1);
  const eventRow = await trigger.boundingBox();
  expect(
    await page
      .locator(".transcript")
      .evaluate((el) => ({ top: el.scrollTop, height: el.scrollHeight })),
  ).toEqual(before);
  await expect(page.locator(".transcript-row details")).toHaveCount(0);

  await page.locator(".paste-chip").first().click();
  const preview = page.getByRole("dialog", { name: "붙여넣기 원문" });
  await expect(preview).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  await expect(preview.locator("pre")).toHaveText(
    "first\nsecond\nthird\nfourth",
  );
  const back = page.locator(`[data-sheet="${eventId}"]`);
  await expect(back).toHaveAttribute("inert", "");
  await expect(back).toHaveCSS("transform", "matrix(0.96, 0, 0, 0.96, 0, 22)");
  await expect(back).toHaveCSS("opacity", "0.35");
  const firstPreviewId = await preview.getAttribute("data-sheet");
  await page.locator(".paste-chip").nth(1).click();
  await expect(preview.locator("pre")).toHaveText("another\npaste\nbody\nhere");
  await expect(page.locator(`[data-sheet="${firstPreviewId}"]`)).toHaveCSS(
    "transform",
    "matrix(0.96, 0, 0, 0.96, 0, 22)",
  );
  await expect(page.getByRole("dialog")).toHaveCount(1);
  await expect(input).toHaveValue(draft);
  await page.screenshot({
    path: "test-results/bottom-sheet-stack.png",
    fullPage: true,
  });

  await page.keyboard.press("Escape");
  await expect(preview.locator("pre")).toHaveText(
    "first\nsecond\nthird\nfourth",
  );
  await expect(preview).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  await page.keyboard.press("Escape");
  await expect(back).toHaveAttribute("data-active", "true");
  await expect(back).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  expect(await trigger.boundingBox()).toEqual(eventRow);
  await page
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(input).toHaveValue(draft);
});

test("mobile sheets stay below the title and stale previews cannot replace an edited draft", async ({
  page,
}) => {
  const input = await open(page);
  const body = "a\nb\nc\nd\n".repeat(100);
  await paste(page, body);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator(".paste-chip").click();
  const preview = page.getByRole("dialog", { name: "붙여넣기 원문" });
  await expect(preview).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  const bounds = (await preview.boundingBox())!;
  const header = (await page.locator(".conversation > header").boundingBox())!;
  expect(bounds.y).toBeGreaterThanOrEqual(header.y + header.height);
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeVisible();
  await input.fill("edited while the preview is open");
  await preview
    .getByRole("button", { name: "원문 펼치기", exact: true })
    .click();
  await expect(preview.getByRole("status")).toContainText(
    "입력 내용이 변경되었습니다",
  );
  await expect(input).toHaveValue("edited while the preview is open");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(preview).toHaveCSS("transition-duration", "0s");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(input).toBeFocused();
});
