import { test, expect, devices } from "@playwright/test";
import {
  copyTerminalSelection,
  disableTerminalWebGL,
  loseTerminalContext,
} from "../test-support/terminal";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

async function openShell(page: import("@playwright/test").Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.keyboard.press("Control+Backquote");
  const panel = page.getByRole("region", { name: "Workspace terminal" });
  await expect(panel.locator(".xterm-helper-textarea")).toBeFocused();
  await expect(panel).toContainText("Simulated · /workspace");
  return panel;
}

test("WebGL paints, supports drag copying, and preserves selection and output after context loss", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const panel = await openShell(page);
  const canvas = panel.locator(".xterm-screen canvas").last();
  await expect(canvas).toBeVisible();
  // Verify actual painted text, rather than merely an empty canvas.
  await expect
    .poll(async () => {
      // Use a compositor screenshot: WebGL's default drawing buffer may have
      // already been cleared by the time an out-of-frame readPixels runs.
      const png: string = (await canvas.screenshot()).toString("base64");
      return page.evaluate(async (encoded) => {
        const bytes = Uint8Array.from(atob(encoded), (ch) => ch.charCodeAt(0));
        const image = await createImageBitmap(
          new Blob([bytes], { type: "image/png" }),
        );
        const snapshot = document.createElement("canvas");
        snapshot.width = image.width;
        snapshot.height = image.height;
        const context = snapshot.getContext("2d")!;
        context.drawImage(image, 0, 0);
        image.close();
        const pixels = context.getImageData(
          0,
          0,
          snapshot.width,
          snapshot.height,
        ).data;
        let min = 255,
          max = 0;
        for (let i = 0; i < pixels.length; i += 4) {
          min = Math.min(min, pixels[i]);
          max = Math.max(max, pixels[i]);
        }
        return max - min;
      }, png);
    })
    .toBeGreaterThan(60);

  const screen = (await panel.locator(".xterm-screen").boundingBox())!;
  await page.mouse.move(screen.x + 1, screen.y + 6);
  await page.mouse.down();
  await page.mouse.move(screen.x + 150, screen.y + 6, { steps: 8 });
  await page.mouse.up();
  const selected = await copyTerminalSelection(panel);
  expect(selected).toMatch(/^WASM simulated/);
  await loseTerminalContext(panel);
  await expect(panel.locator(".xterm-rows")).toBeVisible({ timeout: 10000 });
  await expect(panel.locator(".xterm-screen canvas")).toHaveCount(0);
  expect(await copyTerminalSelection(panel)).toBe(selected);
  await expect(panel).toContainText("WASM simulated shell");
  await page.keyboard.type("echo after-loss");
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("after-loss");
  expect(errors).toEqual([]);
});

test("unsupported WebGL keeps a working DOM shell", async ({ page }) => {
  await disableTerminalWebGL(page);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const panel = await openShell(page);
  await expect(panel.locator(".xterm-rows")).toBeVisible();
  await page.keyboard.type("echo without-gpu");
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("without-gpu");
  await expect(panel.locator(".xterm-screen canvas")).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("an optional renderer download failure does not interrupt the shell", async ({
  page,
}) => {
  let blocked = 0;
  await page.route(/addon-webgl.*\.js/, (route) => {
    blocked++;
    return route.abort();
  });
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const panel = await openShell(page);
  await page.keyboard.type("echo offline-renderer");
  await page.keyboard.press("Enter");
  await expect(panel).toContainText("offline-renderer");
  await expect(panel.locator(".xterm-screen canvas")).toHaveCount(0);
  expect(blocked).toBeGreaterThan(0);
  expect(errors).toEqual([]);
});
