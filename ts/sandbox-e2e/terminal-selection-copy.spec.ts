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

async function ready(page: Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
}
async function shell(page: Page) {
  await ready(page);
  await page.keyboard.press("Control+Backquote");
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  await expect(panel).toContainText("Simulated · /workspace");
  await expect(panel.locator(".xterm-helper-textarea")).toBeFocused();
  return panel;
}
async function drag(page: Page, panel: Locator) {
  const box = (await panel.locator(".xterm-screen").boundingBox())!;
  await page.mouse.move(box.x + 1, box.y + 6);
  await page.mouse.down();
  await page.mouse.move(box.x + 150, box.y + 6, { steps: 8 });
}
async function general(page: Page) {
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
  await page
    .getByRole("navigation", { name: "Settings topics" })
    .getByRole("link", { name: "General", exact: true })
    .click();
  return page.getByRole("switch", { name: "Copy on selection", exact: true });
}

for (const renderer of ["WebGL", "DOM"]) {
  test(`${renderer} copies only the completed selection and confirms the successful write`, async ({
    page,
    context,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.addInitScript(() => {
      const write = navigator.clipboard.writeText.bind(navigator.clipboard);
      (window as any).terminalCopyWrites = 0;
      Object.defineProperty(navigator.clipboard, "writeText", {
        value: (text: string) => {
          (window as any).terminalCopyWrites++;
          return write(text);
        },
      });
    });
    if (renderer === "DOM") await disableTerminalWebGL(page);
    const panel = await shell(page);
    if (renderer === "WebGL")
      await expect(panel.locator(".xterm-screen canvas").last()).toBeVisible();
    await page.evaluate(() => navigator.clipboard.writeText("unchanged"));
    await page.evaluate(() => {
      (window as any).terminalCopyWrites = 0;
    });
    await drag(page, panel);
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      "unchanged",
    );
    expect(await page.evaluate(() => (window as any).terminalCopyWrites)).toBe(
      0,
    );
    await page.mouse.up();
    await expect(panel.locator(".terminal-clipboard-feedback")).toHaveText(
      "Copied",
    );
    expect(await page.evaluate(() => navigator.clipboard.readText())).toMatch(
      /^WASM simulated/,
    );
    expect(await page.evaluate(() => (window as any).terminalCopyWrites)).toBe(
      1,
    );
  });
}

test("the General toggle persists and updates an open shell across tabs without replacing it", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await disableTerminalWebGL(page);
  const panel = await shell(page);
  await page.keyboard.type("echo preserved-shell");
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("preserved-shell");
  const other = await context.newPage();
  await ready(other);
  const toggle = await general(other);
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          JSON.parse(localStorage.getItem("settings")!)[
            "terminal.copyOnSelect"
          ],
      ),
    )
    .toBe(false);
  await page.evaluate(() => navigator.clipboard.writeText("unchanged"));
  await drag(page, panel);
  await page.mouse.up();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    "unchanged",
  );
  await expect(panel.locator(".terminal-clipboard-feedback")).toBeEmpty();
  await expect(panel).toContainText("preserved-shell");
  await other.reload();
  await expect(
    other.getByRole("switch", { name: "Copy on selection" }),
  ).toHaveAttribute("aria-checked", "false");
  await other.getByRole("switch", { name: "Copy on selection" }).press("Space");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          JSON.parse(localStorage.getItem("settings")!)[
            "terminal.copyOnSelect"
          ],
      ),
    )
    .toBe(true);
  await drag(page, panel);
  await page.mouse.up();
  await expect(panel.locator(".terminal-clipboard-feedback")).toHaveText(
    "Copied",
  );
  await expect(panel).toContainText("preserved-shell");
});

test("clipboard rejection reports failure and a subsequent gesture can retry", async ({
  page,
}) => {
  await disableTerminalWebGL(page);
  await page.addInitScript(() => {
    let reject = true;
    Object.defineProperty(navigator.clipboard, "writeText", {
      value: async () => {
        if (reject) {
          reject = false;
          throw new DOMException("Denied", "NotAllowedError");
        }
      },
    });
  });
  const panel = await shell(page);
  await drag(page, panel);
  await page.mouse.up();
  await expect(panel.locator(".terminal-clipboard-feedback")).toHaveText(
    "Copy failed",
  );
  await drag(page, panel);
  await page.mouse.up();
  await expect(panel.locator(".terminal-clipboard-feedback")).toHaveText(
    "Copied",
  );
});
