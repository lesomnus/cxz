import { test, expect, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("authenticated terminal runs a PTY shell under production CSP and preserves it while folded", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await page.getByRole("link", { name: /demo-chat/ }).click();
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(m.text());
  });
  await page.keyboard.press("Control+Backquote");
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  await expect(panel).toContainText("fixture$");
  await expect(panel.locator(".xterm-helper-textarea")).toBeFocused();
  await page.keyboard.type(
    "CXZ_WEB_KEEP=preserved; printf 'connected:%s\\n' pty",
  );
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("connected:pty");
  await page.keyboard.press("Control+Backquote");
  await expect(panel).toBeHidden();
  await page.keyboard.press("Control+Backquote");
  await expect(panel.locator(".xterm-helper-textarea")).toBeFocused();
  await page.keyboard.type("printf 'state:%s\\n' \"$CXZ_WEB_KEEP\"");
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("state:preserved");
  await page.keyboard.type("sleep 30");
  await page.keyboard.press("Enter");
  await page.keyboard.press("Control+c");
  await page.keyboard.type("printf 'interrupted:%s\\n' yes");
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("interrupted:yes");
  await page.setViewportSize({ width: 1000, height: 700 });
  await page.keyboard.type("stty size");
  await page.keyboard.press("Enter");
  await page.keyboard.type("exit");
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("Shell exited");
  await panel.getByRole("button", { name: "Reconnect" }).click();
  await expect(panel).toContainText("fixture$");
  await expect(panel).not.toContainText("state:preserved");
  expect(errors).toEqual([]);
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(panel).toHaveCount(0);
});
