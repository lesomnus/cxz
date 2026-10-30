import { expect, test } from "@playwright/test";

test("images show only progress then size with an image icon and unchanged label width", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--upload-transition&viewMode=story",
  );
  const image = page.locator('.paste-chip[data-image="true"]');
  await expect(image).toHaveAttribute("data-upload-state", "uploading");
  await expect(image).toHaveAccessibleName("Image attachment");
  await expect(image).not.toContainText("image.png");
  await expect(image.locator("[data-file-chip-label]")).toHaveText(/\d{3}%/);
  const width = await image
    .locator("[data-file-chip-label]")
    .evaluate((node) => node.getBoundingClientRect().width);
  const mask = await image.evaluate(
    (node) => getComputedStyle(node, "::before").maskImage,
  );
  expect(decodeURIComponent(mask)).toContain("<circle");
  await expect(image).toHaveAttribute("data-upload-state", "ready", {
    timeout: 8000,
  });
  await expect(image.locator("[data-file-chip-label]")).toHaveText("2.50M");
  await expect(image).not.toContainText("%");
  await expect(image).not.toContainText("image.png");
  expect(
    await image
      .locator("[data-file-chip-label]")
      .evaluate((node) => node.getBoundingClientRect().width),
  ).toBe(width);
  await image.click();
  const preview = page.getByRole("dialog", {
    name: "Image preview",
    exact: true,
  });
  await expect(preview.locator("img")).toBeVisible();
  await preview.getByRole("button", { name: "Close", exact: true }).click();
  await expect(preview).toHaveCount(0);
});

test("pasting an actual image uses the compact chip and leaves the caret after it", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.evaluate((node) => {
    node.focus();
    const data = new DataTransfer();
    const bytes = Uint8Array.from(
      atob(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l1sAAAAASUVORK5CYII=",
      ),
      (c) => c.charCodeAt(0),
    );
    data.items.add(new File([bytes], "image.png", { type: "image/png" }));
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData: data,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  const image = page.locator('.paste-chip[data-image="true"]');
  await expect(image).toBeVisible();
  await expect(image).not.toContainText("image.png");
  await expect(image).toHaveAttribute("data-upload-state", "ready");
  await expect(image.locator("[data-file-chip-label]")).toHaveText("0068B");
  const position = await image.evaluate((node) => {
    const native = node
      .closest("[data-chip-reservation]")!
      .querySelector("[data-native-chip-text]")!;
    const range = document.createRange();
    range.selectNodeContents(native);
    return {
      chip: node.getBoundingClientRect().right,
      caret: range.getBoundingClientRect().right,
    };
  });
  expect(position.caret).toBeGreaterThan(position.chip);
  expect(
    await input.evaluate(
      (node: HTMLTextAreaElement) => node.selectionStart === node.value.length,
    ),
  ).toBe(true);
});

test("upload chips shimmer and fade into outlined chips on completion", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--upload-transition&viewMode=story",
  );
  const chips = page.locator(".paste-chip");
  await expect(chips).toHaveCount(3);
  // Shared activity paints behind the label and never covers the caret.
  expect(
    await chips
      .first()
      .evaluate(
        (el) =>
          getComputedStyle(el.querySelector("[data-file-chip-activity]")!)
            .zIndex,
      ),
  ).toBe("auto");
  await expect(chips.first().locator("[data-file-chip-label]")).toHaveCSS(
    "z-index",
    "1",
  );
  await expect(chips.first()).toHaveAttribute("aria-busy", "true");
  const effect = () =>
    chips.first().evaluate((el) => {
      const style = getComputedStyle(
        el.querySelector("[data-file-chip-activity]")!,
      );
      return {
        opacity: style.opacity,
        playState: style.animationPlayState,
        duration: style.transitionDuration,
      };
    });
  await expect
    .poll(effect)
    .toEqual({ opacity: "1", playState: "running", duration: "0.45s" });
  await expect(chips.first()).toHaveAttribute("data-upload-state", "ready", {
    timeout: 8000,
  });
  await expect
    .poll(effect)
    .toEqual({ opacity: "0", playState: "paused", duration: "0.45s" });
  await expect(chips.first()).toHaveAttribute("aria-busy", "false");
  await page.getByRole("button", { name: "Replay upload" }).click();
  await expect(chips.first()).toHaveAttribute("data-upload-state", "uploading");
  await expect
    .poll(effect)
    .toEqual({ opacity: "1", playState: "running", duration: "0.45s" });
  await page.emulateMedia({ reducedMotion: "reduce" });
  expect(
    await chips
      .first()
      .evaluate(
        (el) =>
          getComputedStyle(el.querySelector("[data-file-chip-activity]")!)
            .animationName,
      ),
  ).toBe("none");
});
