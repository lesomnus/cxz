import { test, expect, devices } from "@playwright/test";
import { createHash } from "node:crypto";
import {
  copyTerminalSelection,
  disableTerminalWebGL,
  loseTerminalContext,
} from "../test-support/terminal";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("large native paste preserves bytes and sustained output drains before shell exit", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await disableTerminalWebGL(page);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await page.getByRole("link", { name: /demo-chat/ }).click();
  await page.keyboard.press("Control+Backquote");
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  const input = panel.locator(".xterm-helper-textarea");
  await expect(panel).toContainText("fixture$");
  const text = "terminal-throughput-".repeat(12000);
  const digest = createHash("sha256").update(text).digest("hex");
  await page.evaluate(
    (command) => navigator.clipboard.writeText(command),
    `stty raw -echo; printf 'PASTE-READY\\n'; dd bs=${text.length} count=1 iflag=fullblock of=paste.txt 2>/dev/null; stty sane; printf 'paste-bytes:'; wc -c < paste.txt; sha256sum paste.txt`,
  );
  await input.press("Control+v");
  await input.press("Enter");
  await expect
    .poll(async () =>
      (await panel.locator(".xterm-rows > div").allTextContents()).map((text) =>
        text.trimEnd(),
      ),
    )
    .toContain("PASTE-READY");
  await page.evaluate((value) => navigator.clipboard.writeText(value), text);
  await input.press("Control+v");
  await expect(panel).toContainText(`paste-bytes:${text.length}`);
  await expect(panel).toContainText(digest);
  await page.evaluate(() =>
    navigator.clipboard.writeText(
      "awk 'BEGIN { for (i=0;i<10000;i++) printf \"burst:%05d abcdefghijklmnopqrstuvwxyz\\n\",i }'; printf 'BURST-DRAINED\\n'; exit",
    ),
  );
  await input.press("Control+v");
  await input.press("Enter");
  await expect(panel).toContainText("burst:09999");
  await expect(panel).toContainText("BURST-DRAINED");
  await expect(panel).toContainText("Shell exited");
  expect(errors).toEqual([]);
});

test("terminal keyboard shortcuts paste once, erase words and select without editing the PTY input", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await disableTerminalWebGL(page);
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await page.getByRole("link", { name: /demo-chat/ }).click();
  await page.keyboard.press("Control+Backquote");
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  const input = panel.locator(".xterm-helper-textarea");
  await expect(panel).toContainText("fixture$");
  await page.evaluate(() =>
    navigator.clipboard.writeText("printf 'result:%s\\n' preserved discarded"),
  );
  await input.press("Control+v");
  await expect(panel).toContainText("preserved discarded");
  await input.press("Control+Backspace");
  await expect(panel).not.toContainText("discarded");
  const cursor = panel.locator(".xterm-cursor");
  const beforeBackspace = (await cursor.boundingBox())!.x;
  await input.press("Backspace");
  await expect
    .poll(async () => (await cursor.boundingBox())!.x)
    .toBeLessThan(beforeBackspace);
  await input.press("Shift+ArrowLeft");
  await expect.poll(() => copyTerminalSelection(panel)).toBe("d");
  await input.press("Shift+ArrowLeft");
  expect(await copyTerminalSelection(panel)).toBe("ed");
  await input.press("Shift+ArrowRight");
  expect(await copyTerminalSelection(panel)).toBe("d");
  await input.press("Escape");
  await input.press("Enter");
  await expect
    .poll(() => panel.locator(".xterm-rows > div").allTextContents())
    .toContain("result:preserved");
  // Run a raw, alternate-screen reader to check the application receives
  // Shift+Left intact. Its own readiness text avoids entering keys too early.
  await page.evaluate(() =>
    navigator.clipboard.writeText(
      "stty raw -echo; printf '\\033[?1049h\\033[H\\033[2JALT-READY'; CXZ_KEY_BYTES=$(dd bs=1 count=6 2>/dev/null | od -An -tx1); printf '\\033[?1049l'; stty sane; printf 'key:%s\\n' \"$CXZ_KEY_BYTES\"",
    ),
  );
  await input.press("Control+v");
  await input.press("Enter");
  // The echoed command also contains this string; require the alternate
  // screen's standalone row instead of matching the whole panel.
  const rows = panel.locator(".xterm-rows > div");
  await expect
    .poll(async () =>
      (await rows.allTextContents()).map((text) => text.trimEnd()),
    )
    .toContain("ALT-READY");
  await input.press("Shift+ArrowLeft");
  await expect
    .poll(async () =>
      (await rows.allTextContents()).map((text) => text.trimEnd()),
    )
    .not.toContain("ALT-READY");
  await expect(panel).toContainText("key: 1b 5b 31 3b 32 44");
  await expect(panel).toContainText("fixture$");
  expect(await copyTerminalSelection(panel)).toBe("");
});

test("authenticated terminal runs a PTY shell under production CSP and preserves it while folded", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await disableTerminalWebGL(page);
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
  const screen = (await panel.locator(".xterm-screen").boundingBox())!;
  await page.mouse.move(screen.x + 1, screen.y + 6);
  await page.mouse.down();
  await page.mouse.move(screen.x + 45, screen.y + 6, { steps: 8 });
  await page.mouse.up();
  await expect(panel.locator(".terminal-clipboard-feedback")).toHaveText(
    "Copied",
  );
  expect(await page.evaluate(() => navigator.clipboard.readText())).toMatch(
    /^fixt/,
  );
  await page.keyboard.type(
    "CXZ_WEB_KEEP=preserved; printf 'connected:%s\\n' pty",
  );
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("connected:pty");
  await page.keyboard.type("printf '\\033[31mpalette-red\\033[0m\\n'");
  await page.keyboard.press("Enter");
  await expect(panel.locator(".xterm-fg-1").last()).toHaveCSS(
    "color",
    "rgb(255, 135, 159)",
  );
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

test("WebGL works under production CSP and context loss keeps the PTY alive", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await page.getByRole("link", { name: /demo-chat/ }).click();
  await page.keyboard.press("Control+Backquote");
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  await expect(panel.locator(".xterm-screen canvas").last()).toBeVisible();
  await expect(panel.locator(".xterm-helper-textarea")).toBeFocused();
  await page.keyboard.type(
    "CXZ_WEB_KEEP=webgl; printf 'before-loss:%s\\n' ready",
  );
  await page.keyboard.press("Enter");
  await loseTerminalContext(panel);
  await expect(panel.locator(".xterm-rows")).toBeVisible({ timeout: 10000 });
  await expect(panel).toContainText("before-loss:ready");
  await expect(panel.locator(".xterm-screen canvas")).toHaveCount(0);
  await page.keyboard.type("printf 'after-loss:%s\\n' \"$CXZ_WEB_KEEP\"");
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("after-loss:webgl");
  expect(errors).toEqual([]);
});
