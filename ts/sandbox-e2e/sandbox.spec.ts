import { test, expect } from "@playwright/test";
test("WASM sandbox reuses the UI without backend, credentials or provider calls", async ({
  page,
}) => {
  const errors: string[] = [],
    backend: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("request", (r) => {
    if (/\/auth\/|\/cxz\./.test(r.url())) backend.push(r.url());
  });
  await page.goto("/sandbox.html");
  await expect(page.getByRole("heading", { name: "Current status" })).toBeVisible({
    timeout: 45000,
  });
  await expect(page.locator("body")).toHaveJSProperty(
    "scrollWidth",
    await page.evaluate(() => document.documentElement.clientWidth),
  );
  const message = page.getByRole("textbox", { name: "Message", exact: true });
  await message.fill("Test message");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.locator("summary").filter({ hasText: "Sample tasks completed" }),
  ).toBeVisible();
  // Shared pending-question UI sends the same Reply RPC through the WASM transport.
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  await expect(
    page.getByText("Which environment?", {
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("radio", { name: /Development/ }).check();
  await page.getByRole("button", { name: "Submit answers" }).click();
  await expect(
    page.getByText("Your selection was recorded for this preview."),
  ).toBeVisible();
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-5");
  await expect(
    page.locator("summary").filter({ hasText: "Simulated usage limit" }),
  ).toBeVisible();
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-3");
  await expect(
    page.getByText("History item 2100", { exact: false }),
  ).toBeVisible();
  await expect(page.locator(".transcript article")).toHaveCount(2000);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-7");
  await page.getByText("Actions", { exact: true }).click();
  await page
    .getByRole("button", { name: "Resume session", exact: true })
    .click();
  await expect(page.locator(".conversation header small")).toContainText(
    "idle",
  );
  await page.getByRole("button", { name: "Stop session", exact: true }).click();
  await expect(page.locator(".conversation header small")).toContainText(
    "stopped",
  );
  // Repeating a seed produces the same sequence. Reset drops prior conversations.
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-6");
  await message.fill("Run a sample");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.locator("summary").filter({ hasText: "Sample tasks completed" }),
  ).toBeVisible();
  const first = await page
    .locator("summary")
    .filter({ hasText: /^tool ·/ })
    .allTextContents();
  await page
    .getByRole("button", { name: "Reset sandbox", exact: true })
    .first()
    .click();
  await expect(message).toHaveValue("");
  await expect(page.getByText("Run a sample", { exact: true })).toHaveCount(0);
  await message.fill("Run a sample");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.locator("summary").filter({ hasText: "Sample tasks completed" }),
  ).toBeVisible();
  expect(
    await page
      .locator("summary")
      .filter({ hasText: /^tool ·/ })
      .allTextContents(),
  ).toEqual(first);
  expect(backend).toEqual([]);
  expect(errors).toEqual([]);
  await page.screenshot({
    path: "test-results/sandbox-mobile.png",
    fullPage: true,
  });
});
