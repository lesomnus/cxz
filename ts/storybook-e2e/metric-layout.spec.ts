import { expect, test } from "@playwright/test";

test("response metric values precede their icons and do not move them across value ranges", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-eventcards--metric-ranges&viewMode=story",
  );
  const metrics = page.locator(".response-metric");
  await expect(metrics).toHaveCount(8);
  const geometry = () =>
    metrics.evaluateAll((nodes) =>
      nodes.map((node) => {
        const value = node.querySelector("span")!;
        const icon = node.querySelector("svg")!;
        const box = node.getBoundingClientRect();
        const valueBox = value.getBoundingClientRect();
        const iconBox = icon.getBoundingClientRect();
        return {
          key: node.getAttribute("data-metric"),
          left: box.x,
          top: box.y,
          width: box.width,
          valueWidth: valueBox.width,
          iconLeft: iconBox.x,
          valueBeforeIcon: valueBox.right < iconBox.left,
          valueFits: value.scrollWidth <= value.clientWidth + 1,
        };
      }),
    );
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const small = await geometry();
    await page
      .getByRole("button", { name: "Use large values", exact: true })
      .click();
    await expect(page.locator('[data-metric="tool_calls"]')).toHaveText("0999");
    const large = await geometry();
    expect(large).toEqual(small);
    expect(
      large.every((metric) => metric.valueBeforeIcon && metric.valueFits),
    ).toBe(true);
    await page
      .getByRole("button", { name: "Use small values", exact: true })
      .click();
    await expect(page.locator('[data-metric="tool_calls"]')).toHaveText("0001");
  }
  await expect(page.locator('[data-metric="duration"]')).toHaveText("00:00:00");
  await expect(page.locator('[data-metric="api_duration_ms"]')).toHaveText(
    "00:00:00",
  );
  await expect(page.locator('[data-metric="cost_usd"]')).toHaveText("0100µ");
  const counter = page.locator('[data-metric="tool_calls"]');
  expect(
    await counter.evaluate((node) => {
      const number = node.querySelector('span[role="img"]')!;
      return (
        getComputedStyle(number.children[0]).color !==
        getComputedStyle(number.children[1]).color
      );
    }),
  ).toBe(true);
  const shades = await page
    .locator('[data-metric="input_tokens"]')
    .evaluate((node) => {
      const number = node.querySelector('span[role="img"]')!;
      const canvas = document.createElement("canvas");
      canvas.width = canvas.height = 1;
      const context = canvas.getContext("2d")!;
      return Array.from(number.children).map((part) => {
        context.fillStyle = getComputedStyle(part).color;
        context.fillRect(0, 0, 1, 1);
        return context.getImageData(0, 0, 1, 1).data[0];
      });
    });
  expect(shades[0]).toBeLessThan(shades[2]);
  expect(shades[2]).toBeLessThan(shades[1]);
  expect(
    (await counter.locator(":scope > span").boundingBox())!.width,
  ).toBeLessThan(
    (await page.locator('[data-metric="input_tokens"] > span').boundingBox())!
      .width,
  );
  await page
    .getByRole("button", { name: "Use large values", exact: true })
    .click();
  await expect(page.locator('[data-metric="duration"]')).toHaveText("01:39:59");
  await expect(page.locator('[data-metric="api_duration_ms"]')).toHaveText(
    "01:39:59",
  );
  await expect(page.locator('[data-metric="api_duration_ms"]')).toHaveAttribute(
    "title",
    /5,999,000 ms/,
  );
  await page.waitForTimeout(1100);
  await expect(page.locator('[data-metric="duration"]')).toHaveText("01:39:59");
  await page.goto(
    "/iframe.html?id=components-elapsedtime--running&viewMode=story",
  );
  const timer = page.getByRole("timer", {
    name: "Response elapsed time",
    exact: true,
  });
  await expect(timer).toBeVisible();
  const initial = await timer.textContent();
  await expect
    .poll(() => timer.textContent(), { timeout: 3000 })
    .not.toBe(initial);
  expect(errors).toEqual([]);
});
