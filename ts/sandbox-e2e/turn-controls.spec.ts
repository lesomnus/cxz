import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test.beforeEach(async ({ page }) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  await expect(
    page.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
});

test("a single click expires; a second click interrupts without stopping the session", async ({
  page,
}) => {
  const stop = page.getByRole("button", { name: "Stop response", exact: true });
  const timer = page.getByRole("timer", {
    name: "Response elapsed time",
    exact: true,
  });
  await expect(stop).toBeEnabled();
  await expect(timer).toHaveText(/\d{2}:\d{2}:\d{2}/);
  const colors = await timer.evaluate((el) =>
    [...el.children].map((part) => getComputedStyle(part).color),
  );
  expect(colors[0]).not.toBe(colors[1]);
  const started = await timer.innerText();
  await page.screenshot({ path: "test-results/turn-controls-desktop.png" });
  await stop.click();
  await expect(stop).toHaveAttribute("data-armed", "true");
  await expect(stop).toHaveAttribute("data-armed", "false", { timeout: 5000 });
  await expect.poll(() => timer.innerText()).not.toBe(started);
  await stop.click();
  await expect(stop).toBeEnabled();
  await expect(
    page.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
  await stop.click();
  await expect(stop).toBeDisabled();
  await expect(timer).toHaveText("00:00:00");
  await expect(
    page.getByText("Which environment?", { exact: true }),
  ).toHaveCount(0);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("New turn after interrupt");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(stop).toBeEnabled();
  await expect(
    page.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
});

test("two distinct Escape presses interrupt; held keys and a session switch cannot confirm", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const stop = page.getByRole("button", { name: "Stop response", exact: true });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const clock = (await page.getByRole("timer").boundingBox())!;
  const send = (await page
    .getByRole("button", { name: "Send", exact: true })
    .boundingBox())!;
  const button = (await stop.boundingBox())!;
  expect(button.x + button.width).toBeLessThanOrEqual(clock.x);
  expect(clock.x + clock.width).toBeLessThan(send.x);
  await page.screenshot({ path: "test-results/turn-controls-mobile.png" });
  await input.focus();
  await page.keyboard.down("Escape");
  await page.keyboard.down("Escape");
  await expect(stop).toBeEnabled();
  await expect(stop).toHaveAttribute("data-armed", "true");
  await page.keyboard.up("Escape");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-1");
  await expect(stop).toBeDisabled();
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  await expect(stop).toBeEnabled();
  await input.focus();
  await page.keyboard.press("Escape");
  await expect(stop).toHaveAttribute("data-armed", "true");
  await expect(
    page.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(stop).toBeDisabled();
  await expect(page.getByRole("timer")).toHaveText("00:00:00");
});
