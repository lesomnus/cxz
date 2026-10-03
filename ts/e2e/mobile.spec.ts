import { test, expect } from "@playwright/test";
test("mobile sign-in, conversation, questions, drafts and sign-out", async ({
  page,
  context,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(
    page.getByRole("button", { name: /Demo project/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: /Demo conversation/ }).click();
  await expect(page.getByRole("heading", { name: "Ready" })).toBeVisible();
  await expect(page.getByRole("radio", { name: /Development/ })).toBeVisible();
  await page.getByRole("radio", { name: /Development/ }).check();
  await page.getByRole("button", { name: "Submit answers" }).click();
  await expect(page.locator(".approval")).toHaveCount(0);
  expect(await page.evaluate(() => "pwned" in window)).toBe(false);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .fill("초안 preserved");
  await page.getByRole("button", { name: "Back to sessions" }).click();
  await page.getByRole("button", { name: /Demo conversation/ }).click();
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toHaveValue("초안 preserved");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toHaveValue("");
  await expect(
    page.locator("article.input").filter({ hasText: "초안 preserved" }),
  ).toHaveCount(1);
  await context.setOffline(true);
  await expect(page.getByText(/Disconnected · retrying/)).toBeVisible();
  await context.setOffline(false);
  await expect(page.getByText(/claude · idle · Live/)).toBeVisible({
    timeout: 15000,
  });
  await expect(
    page.locator("article.input").filter({ hasText: "초안 preserved" }),
  ).toHaveCount(1);
  await page.reload();
  await expect(
    page.getByRole("button", { name: /Demo conversation/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: /Demo conversation/ }).click();
  await expect(
    page.locator("article.input").filter({ hasText: "초안 preserved" }),
  ).toHaveCount(1);
  await page.screenshot({
    path: "test-results/mobile-conversation.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Back to sessions" }).click();
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(
    page.getByRole("button", { name: "Connect", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
