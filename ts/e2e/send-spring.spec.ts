import { test, expect, devices } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});
test("pending input pulls down without fading and a failed send springs back with its draft intact", async ({
  page,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.goto("/sessions/session");
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/cxz.SessionService/Send", async (route) => {
    await held;
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ code: "internal", message: "Test send failure" }),
    });
  });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Keep my failed draft");
  const source = page.locator(".composer-input");
  const bounds = await source.boundingBox();
  await page.getByRole("button", { name: "Send", exact: true }).click();
  const mirror = page.locator(".composer-send-ghost .editor-mirror");
  await expect
    .poll(() =>
      mirror.evaluate(
        (el) => new DOMMatrix(getComputedStyle(el).transform).m42,
      ),
    )
    .toBeGreaterThan(4);
  await expect(mirror).toHaveCSS("opacity", "1");
  expect(await source.boundingBox()).toEqual(bounds);
  release();
  await expect(page.getByRole("alert")).toContainText("Test send failure");
  await expect(page.locator(".composer-send-ghost")).toHaveCount(0);
  await expect(input).toHaveValue("Keep my failed draft");
  await expect(input).toBeEditable();
  await expect(source).not.toHaveAttribute("data-send-exiting", "true");
});
