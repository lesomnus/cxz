import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

test("visual scale changes preserve native chip reservations", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.evaluate((el) => {
    el.focus();
    const data = new DataTransfer();
    data.setData("text/plain", "A pasted message with enough text for a chip.");
    el.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData: data,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await expect(page.locator(".paste-chip")).toHaveAttribute(
    "data-send-as",
    "text",
  );
  const reservation = await input.inputValue();
  for (const scale of [0.97, 0.84, 1]) {
    await page.locator(".composer").evaluate((el, value) => {
      el.style.transform = `scale(${value})`;
      window.dispatchEvent(new Event("resize"));
    }, scale);
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    await expect(input).toHaveValue(reservation);
  }
});

for (const scenario of [
  { name: "system font", uniform: false, zoom: 1, longFile: false },
  { name: "uniform spacing font", uniform: true, zoom: 1, longFile: false },
  {
    name: "uniform spacing font at zoom",
    uniform: true,
    zoom: 1.5,
    longFile: false,
  },
  {
    name: "a long filename in a narrow zoomed viewport",
    uniform: true,
    zoom: 1.5,
    longFile: true,
  },
]) {
  test(`native caret stays outside paste chips with ${scenario.name}`, async ({
    page,
  }, testInfo) => {
    if (scenario.longFile)
      await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(
      "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
    );
    if (scenario.uniform) {
      const bytes = await readFile(
        new URL("../test-support/fixtures/uniform-chip.woff", import.meta.url),
      );
      await page.evaluate(async (data) => {
        const font = new FontFace(
          "ChipUniformFixture",
          `url(data:font/woff;base64,${data})`,
        );
        document.fonts.add(await font.load());
      }, bytes.toString("base64"));
    }
    await page.addStyleTag({
      content: `
        .composer-editor {
          ${scenario.uniform ? "font-family: ChipUniformFixture !important;" : ""}
          zoom: ${scenario.zoom};
        }
        .composer-editor textarea {
          caret-color: #ff0000 !important;
          caret-animation: manual;
        }
      `,
    });
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await input.evaluate((el, longFile) => {
      el.focus();
      const data = new DataTransfer();
      if (longFile) {
        data.items.add(new File(["hello"], `long-${"x".repeat(150)}.txt`));
        el.dispatchEvent(
          new DragEvent("drop", {
            dataTransfer: data,
            bubbles: true,
            cancelable: true,
          }),
        );
      } else {
        data.setData("text/plain", "first line abc\n".repeat(75));
        el.dispatchEvent(
          new ClipboardEvent("paste", {
            clipboardData: data,
            bubbles: true,
            cancelable: true,
          }),
        );
      }
    }, scenario.longFile);
    const chip = page.locator(".paste-chip");
    await expect(chip).toHaveAttribute("data-upload-state", "ready");
    // Do not move the selection to End: paste and upload acknowledgement must
    // leave the native caret in the correct place on their own.
    expect(
      await input.evaluate(
        (el: HTMLTextAreaElement) => el.selectionStart === el.value.length,
      ),
    ).toBe(true);
    const geometry = await chip.evaluate((el) => {
      const native = el
        .closest("[data-chip-reservation]")!
        .querySelector("[data-native-chip-text]")!;
      const range = document.createRange();
      range.selectNodeContents(native);
      return {
        frame: el.getBoundingClientRect().toJSON(),
        reserve: range.getBoundingClientRect().toJSON(),
        lines: range.getClientRects().length,
      };
    });
    expect(geometry.lines).toBe(1);
    expect(geometry.reserve.right).toBeGreaterThan(geometry.frame.right);

    // Check Chromium's painted caret, independently of the mirror/caretPosition
    // helper. Reset only blinking, preserving the paste's native selection.
    await input.evaluate((el) => {
      el.blur();
      el.focus();
    });
    await page.evaluate(
      () => new Promise((resolve) => requestAnimationFrame(resolve)),
    );
    const bounds = (await page.locator(".composer-editor").boundingBox())!;
    const clip = {
      x: Math.floor(bounds.x),
      y: Math.floor(bounds.y),
      width: Math.ceil(bounds.x + bounds.width) - Math.floor(bounds.x),
      height: Math.ceil(bounds.y + bounds.height) - Math.floor(bounds.y),
    };
    const screenshot = await page.screenshot({
      clip,
      caret: "initial",
      path: testInfo.outputPath("native-caret.png"),
    });
    await testInfo.attach("native-caret", {
      body: screenshot,
      contentType: "image/png",
    });
    const painted = await page.evaluate(async (base64) => {
      const image = new Image();
      image.src = `data:image/png;base64,${base64}`;
      await image.decode();
      const canvas = document.createElement("canvas");
      canvas.width = image.width;
      canvas.height = image.height;
      const context = canvas.getContext("2d")!;
      context.drawImage(image, 0, 0);
      const pixels = context.getImageData(
        0,
        0,
        canvas.width,
        canvas.height,
      ).data;
      const columns: number[] = [];
      for (let offset = 0; offset < pixels.length; offset += 4)
        if (
          pixels[offset] > 200 &&
          pixels[offset + 1] < 40 &&
          pixels[offset + 2] < 40
        )
          columns.push((offset / 4) % canvas.width);
      return { count: columns.length, left: Math.min(...columns) };
    }, screenshot.toString("base64"));
    expect(painted.count).toBeGreaterThan(3);
    expect(clip.x + painted.left).toBeGreaterThanOrEqual(
      Math.floor(geometry.frame.right),
    );
    await input.pressSequentially("x");
    expect(
      await input.evaluate((el: HTMLTextAreaElement) => el.value.endsWith("x")),
    ).toBe(true);
    await expect(page.locator(".editor-mirror")).toContainText("x");
  });
}
