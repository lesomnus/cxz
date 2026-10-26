import { test, expect } from "@playwright/test";

const editorModule = /\/packages\/nox\/src\/editor\/code-editor\.ts(?:\?.*)?$/;

test("a failed lazy editor request recovers through a fresh module URL without losing the page or draft", async ({
  page,
}) => {
  const attempts: string[] = [];
  await page.route(editorModule, async (route) => {
    attempts.push(route.request().url());
    if (attempts.length === 1) await route.abort("failed");
    else await route.continue();
  });
  await page.goto("/test-support/nox-editor.html");
  await page
    .getByRole("textbox", { name: "Draft", exact: true })
    .fill("Keep this draft");
  const lifetime = await page.evaluate(() => performance.timeOrigin);
  await page.getByRole("button", { name: "Open editor", exact: true }).click();
  await expect(page.locator(".monaco-editor")).toBeVisible();
  await expect(page.locator('.code-editor > [role="alert"]')).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Draft", exact: true }),
  ).toHaveValue("Keep this draft");
  expect(await page.evaluate(() => performance.timeOrigin)).toBe(lifetime);
  expect(attempts).toHaveLength(2);
  expect(new URL(attempts[0]!).searchParams.has("nox-editor-retry")).toBe(
    false,
  );
  expect(new URL(attempts[1]!).searchParams.has("nox-editor-retry")).toBe(true);
});

test("a persistent network failure stops after one recovery attempt and reports the error", async ({
  page,
}) => {
  let attempts = 0;
  await page.route(editorModule, async (route) => {
    attempts++;
    await route.abort("failed");
  });
  await page.goto("/test-support/nox-editor.html");
  await page.getByRole("button", { name: "Open editor", exact: true }).click();
  await expect(page.locator('.code-editor > [role="alert"]')).toContainText(
    "Failed to fetch dynamically imported module",
  );
  expect(attempts).toBe(2);
});

test("an editor evaluation error is reported without retrying", async ({
  page,
}) => {
  let attempts = 0;
  await page.route(editorModule, async (route) => {
    attempts++;
    await route.fulfill({
      contentType: "application/javascript",
      body: "throw new TypeError('Editor setup failed');",
    });
  });
  await page.goto("/test-support/nox-editor.html");
  await page.getByRole("button", { name: "Open editor", exact: true }).click();
  await expect(page.locator('.code-editor > [role="alert"]')).toHaveText(
    "TypeError: Editor setup failed",
  );
  expect(attempts).toBe(1);
});
