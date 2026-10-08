import { test, expect, devices } from "@playwright/test";
import { disableTerminalWebGL } from "../test-support/terminal";

test.beforeEach(async ({ page }) => disableTerminalWebGL(page));

test.describe("desktop terminal", () => {
  test.use({
    userAgent: devices["Desktop Chrome"].userAgent,
    isMobile: false,
    hasTouch: false,
    deviceScaleFactor: 1,
    viewport: { width: 1440, height: 900 },
  });
  test("Ctrl+Backquote opens a project shell and folding preserves its screen and draft", async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto("/sandbox.html");
    await expect(
      page.getByRole("heading", { name: "Current status" }),
    ).toBeVisible({ timeout: 45000 });
    const composer = page.getByRole("textbox", {
      name: "Message",
      exact: true,
    });
    await composer.fill("Keep my draft");
    await page.keyboard.press("Control+Backquote");
    const panel = page.getByRole("region", { name: "Workspace terminal" });
    await expect(panel).toBeVisible();
    await expect(panel).toContainText("sandbox@project-1:/workspace$");
    await expect(panel.locator(".xterm-helper-textarea")).toBeFocused();
    await page.keyboard.type("echo terminal-marker");
    await page.keyboard.press("Enter");
    await expect(panel).toContainText("terminal-marker");
    await page.keyboard.type("cat README.md");
    await page.keyboard.press("Enter");
    await expect(panel).toContainText("This workspace is simulated");
    await page.keyboard.press("Control+Backquote");
    await expect(panel).toBeHidden();
    await expect(composer).toHaveValue("Keep my draft");
    await expect(composer).toBeFocused();
    await page.keyboard.press("Control+Backquote");
    await expect(panel).toBeVisible();
    await expect(panel).toContainText("terminal-marker");
    await page.setViewportSize({ width: 1904, height: 1000 });
    await expect(page.locator(".conversation")).toHaveCSS("width", "800px");
    await expect(panel).toBeVisible();
    await page.keyboard.type("exit");
    await page.keyboard.press("Enter");
    await expect(panel).toContainText("Shell exited");
    await panel.getByRole("button", { name: "Reconnect" }).click();
    await expect(panel).toContainText("sandbox@project-1:/workspace$");
    await expect(panel).not.toContainText("terminal-marker");
    await page
      .getByLabel("Scenario", { exact: true })
      .selectOption("session-8");
    await page
      .getByRole("button", { name: "Session menu", exact: true })
      .click();
    await page
      .getByRole("menuitemcheckbox", { name: "Terminal", exact: true })
      .click();
    await expect(panel).toContainText("sandbox@project-2:/workspace$");
    expect(errors).toEqual([]);
    await page.screenshot({
      path: "test-results/workspace-terminal.png",
      fullPage: true,
    });
  });
});

test("mobile terminal button fits the conversation and keeps composer accessible", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  await page
    .getByRole("menuitemcheckbox", { name: "Terminal", exact: true })
    .click();
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  await expect(panel).toContainText("sandbox@project-1:/workspace$");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await panel.getByRole("button", { name: "Hide terminal" }).click();
  await expect(panel).toBeHidden();
});
