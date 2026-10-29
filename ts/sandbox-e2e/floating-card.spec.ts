import { inspectPaste } from "../test-support/paste-chip";
import {
  readChipDisplay,
  expectChipDisplay,
} from "../test-support/chip-display";
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
  await expect(page.locator(".paste-chip").last()).toHaveAttribute(
    "data-upload-state",
    "ready",
  );
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
  await copy.getByRole("button", { name: "Copy", exact: true }).focus();
  await expect(copy).toHaveCSS("opacity", "1");
  await expect(copy.getByRole("tooltip")).toHaveText("Copy");
  expect(await response.boundingBox()).toEqual(before);
});

test("task details and replacing paste previews preserve the transcript and draft", async ({
  page,
}) => {
  const input = await open(page);
  await paste(page, "first\nsecond\nthird\nfourth".repeat(4));
  await input.press("End");
  await input.pressSequentially(" ");
  await paste(page, "another\npaste\nbody\nhere".repeat(4));
  const draft = await readChipDisplay(input);
  const trigger = page
    .locator("[data-work-summary]")
    .getByRole("button", { name: "View details", exact: true })
    .last();
  const geometry = () =>
    page
      .locator(".transcript")
      .evaluate((el) => ({ top: el.scrollTop, height: el.scrollHeight }));
  const before = await geometry();
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Tasks between responses" });
  const task = dialog.locator("button[data-seq]").first();
  await task.click();
  await expect(
    dialog
      .getByRole("region", { name: "Task details" })
      .locator(".monaco-editor"),
  ).toBeVisible();
  expect(await geometry()).toEqual(before);
  await task.click();
  await expect(
    dialog.getByRole("region", { name: "Task details" }),
  ).toHaveCount(0);
  await dialog.getByRole("button", { name: "Close task dialog" }).click();
  await inspectPaste(page, page.locator(".paste-chip").first());
  const preview = page.getByRole("dialog", { name: "Paste source" });
  await expect(preview).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  await expect(preview.locator(".card-body")).toHaveText(
    "first\nsecond\nthird\nfourth".repeat(4),
  );
  const firstPreviewId = await preview.getAttribute("data-card");
  await inspectPaste(page, page.locator(".paste-chip").nth(1));
  await expect(preview.locator(".card-body")).toHaveText(
    "another\npaste\nbody\nhere".repeat(4),
  );
  await expect(page.locator(`[data-card="${firstPreviewId}"]`)).toHaveCount(0);
  await expect(page.locator(".floating-card")).toHaveCount(1);
  await expectChipDisplay(input, draft);
  const blur = await preview.evaluate((el) =>
    getComputedStyle(el).getPropertyValue("--ui-dialog-blur").trim(),
  );
  await expect(preview).toHaveCSS("backdrop-filter", `blur(${blur})`);
  expect(await geometry()).toEqual(before);
  await page.screenshot({
    path: "test-results/floating-card.png",
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(input).toBeFocused();
  await expectChipDisplay(input, draft);
});

test.describe("rendered backdrop", () => {
  test.use({ deviceScaleFactor: 1 });
  test("card backdrop blurs rendered content rather than only tinting it", async ({
    page,
  }) => {
    await open(page);
    await paste(page, "blur\npreview\nbody\nhere".repeat(4));
    await inspectPaste(page, page.locator(".paste-chip"));
    const card = page.locator(".floating-card");
    await expect(card).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
    await expect(card).toHaveCSS("opacity", "1");
    const bounds = (await card.boundingBox())!;
    const footer = (await card.locator(".card-footer").boundingBox())!;
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
        async ({ base64, box, sampleY }) => {
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
            Math.floor(sampleY * scale),
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
        {
          base64: png.toString("base64"),
          box: bounds,
          sampleY: footer.y + footer.height / 2 - 2,
        },
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

test("mobile cards stay inside the conversation and stale previews cannot replace an edited draft", async ({
  page,
}) => {
  const input = await open(page);
  const body = "a\nb\nc\nd\n".repeat(100);
  await paste(page, body);
  await page.setViewportSize({ width: 390, height: 844 });
  await inspectPaste(page, page.locator(".paste-chip"));
  const preview = page.getByRole("dialog", { name: "Paste source" });
  await expect(preview).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  const bounds = (await preview.boundingBox())!;
  const area = (await page.locator(".transcript-area").boundingBox())!;
  expect(bounds.y).toBeGreaterThanOrEqual(area.y);
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
  await preview.getByRole("button", { name: "Expand", exact: true }).click();
  await expect(preview.getByRole("status")).toContainText(
    "The input has changed",
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
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  await paste(page, "first\nsecond\nthird\nfourth".repeat(4));
  const draft = await readChipDisplay(input);
  const preview = page.getByRole("dialog", { name: "Paste source" });
  await inspectPaste(page, page.locator(".paste-chip"));
  await expect(preview).toHaveCSS("opacity", "1");
  await preview.locator(".card-body").click();
  await expect(preview).toBeVisible();
  await input.click();
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
  await expectChipDisplay(input, draft);
  expect(await pane.evaluate((el) => el.scrollTop)).toBeCloseTo(scroll, 0);
  await inspectPaste(page, page.locator(".paste-chip"));
  await expect(preview).toHaveCSS("opacity", "1");
  await page.mouse.click(column.x + column.width + 20, area.y + 40);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator(".floating-card")).toHaveCount(0);
  await expectChipDisplay(input, draft);
});

test("pending questions stay dimmed behind previews and only lift for a taller card", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 620 });
  const input = await open(page);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  const question = page.locator(".approval");
  const layer = page.locator(".question-cards");
  await expect(
    question.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
  await expect(question).toHaveCSS("border-top-width", "1px");
  await expect(question.locator(".question-body")).toHaveCSS(
    "border-top-width",
    "0px",
  );
  await expect(question).toHaveCSS("backdrop-filter", "blur(24px)");
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

  await input.fill("");
  await paste(page, "preview source line\n".repeat(80));
  const details = page.locator(".composer .paste-chip");
  await inspectPaste(page, details);
  const preview = page.getByRole("dialog", { name: "Paste source" });
  await expect(preview).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
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
  await expect(input).toBeFocused();
  await expect(choice).not.toBeChecked();
  await expect(other).toHaveValue("Keep my answer\n");

  await input.fill("");
  await paste(
    page,
    "small preview body text without newlines, kept compact inside the card",
  );
  const draft = await readChipDisplay(input);
  await inspectPaste(page, page.locator(".paste-chip"));
  const short = page.getByRole("dialog", { name: "Paste source" });
  await expect(short).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  expect((await short.boundingBox())!.height).toBeLessThan(original.height);
  await expect(layer).toHaveAttribute("data-covered", "true");
  await expect(layer).toHaveJSProperty("inert", true);
  await expect(layer).toHaveCSS("transform", "matrix(0.97, 0, 0, 0.97, 0, 0)");
  await expect
    .poll(() =>
      question.evaluate((el) => getComputedStyle(el, "::after").opacity),
    )
    .toBe("1");
  const area = (await page.locator(".transcript-area").boundingBox())!;
  const column = (await page.locator(".transcript-content").boundingBox())!;
  await page.mouse.click(column.x - 20, area.y + 40);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(question).toHaveCount(1);
  await expect(layer).toHaveJSProperty("inert", false);
  await expect(layer).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  await expect(choice).not.toBeChecked();
  await expectChipDisplay(input, draft);
  expect(
    await page
      .locator(".transcript")
      .evaluate((el) => ({ top: el.scrollTop, height: el.scrollHeight })),
  ).toEqual(geometry);
  await question.getByRole("button", { name: "Submit" }).click();
  await expect(question).toHaveCount(0);
  await expect(
    page.getByText("Your selection was recorded for this preview.", {
      exact: false,
    }),
  ).toBeVisible();
});

test.describe("touch questions", () => {
  test.use({ hasTouch: true });
  test("question and covering details stay bounded on mobile and dismiss independently", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 620 });
    await open(page);
    await page
      .getByLabel("Scenario", { exact: true })
      .selectOption("session-4");
    const question = page.locator(".approval");
    await expect(
      question.getByText("Which environment?", { exact: true }),
    ).toBeVisible();
    await paste(page, "preview source line\n".repeat(80));
    await inspectPaste(page, page.locator(".composer .paste-chip"));
    const layer = page.locator(".question-cards");
    const preview = page.getByRole("dialog", { name: "Paste source" });
    await expect(preview).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
    await expect(layer).toHaveAttribute("data-covered", "true");
    await expect
      .poll(async () =>
        Math.round(
          (await preview.boundingBox())!.y - (await question.boundingBox())!.y,
        ),
      )
      .toBe(28);
    await page.setViewportSize({ width: 390, height: 560 });
    await expect
      .poll(async () =>
        Math.round(
          (await preview.boundingBox())!.y - (await question.boundingBox())!.y,
        ),
      )
      .toBe(28);
    expect(await page.locator("body").evaluate((el) => el.scrollWidth)).toBe(
      390,
    );
    const area = (await page.locator(".transcript-area").boundingBox())!;
    const bounds = (await question.boundingBox())!;
    expect(bounds.y).toBeGreaterThanOrEqual(area.y + 7);
    expect(bounds.x).toBeGreaterThanOrEqual(0);
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
    await expect(
      page.getByRole("button", { name: "Send", exact: true }),
    ).toBeVisible();
    await preview
      .getByRole("button", { name: "Close paste preview", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(question).toHaveCount(1);
    await page.emulateMedia({ reducedMotion: "reduce" });
    await expect(layer).toHaveCSS("transition-duration", "0s");
    await question.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(question).toHaveCount(1);
    await question
      .getByRole("button", { name: "Confirm cancel", exact: true })
      .tap();
    await expect(question).toHaveCount(0);
  });
});
