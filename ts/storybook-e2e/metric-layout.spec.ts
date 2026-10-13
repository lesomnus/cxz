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
    await expect(page.locator('[data-metric="tool_calls"]')).toHaveText("999");
    const large = await geometry();
    expect(large).toEqual(small);
    expect(
      large.every((metric) => metric.valueBeforeIcon && metric.valueFits),
    ).toBe(true);
    await page
      .getByRole("button", { name: "Use small values", exact: true })
      .click();
    await expect(page.locator('[data-metric="tool_calls"]')).toHaveText("1");
  }
  expect(errors).toEqual([]);
});
