import {
  test,
  expect,
  devices,
  type Locator,
  type Page,
} from "@playwright/test";
import { disableTerminalWebGL } from "../test-support/terminal";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});
test.beforeEach(async ({ page }) => {
  await disableTerminalWebGL(page);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByRole("button", { name: "Terminal", exact: true }).click();
});
async function command(page: Page, pane: Locator, text: string) {
  await expect(pane.locator(".xterm-helper-textarea:visible")).toBeFocused();
  await page.keyboard.type(text);
  await page.keyboard.press("Enter");
  await expect(pane.locator(".terminal-screen:visible")).toContainText(text);
}
test("tabs own independent shells; folding, reordering, moving and closing preserve the surviving shells", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  let left = panel.locator("[data-terminal-group]").first();
  await command(page, left, "echo first-shell-marker");
  await left.getByRole("button", { name: "New terminal" }).click();
  await command(page, left, "echo second-shell-marker");
  await page.keyboard.type("echo unfinished-shell-marker");
  await expect(
    left.getByRole("tab", { name: "Terminal 2", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(left.locator(".terminal-screen:visible")).not.toContainText(
    "first-shell-marker",
  );
  await left.getByRole("tab", { name: "Terminal 1", exact: true }).click();
  await expect(left.locator(".terminal-screen:visible")).toContainText(
    "first-shell-marker",
  );
  await expect(left.locator(".terminal-screen:visible")).not.toContainText(
    "second-shell-marker",
  );

  await left
    .locator('[data-terminal-tab="terminal-2"]')
    .dragTo(left.locator('[data-terminal-tab="terminal-1"]'), {
      targetPosition: { x: 2, y: 12 },
    });
  await expect(left.getByRole("tab").first()).toHaveText("Terminal 2");
  await left.getByRole("button", { name: "Terminal layout" }).click();
  await page.getByRole("menuitem", { name: "Split right" }).click();
  await expect(panel.locator("[data-terminal-group]")).toHaveCount(2);
  const right = panel.locator("[data-terminal-group]").last();
  await command(page, right, "echo third-shell-marker");
  await left
    .locator('[data-terminal-tab="terminal-2"]')
    .dragTo(right.getByRole("tabpanel"));
  await expect(
    right.getByRole("tab", { name: "Terminal 2", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(right.locator(".terminal-screen:visible")).toContainText(
    "second-shell-marker",
  );
  await expect(right.locator(".xterm-helper-textarea:visible")).toBeFocused();
  await page.keyboard.press("Enter");
  await expect
    .poll(async () =>
      (
        await right
          .locator(".terminal-screen:visible .xterm-rows > div")
          .allTextContents()
      ).map((text) => text.trim()),
    )
    .toContain("unfinished-shell-marker");
  await command(page, right, "echo moved-shell-marker");
  await right.getByRole("tab", { name: "Terminal 3", exact: true }).click();
  await expect(right.locator(".terminal-screen:visible")).toContainText(
    "third-shell-marker",
  );
  await expect(right.locator(".terminal-screen:visible")).not.toContainText(
    "moved-shell-marker",
  );

  await panel.getByRole("button", { name: "Hide terminal" }).click();
  await expect(panel).toBeHidden();
  await page.getByRole("button", { name: "Terminal", exact: true }).click();
  await expect(panel.locator("[data-terminal-group]")).toHaveCount(2);
  await right
    .getByRole("button", { name: "Close Terminal 3", exact: true })
    .click();
  await expect(right.locator(".terminal-screen:visible")).toContainText(
    "moved-shell-marker",
  );
  await right
    .getByRole("button", { name: "Close Terminal 2", exact: true })
    .click();
  await expect(panel.locator("[data-terminal-group]")).toHaveCount(1);
  left = panel.locator("[data-terminal-group]").first();
  await expect(left.locator(".terminal-screen:visible")).toContainText(
    "first-shell-marker",
  );
  await left
    .getByRole("button", { name: "Close Terminal 1", exact: true })
    .click();
  await expect(panel).toBeHidden();
  await page.getByRole("button", { name: "Terminal", exact: true }).click();
  await expect(panel.getByRole("tab")).toHaveText("Terminal 4");
  await expect(panel).toContainText("sandbox@project-1:/workspace$");
  await expect(panel).not.toContainText("first-shell-marker");
  expect(errors).toEqual([]);
});

test("edge drops create nested panes and both split separators and the panel height can be resized", async ({
  page,
}) => {
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  const first = panel.locator("[data-terminal-group]").first();
  await first.getByRole("button", { name: "New terminal" }).click();
  await command(page, first, "echo dock-marker");
  const slot = first.getByRole("tabpanel");
  const rect = (await slot.boundingBox())!;
  await first.locator('[data-terminal-tab="terminal-2"]').dragTo(slot, {
    targetPosition: { x: rect.width - 8, y: rect.height / 2 },
  });
  await expect(panel.locator("[data-terminal-group]")).toHaveCount(2);
  const right = panel.locator("[data-terminal-group]").last();
  await expect(right.locator(".terminal-screen:visible")).toContainText(
    "dock-marker",
  );
  const separator = panel.getByRole("separator", { name: "Resize panes" });
  const previous = (await first.boundingBox())!.width;
  await separator.focus();
  await page.keyboard.press("Shift+ArrowRight");
  await expect
    .poll(async () => (await first.boundingBox())!.width)
    .toBeGreaterThan(previous + 50);
  const handle = (await separator.boundingBox())!;
  await page.mouse.move(
    handle.x + handle.width / 2,
    handle.y + handle.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(handle.x - 100, handle.y + handle.height / 2, {
    steps: 6,
  });
  await page.mouse.up();
  await expect
    .poll(async () => (await first.boundingBox())!.width)
    .toBeLessThan(previous + 30);
  await right.getByRole("button", { name: "Terminal layout" }).click();
  await page.getByRole("menuitem", { name: "Split down" }).click();
  await expect(panel.locator("[data-terminal-group]")).toHaveCount(3);
  await expect(
    panel.getByRole("separator", { name: "Resize panes" }),
  ).toHaveCount(2);
  const top = panel.getByRole("separator", { name: "Resize terminal" });
  const beforeHeight = (await panel.boundingBox())!.height;
  const topBounds = (await top.boundingBox())!;
  await page.mouse.move(topBounds.x + topBounds.width / 2, topBounds.y + 2);
  await page.mouse.down();
  await page.mouse.move(topBounds.x + topBounds.width / 2, topBounds.y - 100, {
    steps: 6,
  });
  await page.mouse.up();
  await expect
    .poll(async () => (await panel.boundingBox())!.height)
    .toBeGreaterThan(beforeHeight + 80);
  await expect(
    panel.getByRole("tab", { name: "Terminal 2", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({ path: "test-results/terminal-docking-mobile.png" });
});
