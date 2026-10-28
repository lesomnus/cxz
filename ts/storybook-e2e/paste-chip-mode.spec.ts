import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { caretPosition } from "../src/features/session/composer/editor-caret";
import {
  fractionalChipBrowser,
  fractionalChipFont,
} from "../packages/nox/e2e/fractional-chip-font";

test.use(fractionalChipBrowser);

test("paste chips use icon modes and a ten-character flattened hover preview without opening a dialog", async ({
  page,
}, testInfo) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await fractionalChipFont(page);
  await page.addStyleTag({
    content:
      ".composer-editor { font-family: ChipFractionalFixture !important; }",
  });
  const fractional = await input.evaluate((el) => {
    const probe = document.createElement("span");
    Object.assign(probe.style, {
      position: "fixed",
      visibility: "hidden",
      fontFamily: "ChipFractionalFixture",
      fontSize: "14px",
    });
    probe.textContent = "\u2007".repeat(5);
    el.after(probe);
    const range = document.createRange();
    range.selectNodeContents(probe);
    const width = range.getBoundingClientRect().width;
    probe.remove();
    return width;
  });
  expect(Number.isInteger(fractional)).toBe(false);
  await input.evaluate((node) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData(
      "text/plain",
      "first\nline\r\nthird\nfourth\n".repeat(60),
    );
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  const chip = page.locator(".paste-chip");
  await expect(chip).toBeVisible();
  const layout = await chip.evaluate((el) =>
    [el, ...el.querySelectorAll("*")].map((node) => {
      const bounds = node.getBoundingClientRect();
      const style = getComputedStyle(node);
      return {
        tag: node.tagName,
        className: node.getAttribute("class"),
        width: style.width,
        height: style.height,
        renderedWidth: bounds.width,
        renderedHeight: bounds.height,
      };
    }),
  );
  await testInfo.attach("chip-layout", {
    body: JSON.stringify(layout, null, 2),
    contentType: "application/json",
  });
  await writeFile(
    testInfo.outputPath("chip-layout.json"),
    JSON.stringify(layout, null, 2),
  );
  for (const item of layout) {
    const width = parseFloat(item.width);
    if (Number.isFinite(width))
      expect(Number.isInteger(width), JSON.stringify(item)).toBe(true);
  }
  await expect(chip.locator("[data-file-chip-prefix]")).toHaveCSS(
    "width",
    "37px",
  );
  await expect(
    chip.getByRole("radio", { name: "File", exact: true }),
  ).toBeChecked();
  await expect(chip.getByText("Text", { exact: true })).toHaveCount(0);
  await expect(chip.getByText("File", { exact: true })).toHaveCount(0);
  const leading = await chip.evaluate((node) => {
    const reserved = node
      .querySelector("[data-file-chip-prefix]")!
      .getBoundingClientRect();
    const control = node
      .querySelector('[role="radiogroup"]')!
      .getBoundingClientRect();
    return {
      gap: reserved.width - control.width,
      inset: control.left - node.getBoundingClientRect().left,
      border: parseFloat(
        getComputedStyle(node).getPropertyValue("--ui-file-chip-border-width"),
      ),
      space: parseFloat(getComputedStyle(node).getPropertyValue("--ui-space")),
    };
  });
  expect(leading.gap).toBeGreaterThanOrEqual(leading.space * 0.5);
  expect(leading.gap).toBeLessThanOrEqual(leading.space * 2);
  expect(leading.inset).toBe(leading.border);
  const control = chip.getByRole("radiogroup");
  await expect(chip.locator("[data-file-chip-control]")).toHaveCSS(
    "width",
    "34px",
  );
  await expect(control).toHaveCSS("width", "34px");
  await expect(control).toHaveCSS("height", "18px");
  for (const label of await control.locator("label").all()) {
    await expect(label).toHaveCSS("width", "16px");
    await expect(label).toHaveCSS("height", "16px");
  }
  const chipBounds = (await chip.boundingBox())!;
  const controlBounds = (await control.boundingBox())!;
  expect(controlBounds.y).toBeGreaterThanOrEqual(chipBounds.y + leading.border);
  expect(controlBounds.y + controlBounds.height).toBeLessThanOrEqual(
    chipBounds.y + chipBounds.height - leading.border,
  );
  await input.press("Control+End");
  const selection = await input.evaluate(
    (el: HTMLTextAreaElement) => el.selectionStart,
  );
  const point = await input.evaluate(caretPosition, selection);
  const inputBounds = (await input.boundingBox())!;
  const gap = await chip.evaluate((el) =>
    parseFloat(
      getComputedStyle(el).getPropertyValue("--ui-file-chip-caret-gap"),
    ),
  );
  expect(
    Math.abs(
      inputBounds.x + point.left - chipBounds.x - chipBounds.width - gap,
    ),
  ).toBeLessThan(0.1);
  await chip.hover();
  await expect(page.getByRole("tooltip")).toHaveText("first line…");
  expect(
    await page.getByRole("tooltip").evaluate((el) => el.childElementCount),
  ).toBe(0);
  await page.screenshot({ path: testInfo.outputPath("paste-chip-hint.png") });
  await chip.locator(".paste-chip-label").click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const bounds = (await chip.boundingBox())!;
  await chip.getByRole("radio", { name: "Text", exact: true }).check();
  await expect(chip).toHaveAttribute("data-send-as", "text");
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeEnabled();
  await expect(chip.locator(".paste-chip-label")).not.toContainText("%");
  await chip.getByRole("radio", { name: "File", exact: true }).check();
  await expect(chip).toHaveAttribute("data-send-as", "file");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  const after = (await chip.boundingBox())!;
  expect(Math.abs(after.width - bounds.width)).toBeLessThan(0.125);
  await chip.getByRole("radio", { name: "File", exact: true }).focus();
  await page.keyboard.press("ArrowLeft");
  await expect(chip).toHaveAttribute("data-send-as", "text");
});

test("long file chips keep integer boxes and leave their native caret outside the frame", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  await fractionalChipFont(page);
  await page.addStyleTag({
    content:
      ".composer-editor { font-family: ChipFractionalFixture !important; }",
  });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.evaluate((el) => {
    el.focus();
    const data = new DataTransfer();
    data.items.add(
      new File(["hello"], `long-${"한글".repeat(12)}${"x".repeat(140)}.txt`),
    );
    el.dispatchEvent(
      new DragEvent("drop", {
        dataTransfer: data,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  const chip = page.locator(".paste-chip");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  const widths = await chip.evaluate((el) =>
    [el, ...el.querySelectorAll("*")].map((node) => ({
      tag: node.tagName,
      width: getComputedStyle(node).width,
    })),
  );
  for (const item of widths) {
    const width = parseFloat(item.width);
    if (Number.isFinite(width))
      expect(Number.isInteger(width), JSON.stringify(item)).toBe(true);
  }
  await expect
    .poll(() =>
      chip.evaluate(
        (el) =>
          el
            .closest("[data-chip-reservation]")!
            .querySelector("[data-native-chip-text]")!
            .getClientRects().length,
      ),
    )
    .toBe(1);
  await input.press("Control+End");
  const caret = await input.evaluate(
    (el: HTMLTextAreaElement) => el.selectionStart,
  );
  const point = await input.evaluate(caretPosition, caret);
  const field = (await input.boundingBox())!;
  const frame = (await chip.boundingBox())!;
  expect(field.x + point.left).toBeGreaterThan(frame.x + frame.width);
});
