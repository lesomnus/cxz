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

test("floating event and paste cards replace one another without moving the transcript", async ({
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
  const event = page.locator('.floating-card[data-active="true"]');
  await expect(event).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  const eventId = await event.getAttribute("data-card");
  const wrapper = (await page.locator(".composer-wrapper").boundingBox())!;
  const card = (await event.boundingBox())!;
  expect(wrapper.y - card.y - card.height).toBeCloseTo(12, 0);
  await expect(event).toHaveCSS("border-top-width", "0px");
  await expect(event).toHaveCSS("border-radius", "12px");
  await expect(event).toHaveCSS("backdrop-filter", "blur(48px)");
  await expect(event.locator(".card-body")).toHaveCSS("padding-left", "4px");
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
  await expect(page.locator(`[data-card="${eventId}"]`)).toHaveCount(0);
  const firstPreviewId = await preview.getAttribute("data-card");
  await page.locator(".paste-chip").nth(1).click();
  await expect(preview.locator("pre")).toHaveText("another\npaste\nbody\nhere");
  await expect(page.locator(`[data-card="${firstPreviewId}"]`)).toHaveCount(0);
  await expect(page.locator(".floating-card")).toHaveCount(1);
  await expect(input).toHaveValue(draft);
  await expect(preview).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  await page.screenshot({
    path: "test-results/floating-card.png",
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator(".floating-card")).toHaveCount(0);
  await expect(input).toBeFocused();
  expect(await trigger.boundingBox()).toEqual(eventRow);
  await trigger.click();
  await page
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(input).toHaveValue(draft);
});

test("mobile cards stay below the title and stale previews cannot replace an edited draft", async ({
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

test("only the transcript's empty side margins dismiss the floating card", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 720 });
  const input = await open(page);
  await paste(page, "first\nsecond\nthird\nfourth");
  const draft = await input.inputValue();
  const preview = page.getByRole("dialog", { name: "붙여넣기 원문" });
  await page.locator(".paste-chip").click();
  await expect(preview).toHaveCSS("opacity", "1");
  await preview.locator("pre").click();
  await expect(preview).toBeVisible();
  await page.getByRole("heading", { name: "Current status" }).click();
  await expect(preview).toBeVisible();
  // The scrollbar sits in a side margin but remains an interactive control.
  const thumb = page.getByRole("scrollbar", { name: "Conversation scroll" });
  await expect(thumb).toBeVisible();
  await thumb.hover();
  await page.mouse.down();
  await page.mouse.up();
  await expect(preview).toBeVisible();
  const area = (await page.locator(".transcript-area").boundingBox())!;
  const column = (await page.locator(".transcript-content").boundingBox())!;
  const pane = page.locator(".transcript");
  const scroll = await pane.evaluate((el) => el.scrollTop);
  await page.mouse.click(column.x - 20, area.y + 40);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(input).toHaveValue(draft);
  expect(await pane.evaluate((el) => el.scrollTop)).toBeCloseTo(scroll, 0);
  await page.locator(".paste-chip").click();
  await expect(preview).toHaveCSS("opacity", "1");
  await page.mouse.click(column.x + column.width + 20, area.y + 40);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator(".floating-card")).toHaveCount(0);
  await expect(input).toHaveValue(draft);
});
