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
  await expect(event).toHaveCSS("border-top-width", "1px");
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

test.describe("rendered backdrop", () => {
  test.use({ deviceScaleFactor: 1 });
  test("card backdrop blurs rendered content rather than only tinting it", async ({
    page,
  }) => {
    await open(page);
    await page.locator(".transcript-row > .event-detail").last().click();
    const card = page.locator(".floating-card");
    await expect(card).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
    await expect(card).toHaveCSS("opacity", "1");
    const bounds = (await card.boundingBox())!;
    // A real background in the conversation must be sampled across the composer
    // and host ancestors. A computed blur value alone misses backdrop isolation.
    await page.evaluate((box) => {
      const backing = document.createElement("div");
      backing.style.cssText = `position:fixed;left:${box.x}px;top:${box.y}px;width:${box.width}px;height:${box.height}px;pointer-events:none;background:repeating-linear-gradient(90deg,#000 0 32px,#fff 32px 64px)`;
      document.querySelector(".transcript-area")!.append(backing);
    }, bounds);
    const edgeContrast = async () => {
      // Capture the viewport before sampling: a cropped capture can exclude
      // the pixels needed by the filter, particularly at higher pixel density.
      const png = await page.screenshot();
      return page.evaluate(
        async ({ base64, box }) => {
          const image = new Image();
          image.src = `data:image/png;base64,${base64}`;
          await image.decode();
          const canvas = document.createElement("canvas");
          canvas.width = image.width;
          canvas.height = image.height;
          const context = canvas.getContext("2d")!;
          context.drawImage(image, 0, 0);
          const scale = image.width / window.innerWidth;
          const width = Math.floor(300 * scale);
          const height = Math.floor(4 * scale);
          const pixels = context.getImageData(
            Math.floor((box.x + 100) * scale),
            Math.floor((box.y + 30) * scale),
            width,
            height,
          ).data;
          let greatestStep = 0;
          for (let y = 0; y < height; y++) {
            for (let x = 1; x < width; x++) {
              const index = (y * width + x) * 4;
              greatestStep = Math.max(
                greatestStep,
                Math.abs(pixels[index] - pixels[index - 4]),
              );
            }
          }
          return greatestStep;
        },
        { base64: png.toString("base64"), box: bounds },
      );
    };
    expect(await edgeContrast()).toBeLessThan(5);
    await card.evaluate((el) => {
      el.style.backdropFilter = "none";
      el.style.setProperty("-webkit-backdrop-filter", "none");
    });
    expect(await edgeContrast()).toBeGreaterThan(50);
  });
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
  expect(bounds.height).toBeLessThanOrEqual(360);
  expect(
    await preview
      .locator(".card-body")
      .evaluate((el) => el.scrollHeight > el.clientHeight),
  ).toBe(true);
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

test("pending questions survive previews and only recede behind a taller card", async ({
  page,
}) => {
  const input = await open(page);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  const question = page.locator(".approval");
  const layer = page.locator(".question-cards");
  await expect(
    question.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
  await expect(question).toHaveCSS("border-top-width", "1px");
  await expect(question).toHaveCSS("border-radius", "12px");
  await expect(question).toHaveCSS("backdrop-filter", "blur(48px)");
  await expect(question.locator(".card-close")).toHaveCount(0);
  const choice = question.getByRole("radio", { name: /Development/ });
  await choice.check();
  const other = question.getByRole("textbox", { name: /Other answer/ });
  await other.fill("Keep my answer");
  await input.fill("Unsent message");
  await other.press("Enter");
  await expect(input).toHaveValue("Unsent message");
  await page.keyboard.press("Escape");
  await expect(question).toHaveCount(1);
  const original = (await question.boundingBox())!;
  const geometry = await page
    .locator(".transcript")
    .evaluate((el) => ({ top: el.scrollTop, height: el.scrollHeight }));

  const details = question.getByRole("button", {
    name: "Request details",
    exact: true,
  });
  await details.click();
  const preview = page.getByRole("dialog", { name: "Request details" });
  await expect(layer).toHaveAttribute("data-covered", "true");
  await expect(layer).toHaveJSProperty("inert", true);
  await expect
    .poll(async () => {
      const front = (await preview.boundingBox())!;
      const back = (await question.boundingBox())!;
      return Math.round(front.y - back.y);
    })
    .toBe(28);
  expect((await preview.boundingBox())!.height).toBeGreaterThanOrEqual(
    original.height,
  );
  await expect
    .poll(() =>
      question.evaluate((el) => getComputedStyle(el, "::after").opacity),
    )
    .toBe("1");
  expect((await question.boundingBox())!.width).toBeCloseTo(
    original.width * 0.97,
    0,
  );
  await page.screenshot({
    path: "test-results/question-behind-details.png",
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(layer).toHaveJSProperty("inert", false);
  await expect(details).toBeFocused();
  await expect(choice).toBeChecked();
  await expect(other).toHaveValue("Keep my answer");

  await input.fill("");
  await paste(page, "small\npreview\nbody\nhere");
  const draft = await input.inputValue();
  await page.locator(".paste-chip").click();
  const short = page.getByRole("dialog", { name: "붙여넣기 원문" });
  await expect(short).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  expect((await short.boundingBox())!.height).toBeLessThan(original.height);
  await expect(layer).toHaveAttribute("data-covered", "false");
  await expect(layer).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  const area = (await page.locator(".transcript-area").boundingBox())!;
  const column = (await page.locator(".transcript-content").boundingBox())!;
  await page.mouse.click(column.x - 20, area.y + 40);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(question).toHaveCount(1);
  await expect(choice).toBeChecked();
  await expect(input).toHaveValue(draft);
  expect(
    await page
      .locator(".transcript")
      .evaluate((el) => ({ top: el.scrollTop, height: el.scrollHeight })),
  ).toEqual(geometry);
  await question.getByRole("button", { name: "Submit answers" }).click();
  await expect(question).toHaveCount(0);
  await expect(
    page.getByText("Your selection was recorded for this preview.", {
      exact: false,
    }),
  ).toBeVisible();
});

test("question and covering details stay bounded on mobile and dismiss independently", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 720 });
  await open(page);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  const question = page.locator(".approval");
  await expect(
    question.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
  await question
    .getByRole("button", { name: "Request details", exact: true })
    .click();
  const layer = page.locator(".question-cards");
  const preview = page.getByRole("dialog", { name: "Request details" });
  await expect(layer).toHaveAttribute("data-covered", "true");
  await expect
    .poll(async () =>
      Math.round(
        (await preview.boundingBox())!.y - (await question.boundingBox())!.y,
      ),
    )
    .toBe(28);
  await page.setViewportSize({ width: 390, height: 620 });
  await expect
    .poll(async () =>
      Math.round(
        (await preview.boundingBox())!.y - (await question.boundingBox())!.y,
      ),
    )
    .toBe(28);
  expect(await page.locator("body").evaluate((el) => el.scrollWidth)).toBe(390);
  const header = (await page.locator(".conversation > header").boundingBox())!;
  const bounds = (await question.boundingBox())!;
  expect(bounds.y).toBeGreaterThanOrEqual(header.y + header.height + 7);
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeVisible();
  await preview
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(question).toHaveCount(1);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(layer).toHaveCSS("transition-duration", "0s");
  await question.getByRole("button", { name: "Deny", exact: true }).click();
  await expect(question).toHaveCount(0);
});
