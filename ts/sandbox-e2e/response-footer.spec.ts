import { expect, test } from "@playwright/test";

test("final response metrics and top-right rectangular icon copy work for both providers", async ({
  page,
}) => {
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      value: {
        writeText: async (text: string) => {
          (window as any).copiedResponse = text;
        },
      },
    }),
  );
  await page.goto("/sandbox.html");
  const metrics = page.getByLabel("Response metrics", { exact: true });
  await expect(metrics).toBeVisible({ timeout: 45000 });
  await expect(metrics).toContainText("00:00:04");
  await expect(metrics.locator('[data-metric="input_tokens"]')).toHaveText(
    "1.20K",
  );
  await expect(metrics.locator('[data-metric="cost_usd"]')).toHaveText("12.3m");
  await expect(metrics.locator("time")).toHaveCount(0);
  const time = page.locator(".response-time time").first();
  await expect(time).not.toContainText(/\d{4}/);
  const recorded = await time.getAttribute("datetime");
  expect(Math.abs(Date.now() - Date.parse(recorded!))).toBeLessThan(60_000);
  for (const metric of await metrics.locator(".response-metric").all()) {
    await expect(metric.locator("svg")).toHaveCount(1);
    await expect(metric).toHaveAttribute("aria-label", /.+/);
  }
  const response = page.locator(".response").first();
  const copy = response
    .locator(".copy-control")
    .getByRole("button", { name: "Copy", exact: true });
  await expect(copy.locator("svg")).toHaveCount(1);
  await expect(copy).toHaveText("");
  const bounds = (await copy.boundingBox())!,
    box = (await response.boundingBox())!;
  expect(box.x + box.width - bounds.x - bounds.width).toBeCloseTo(12, 0);
  const heading = (await response.locator(".response-heading").boundingBox())!;
  expect(bounds.y + bounds.height / 2).toBeCloseTo(
    heading.y + heading.height / 2,
    0,
  );
  expect(bounds.width).toBeGreaterThan(bounds.height);
  await expect(response.locator(".copy-control")).toHaveCSS("opacity", "0");
  await response.hover();
  await expect(response.locator(".copy-control")).toHaveCSS("opacity", "1");
  await copy.hover();
  await expect(
    response.locator(".copy-control").getByRole("tooltip"),
  ).toBeVisible();
  await expect(
    response.locator(".copy-control").getByRole("tooltip"),
  ).toHaveText("Copy");
  await copy.click();
  await expect
    .poll(() => page.evaluate(() => (window as any).copiedResponse))
    .toContain("## Current status");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(metrics.locator('[data-metric="input_tokens"]')).toHaveText(
    "1.20K",
  );
  await expect(metrics.locator('[data-metric="input_tokens"]')).toHaveAttribute(
    "title",
    /Input — Last model call/,
  );
  await expect(metrics.locator('[data-metric="reasoning_tokens"]')).toHaveText(
    "0120·",
  );
  await expect(metrics.locator('[data-metric="cost_usd"]')).toHaveCount(0);
  await expect(
    metrics.locator('[data-metric="reasoning_tokens"]'),
  ).toHaveAttribute(
    "title",
    /Reasoning — Last model call.*Included in output tokens/,
  );
  await page.mouse.move(0, 0);
  await page.screenshot({
    path: "test-results/response-footer.png",
    fullPage: true,
  });
});
