import { expect, test } from "@playwright/test";

test("provider errors show their message in a red face, including the hover portal", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-eventcards--model-at-capacity&viewMode=story",
  );
  const card = page.locator(".event-detail");
  await expect(card).toContainText(
    "Selected model is at capacity. Please try a different model.",
  );
  await expect(card).toContainText("serverOverloaded · No automatic retry");
  const face = card.locator(".activity-card-face");
  const background = await face.evaluate(
    (node) => getComputedStyle(node).backgroundColor,
  );
  // Resolve color-mix through canvas to compare channels across Chromium versions.
  const channels = await face.evaluate((node) => {
    const canvas = document.createElement("canvas");
    const ctx = canvas.getContext("2d")!;
    ctx.fillStyle = getComputedStyle(node).backgroundColor;
    ctx.fillRect(0, 0, 1, 1);
    return [...ctx.getImageData(0, 0, 1, 1).data];
  });
  expect(channels[0]).toBeGreaterThan(channels[1]);
  expect(channels[0]).toBeGreaterThan(channels[2]);
  expect(channels[3]).toBe(255);
  await card.hover();
  const preview = page.locator(".activity-card-preview .activity-card-face");
  await expect(preview).toBeVisible();
  await expect(preview).toHaveCSS("background-color", background);
  await expect(preview).toContainText("Selected model is at capacity.");
  expect(errors).toEqual([]);
});

test("failed Bash stays neutral before and during hover", async ({ page }) => {
  await page.goto(
    "/iframe.html?id=conversation-eventcards--failed-tool&viewMode=story",
  );
  const card = page.locator(".event-detail[data-state='failed']");
  await expect(card).toContainText("Bash");
  await expect(card.getByRole("img", { name: "Failed" })).toHaveText("×");
  const face = card.locator(".activity-card-face");
  const background = await face.evaluate(
    (node) => getComputedStyle(node).backgroundColor,
  );
  const channels = await face.evaluate((node) => {
    const ctx = document.createElement("canvas").getContext("2d")!;
    ctx.fillStyle = getComputedStyle(node).backgroundColor;
    ctx.fillRect(0, 0, 1, 1);
    return [...ctx.getImageData(0, 0, 1, 1).data];
  });
  expect(channels[0]).toBe(channels[1]);
  expect(channels[1]).toBe(channels[2]);
  await card.hover();
  await expect(
    page.locator(".activity-card-preview .activity-card-face"),
  ).toHaveCSS("background-color", background);
});

for (const id of ["earlier-error", "legacy-error-history"]) {
  test(`${id}: error stays upright with the full native message`, async ({
    page,
  }) => {
    await page.goto(
      `/iframe.html?id=conversation-eventcards--${id}&viewMode=story`,
    );
    const card = page.locator(".event-detail[data-seq='1']");
    await expect(card).toContainText(
      "Selected model is at capacity. Please try a different model.",
    );
    const row = page.locator("[data-row='1']");
    await expect(row).toHaveAttribute("data-activity", "false");
    const face = card.locator(".activity-card-face");
    await expect(face).toHaveCSS("transform", "none");
    expect(
      await row.evaluate((node) => node.getBoundingClientRect().height),
    ).toBeGreaterThan(40);
    await card.hover();
    const preview = page.locator(".activity-card-preview");
    await expect(preview).toHaveAttribute("data-unfolded", "true");
    await expect(preview).toContainText(
      "serverOverloaded · No automatic retry",
    );
    const upright = (
      await preview
        .locator(".activity-card-face")
        .evaluate((node) => [
          ...new DOMMatrix(getComputedStyle(node).transform).toFloat64Array(),
        ])
    ).map((number) => Math.round(number * 1000) / 1000);
    expect(upright).toEqual([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]);
    await card.dblclick();
    await expect(
      page.getByRole("tab", { name: "Result", exact: true }),
    ).toBeVisible();
  });
}
