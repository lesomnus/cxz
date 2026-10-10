import { test, expect, devices } from "@playwright/test";
import {
  copyTerminalSelection,
  disableTerminalWebGL,
} from "../test-support/terminal";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

for (const renderer of ["DOM", "WebGL"]) {
  test(`${renderer} supports native Ctrl+V, word erase and reversible keyboard selection`, async ({
    page,
    context,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    if (renderer === "DOM") await disableTerminalWebGL(page);
    await page.goto("/sandbox.html");
    await expect(
      page.getByRole("heading", { name: "Current status" }),
    ).toBeVisible({ timeout: 45000 });
    await page.keyboard.press("Control+Backquote");
    const panel = page.getByRole("region", { name: "Workspace terminal" });
    const input = panel.locator(".xterm-helper-textarea");
    await expect(input).toBeFocused();
    await expect(panel).toContainText("Simulated · /workspace");
    if (renderer === "WebGL")
      await expect(panel.locator(".xterm-screen canvas").last()).toBeVisible();
    await page.evaluate(() =>
      navigator.clipboard.writeText("echo keyboard-paste discarded"),
    );
    await input.press("Control+v");
    await input.press("Control+Backspace");
    await input.press("Backspace");
    // Wait for streamed echo parsing through the public selection surface.
    // Escape only dismisses the selection; selecting never edits shell input.
    await expect
      .poll(async () => {
        await input.press("Escape");
        for (let i = 0; i < 5; i++) await input.press("Shift+ArrowLeft");
        return copyTerminalSelection(panel);
      })
      .toBe("paste");
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe("paste");
    await input.press("Shift+ArrowRight");
    expect(await copyTerminalSelection(panel)).toBe("aste");
    for (let i = 0; i < 4; i++) await input.press("Shift+ArrowRight");
    expect(await copyTerminalSelection(panel)).toBe("");
    await input.press("Shift+ArrowLeft");
    expect(await copyTerminalSelection(panel)).toBe("e");
    await input.press("Shift+ArrowUp");
    expect((await copyTerminalSelection(panel)).length).toBeGreaterThan(5);
    await input.press("Shift+ArrowDown");
    expect(await copyTerminalSelection(panel)).toBe("e");
    await input.press("Escape");
    expect(await copyTerminalSelection(panel)).toBe("");
    await input.press("Enter");
    if (renderer === "DOM") {
      await expect
        .poll(() => panel.locator(".xterm-rows > div").allTextContents())
        .toContain("keyboard-paste");
      await expect(panel).not.toContainText("discarded");
    }
  });
}
