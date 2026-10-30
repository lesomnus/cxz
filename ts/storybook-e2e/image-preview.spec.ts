import { expect, test, type Page } from "@playwright/test";

async function watchImageUrls(page: Page) {
  await page.addInitScript(() => {
    const images = { created: [] as string[], revoked: [] as string[] };
    (window as typeof window & { imageUrls: typeof images }).imageUrls = images;
    const create = URL.createObjectURL.bind(URL);
    const revoke = URL.revokeObjectURL.bind(URL);
    URL.createObjectURL = (blob) => {
      const url = create(blob);
      if (blob instanceof Blob && blob.type.startsWith("image/"))
        images.created.push(url);
      return url;
    };
    URL.revokeObjectURL = (url) => {
      if (images.created.includes(url)) images.revoked.push(url);
      revoke(url);
    };
  });
}

for (const story of ["ready-image", "restored-image"]) {
  test(`${story} fits transparency on a checkerboard, zooms, pans within bounds and cleans up`, async ({
    page,
  }, testInfo) => {
    await watchImageUrls(page);
    if (story === "restored-image")
      await page.setViewportSize({ width: 390, height: 640 });
    await page.goto(
      `/iframe.html?id=composer-upload-chips--${story}&viewMode=story${story === "restored-image" ? "&globals=theme:light" : ""}`,
    );
    const chip = page.getByRole("button", {
      name: "Image attachment",
      exact: true,
    });
    await chip.click();
    const dialog = page.getByRole("dialog", {
      name: "Image preview",
      exact: true,
    });
    const viewport = dialog.locator("[data-image-viewport]");
    const image = dialog.locator("img");
    await expect(image).toBeVisible();
    expect(
      await dialog.evaluate((node) => !!node.closest(".floating-card-host")),
    ).toBe(true);
    await expect(page.locator("dialog[open]")).toHaveCount(0);
    await expect(dialog).toHaveAttribute("data-entered", "true");
    await dialog.evaluate(async (node) => {
      await Promise.all(
        node
          .getAnimations({ subtree: true })
          .filter((a) => a.effect?.getTiming().iterations !== Infinity)
          .map((a) => a.finished.catch(() => {})),
      );
    });
    const box = (await viewport.boundingBox())!;
    const frame = (await dialog.boundingBox())!;
    const composer = (await page.locator(".composer-wrapper").boundingBox())!;
    expect(frame.y + frame.height).toBeLessThan(composer.y);
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await input.focus();
    await expect(input).toBeFocused();
    await expect(dialog).toBeVisible();
    const initial = (await image.boundingBox())!;
    expect(initial.x).toBeGreaterThanOrEqual(box.x - 1);
    expect(initial.y).toBeGreaterThanOrEqual(box.y - 1);
    expect(initial.x + initial.width).toBeLessThanOrEqual(
      box.x + box.width + 1,
    );
    expect(initial.y + initial.height).toBeLessThanOrEqual(
      box.y + box.height + 1,
    );
    expect(
      Math.min(
        Math.abs(initial.width - box.width),
        Math.abs(initial.height - box.height),
      ),
    ).toBeLessThan(1);
    await expect(viewport).toHaveCSS("padding", "0px");
    expect(
      await viewport.evaluate((node) => getComputedStyle(node).backgroundImage),
    ).toContain("conic-gradient");
    if (story === "ready-image")
      await dialog.screenshot({
        path: testInfo.outputPath("checkerboard.png"),
      });

    const slider = dialog.getByRole("slider", { name: "Image zoom" });
    const nativeSize = await image.evaluate((node: HTMLImageElement) => ({
      width: node.naturalWidth,
      height: node.naturalHeight,
    }));
    const fittedZoom = Number(await slider.inputValue());
    expect(fittedZoom).toBeLessThanOrEqual(100);
    expect(
      Math.abs(initial.width - (nativeSize.width * fittedZoom) / 100),
    ).toBeLessThan(1);
    const gaps = await dialog.evaluate((node) => {
      const outer = node.getBoundingClientRect();
      const inner = node.querySelector(".card-body")!.getBoundingClientRect();
      return { top: inner.top - outer.top, left: inner.left - outer.left };
    });
    expect(gaps.top).toBeGreaterThan(0);
    expect(gaps.top).toBeCloseTo(gaps.left, 4);
    await slider.evaluate((node: HTMLInputElement) => {
      Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value",
      )!.set!.call(node, "100");
      node.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await expect(slider).toHaveValue("100");
    await expect
      .poll(async () => (await image.boundingBox())!.width)
      .toBeCloseTo(nativeSize.width, 1);
    expect((await image.boundingBox())!.height).toBeCloseTo(
      nativeSize.height,
      1,
    );
    await slider.focus();
    await slider.press("End");
    await expect(slider).toHaveValue("400");
    await expect(viewport).toHaveAttribute("data-can-pan", "true");
    const enlarged = (await image.boundingBox())!;
    expect(Math.abs(enlarged.width / nativeSize.width - 4)).toBeLessThan(0.01);
    const x = box.x + box.width / 2,
      y = box.y + box.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await expect(viewport).toHaveCSS("cursor", "grabbing");
    await page.mouse.move(x + 80, y + 35, { steps: 6 });
    await page.mouse.up();
    await expect(viewport).toHaveCSS("cursor", "grab");
    const moved = (await image.boundingBox())!;
    expect(Math.abs(moved.x - enlarged.x - 80)).toBeLessThan(1);
    expect(Math.abs(moved.y - enlarged.y - 35)).toBeLessThan(1);

    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.mouse.move(10000, 10000, { steps: 4 });
    await page.mouse.up();
    const edge = (await image.boundingBox())!;
    expect(edge.x).toBeLessThanOrEqual(box.x + 1);
    expect(edge.y).toBeLessThanOrEqual(box.y + 1);
    expect(edge.x + edge.width).toBeGreaterThanOrEqual(box.x + box.width - 1);
    expect(edge.y + edge.height).toBeGreaterThanOrEqual(box.y + box.height - 1);

    await slider.focus();
    await slider.press("Home");
    await expect(slider).toHaveValue("1");
    await expect(viewport).not.toHaveAttribute("data-can-pan");
    const small = (await image.boundingBox())!;
    expect(Math.abs(small.x + small.width / 2 - x)).toBeLessThan(1);
    expect(Math.abs(small.y + small.height / 2 - y)).toBeLessThan(1);

    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(input).toBeFocused();
    await expect
      .poll(async () =>
        page.evaluate(() => {
          const urls = (
            window as typeof window & {
              imageUrls: { created: string[]; revoked: string[] };
            }
          ).imageUrls;
          return urls.revoked.length;
        }),
      )
      .toBe(1);
    const urls = await page.evaluate(
      () =>
        (
          window as typeof window & {
            imageUrls: { created: string[]; revoked: string[] };
          }
        ).imageUrls,
    );
    expect(urls.created).toHaveLength(1);
    expect(urls.revoked).toEqual(urls.created);
  });
}

test("pasted PNG opens immediately before upload finishes and reopens with zoom reset", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.evaluate(async (node) => {
    const canvas = document.createElement("canvas");
    canvas.width = 800;
    canvas.height = 600;
    canvas.getContext("2d")!.fillRect(200, 150, 400, 300);
    const blob = await new Promise<Blob>((resolve) =>
      canvas.toBlob((blob) => resolve(blob!), "image/png"),
    );
    const data = new DataTransfer();
    data.items.add(new File([blob], "image.png", { type: "image/png" }));
    node.focus();
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData: data,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  const chip = page.locator('.paste-chip[data-image="true"]');
  await expect(chip).toHaveAttribute("data-upload-state", "uploading");
  await chip.click();
  const dialog = page.getByRole("dialog", {
    name: "Image preview",
    exact: true,
  });
  const image = dialog.locator("img");
  await expect(image).toBeVisible();
  expect(
    await image.evaluate((node: HTMLImageElement) => [
      node.naturalWidth,
      node.naturalHeight,
    ]),
  ).toEqual([800, 600]);
  await expect(chip).toHaveAttribute("data-upload-state", "uploading");
  const slider = dialog.getByRole("slider", { name: "Image zoom" });
  const fittedZoom = await slider.inputValue();
  await slider.press("End");
  await expect(slider).toHaveValue("400");
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(input).toBeFocused();
  await chip.click();
  await expect(image).toBeVisible();
  await expect(slider).toHaveValue(fittedZoom);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("small images open at their original size without automatic enlargement", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .evaluate(async (node) => {
      const canvas = document.createElement("canvas");
      canvas.width = 16;
      canvas.height = 12;
      const blob = await new Promise<Blob>((resolve) =>
        canvas.toBlob((blob) => resolve(blob!), "image/png"),
      );
      const data = new DataTransfer();
      data.items.add(new File([blob], "small.png", { type: "image/png" }));
      node.focus();
      node.dispatchEvent(
        new ClipboardEvent("paste", {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        }),
      );
    });
  await page.locator('.paste-chip[data-image="true"]').click();
  const card = page.getByRole("dialog", { name: "Image preview", exact: true });
  const image = card.locator("img");
  await expect(image).toBeVisible();
  await expect(card).toHaveAttribute("data-entered", "true");
  await card.evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .filter(
          (animation) => animation.effect?.getTiming().iterations !== Infinity,
        )
        .map((animation) => animation.finished.catch(() => {})),
    );
  });
  await expect(card.getByRole("slider", { name: "Image zoom" })).toHaveValue(
    "100",
  );
  const box = (await image.boundingBox())!;
  expect(box.width).toBeCloseTo(16, 1);
  expect(box.height).toBeCloseTo(12, 1);
});

test("invalid image bytes show a recoverable error and keep zoom disabled", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .evaluate((node) => {
      const data = new DataTransfer();
      data.items.add(new File(["invalid"], "image.png", { type: "image/png" }));
      node.focus();
      node.dispatchEvent(
        new ClipboardEvent("paste", {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        }),
      );
    });
  await page.locator('.paste-chip[data-image="true"]').click();
  const dialog = page.getByRole("dialog", {
    name: "Image preview",
    exact: true,
  });
  await expect(dialog.getByRole("alert")).toHaveText(
    "The image could not be loaded.",
  );
  await expect(
    dialog.getByRole("slider", { name: "Image zoom" }),
  ).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "Retry", exact: true }),
  ).toBeVisible();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});
