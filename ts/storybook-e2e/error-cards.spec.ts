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
