import { expect, test } from "@playwright/test";

test("final response metrics and right-aligned icon copy work for both providers", async ({
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
  await expect(metrics).toContainText("4.2초");
  await expect(metrics).toContainText("입력 1.2K");
  await expect(metrics).toContainText("비용 $0.0123");
  await expect(metrics.locator("time")).toHaveAttribute("datetime", /2023-/);
  const footer = page.locator(".response-footer").first();
  const copy = footer.getByRole("button", { name: "Copy", exact: true });
  await expect(copy.locator("svg")).toHaveCount(1);
  await expect(copy).toHaveText("");
  const bounds = (await copy.boundingBox())!,
    box = (await footer.boundingBox())!;
  expect(box.x + box.width - bounds.x - bounds.width).toBeCloseTo(12, 0);
  await copy.hover();
  await expect(footer.getByRole("tooltip")).toBeVisible();
  await expect(footer.getByRole("tooltip")).toHaveText("copy");
  await copy.click();
  await expect
    .poll(() => page.evaluate(() => (window as any).copiedResponse))
    .toContain("## Current status");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(metrics).toContainText("호출 입력 1.2K");
  await expect(metrics).toContainText("호출 추론 120");
  await expect(metrics).not.toContainText("비용");
  await expect(
    metrics.locator("span").filter({ hasText: "호출 추론" }),
  ).toHaveAttribute("title", /출력 토큰에 포함/);
  await page.mouse.move(0, 0);
  await page.screenshot({
    path: "test-results/response-footer.png",
    fullPage: true,
  });
});
