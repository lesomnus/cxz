import { test, expect } from "@playwright/test";
import { disableTerminalWebGL } from "../test-support/terminal";

test("terminal docking story opens independent xterm shells and nested panes", async ({
  page,
}) => {
  await disableTerminalWebGL(page);
  await page.goto("/iframe.html?id=workspace-terminal--docking&viewMode=story");
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  await expect(panel).toContainText("Storybook shell · no server connection");
  await panel.getByRole("button", { name: "New terminal" }).click();
  await expect(
    panel.getByRole("tab", { name: "Terminal 2", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  const input = panel.locator(".xterm-helper-textarea:visible");
  await expect(input).toBeFocused();
  await input.pressSequentially("echo story-marker");
  await input.press("Enter");
  await expect(panel.locator(".terminal-screen:visible")).toContainText(
    "story-marker",
  );
  await panel.getByRole("button", { name: "New terminal" }).click();
  const slot = panel.getByRole("tabpanel");
  const bounds = (await slot.boundingBox())!;
  await panel.locator('[data-terminal-tab="terminal-3"]').dragTo(slot, {
    targetPosition: { x: bounds.width / 2, y: bounds.height - 8 },
  });
  await expect(panel.locator("[data-terminal-group]")).toHaveCount(2);
  await panel
    .getByRole("button", { name: "Close Terminal 3", exact: true })
    .click();
  await expect(panel.locator("[data-terminal-group]")).toHaveCount(1);
  await expect(panel.locator(".terminal-screen:visible")).toContainText(
    "story-marker",
  );
});
