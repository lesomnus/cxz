import { test, expect, devices } from "@playwright/test";

// Playwright runs in Node; the shared browser tsconfig has no Node globals.
declare const process: { env: Record<string, string | undefined> };

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 2000, height: 1000 },
});

test("production file viewer loads under the gateway CSP", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  await page.getByRole("button", { name: /demo-chat/ }).click();
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  await expect(editor).toBeVisible();
  await editor.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText(
    "Fixture workspace",
    { timeout: 30000 },
  );
  await expect(editor.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    true,
  );
  await expect(editor.locator(".monaco-editor")).toHaveCSS(
    "background-color",
    "rgb(20, 20, 20)",
  );
  await expect(page.locator(".markdown [style]")).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("real container editor opens the workspace through the authenticated iframe", async ({
  page,
}) => {
  test.setTimeout(90000);
  test.skip(
    !process.env.CXZ_EDITOR_PROBE_CONTAINER,
    "opt-in real OpenVSCode/container probe",
  );
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await page.getByRole("button", { name: /demo-chat/ }).click();
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  await editor.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText(
    "Fixture workspace",
  );
  await editor.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(
    editor.getByRole("button", { name: "Disconnect", exact: true }),
  ).toBeVisible({ timeout: 45000 });
  const frame = page.frameLocator('iframe[title="VS Code workspace"]');
  await expect(frame.locator(".monaco-workbench")).toBeVisible({
    timeout: 45000,
  });
  const trust = frame.getByRole("button", { name: /Yes, I trust/ });
  // The workbench becomes visible before its first-use trust dialog appears.
  await trust.click({ timeout: 15000 });
  await expect(
    frame.getByText("README.md", { exact: true }).first(),
  ).toBeVisible({ timeout: 30000 });
  await frame.getByText("README.md", { exact: true }).first().dblclick();
  await expect(frame.locator(".editor-instance .view-lines")).toContainText(
    "Editor probe",
    {
      timeout: 30000,
    },
  );
  await expect(editor.locator("iframe")).toHaveCSS("filter", "grayscale(1)");
  await page.screenshot({
    path: "test-results/real-workspace-editor.png",
    fullPage: true,
  });
  await editor.getByRole("button", { name: "Disconnect", exact: true }).click();
  await expect(editor.locator("iframe")).toHaveCount(0);
  await expect(
    editor.getByRole("button", { name: "README.md", exact: true }),
  ).toBeVisible();
});
