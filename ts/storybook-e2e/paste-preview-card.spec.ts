import { expect, test } from "@playwright/test";

for (const [mode, body] of [
  ["text", "  first line\nsecond line with <tag> & emoji 😀  "],
  ["file", "  first line\nsecond line with <tag> & emoji 😀  \n".repeat(40)],
] as const) {
  test(`${mode} chip opens its complete source in a headerless Card with footer actions`, async ({
    page,
  }, testInfo) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(
      "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
    );
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await input.evaluate((el, text) => {
      el.focus();
      const data = new DataTransfer();
      data.setData("text/plain", text);
      el.dispatchEvent(
        new ClipboardEvent("paste", {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        }),
      );
    }, body);
    const chip = page.locator(".paste-chip");
    await expect(chip).toHaveAttribute("data-send-as", mode);
    if (mode === "file")
      await expect(chip).toHaveAttribute("data-upload-state", "ready");
    const draft = await input.inputValue();
    await chip
      .getByRole("button", { name: "View pasted text", exact: true })
      .click();
    const card = page.getByRole("dialog", {
      name: "Paste source",
      exact: true,
    });
    await expect(card).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
    await expect(card.locator(".card-heading")).toHaveCount(0);
    const source = card.locator(".card-body");
    expect(await source.textContent()).toBe(body);
    expect(await source.evaluate((el) => el.childElementCount)).toBe(0);
    await expect(source).toHaveCSS("white-space", "pre-wrap");
    expect(
      await source.evaluate((el) => getComputedStyle(el).fontFamily),
    ).toContain("monospace");
    await expect(card.locator(".attachment-info, pre, code")).toHaveCount(0);
    const footer = card.locator(".card-footer");
    await expect(footer.getByRole("button")).toHaveText(["Expand", "Close"]);
    const expand = footer.getByRole("button", { name: "Expand", exact: true });
    const close = footer.getByRole("button", {
      name: "Close paste preview",
      exact: true,
    });
    const placement = await footer.evaluate((el) => {
      const bounds = el.getBoundingClientRect();
      const css = getComputedStyle(el);
      return {
        left: bounds.left + parseFloat(css.paddingLeft),
        right: bounds.right - parseFloat(css.paddingRight),
      };
    });
    const left = (await expand.boundingBox())!;
    const right = (await close.boundingBox())!;
    expect(left.x).toBeCloseTo(placement.left, 1);
    expect(right.x + right.width).toBeCloseTo(placement.right, 1);
    const wrapper = (await page.locator(".composer-wrapper").boundingBox())!;
    const frame = (await card.boundingBox())!;
    expect(frame.y + frame.height).toBeLessThan(wrapper.y);
    if (mode === "file")
      expect(
        await card
          .locator(".card-body")
          .evaluate((el) => el.scrollHeight > el.clientHeight),
      ).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("paste-preview.png") });
    await close.click();
    await expect(card).toHaveCount(0);
    await expect(input).toHaveValue(draft);
    await expect(input).toBeFocused();
    await chip
      .getByRole("button", { name: "View pasted text", exact: true })
      .focus();
    await page.keyboard.press("Enter");
    await expect(card).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(card).toHaveCount(0);
    await input.press("Control+p");
    await expect(card).toBeVisible();
    await expand.click();
    await expect(card).toHaveCount(0);
    await expect(input).toHaveValue(body);
    await expect(chip).toHaveCount(0);
    expect(errors).toEqual([]);
  });
}
