import { inspectPaste } from "../test-support/paste-chip";
import {
  expect,
  test,
  devices,
  type Locator,
  type Page,
} from "@playwright/test";
import { disableTerminalWebGL } from "../test-support/terminal";

test.use({
  userAgent: devices["Desktop Chrome"].userAgent,
  // Playwright hides native scrollbars by default; exercise the actual painted
  // gutter as well as the CSS state and the app's separate custom handles.
  launchOptions: { ignoreDefaultArgs: ["--hide-scrollbars"] },
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
  deviceScaleFactor: 1,
});

async function ready(page: Page) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
}
/**
 * nearEdge hovers a point measured from the surface's own box and waits for the
 * surface to agree that the pointer is near its scrollbar.
 *
 * Measuring and then moving is a race: the box is read before the pointer
 * moves, and anything that reflows in between -- a font settling, a card
 * resizing, a summary arriving -- leaves the pointer resting somewhere
 * harmless. The attribute then never arrives, and waiting longer cannot help,
 * because nothing is on its way. So the whole gesture retries rather than just
 * the assertion. See cxz#184.
 */
async function nearEdge(
  surface: Locator,
  attribute: string,
  at: (box: { width: number; height: number }) => { x: number; y: number },
  want = "true",
) {
  await expect
    .poll(async () => {
      const box = await surface.boundingBox();
      if (!box) return null;
      const point = at(box);
      await surface.page().mouse.move(box.x + point.x, box.y + point.y);
      return surface.getAttribute(attribute);
    })
    .toBe(want);
}
async function skin(surface: Locator) {
  return surface.evaluate((el) => {
    const bar = getComputedStyle(el, "::-webkit-scrollbar");
    const thumb = getComputedStyle(el, "::-webkit-scrollbar-thumb");
    const track = getComputedStyle(el, "::-webkit-scrollbar-track");
    return {
      width: bar.width,
      height: bar.height,
      color: thumb.backgroundColor,
      track: track.backgroundColor,
      clientWidth: el.clientWidth,
    };
  });
}

test("composer and detail cards share handle-only hover styling without shifting text, selection or native scrolling", async ({
  page,
}) => {
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill(
    Array.from({ length: 120 }, (_, i) => `line ${i}`).join("\n"),
  );
  await input.evaluate((el: HTMLTextAreaElement) => {
    el.scrollTop = 0;
    el.blur();
  });
  await page.mouse.move(0, 0);
  await expect
    .poll(async () => (await skin(input)).color)
    .toBe("rgba(0, 0, 0, 0)");
  const before = await skin(input);
  const bounds = (await input.boundingBox())!;
  await nearEdge(
    input,
    "data-scrollbar-near-y",
    () => ({ x: 40, y: 40 }),
    "false",
  );
  await expect
    .poll(async () => (await skin(input)).color)
    .not.toBe("rgba(0, 0, 0, 0)");
  await nearEdge(input, "data-scrollbar-near-y", (box) => ({
    x: box.width - 18,
    y: 40,
  }));
  expect((await skin(input)).clientWidth).toBe(before.clientWidth);
  expect(before.width).toBe("12px");
  expect(
    await input.evaluate(
      (el) => (el as HTMLElement).offsetWidth - el.clientWidth,
    ),
  ).toBe(12);
  expect(before.track).toBe("rgba(0, 0, 0, 0)");
  await page.mouse.wheel(0, 140);
  await expect
    .poll(() => input.evaluate((el) => el.scrollTop))
    .toBeGreaterThan(0);
  await expect
    .poll(() =>
      input.evaluate((el) =>
        Math.abs(
          el.scrollTop +
            parseFloat(
              (el.parentElement!.querySelector(".editor-mirror") as HTMLElement)
                .style.top,
            ),
        ),
      ),
    )
    .toBeLessThan(1);
  await input.fill("");
  await page.evaluate(() =>
    navigator.clipboard.writeText("detail line\n".repeat(200)),
  );
  await input.focus();
  await input.press("Control+v");
  await inspectPaste(page, page.locator(".paste-chip"));
  const card = page.getByRole("dialog", { name: "Paste source" });
  const body = card.locator(".card-body");
  await expect
    .poll(() => body.evaluate((el) => el.scrollHeight - el.clientHeight))
    .toBeGreaterThan(0);
  await nearEdge(body, "data-scrollbar-near-y", (box) => ({
    x: box.width - 18,
    y: 40,
  }));
  expect((await skin(body)).width).toBe(before.width);
  expect((await skin(body)).track).toBe(before.track);
  await page.mouse.wheel(0, 160);
  await expect
    .poll(() => body.evaluate((el) => el.scrollTop))
    .toBeGreaterThan(0);
  const footer = await input.inputValue();
  await page.screenshot({ path: "test-results/shared-scrollbars.png" });
  await page.keyboard.press("Escape");
  await expect(card).toHaveCount(0);
  await expect(input).toHaveValue(footer);
});

test("horizontal code, dropdowns and Monaco reuse the panel handle styling", async ({
  page,
}) => {
  await ready(page);
  // A wide Markdown code sample exercises the horizontal native scrollbar.
  await page
    .locator(".markdown")
    .first()
    .evaluate((el) => {
      const pre = document.createElement("pre");
      pre.textContent = "long-code ".repeat(120);
      pre.dataset.scrollFixture = "true";
      el.append(pre);
    });
  const code = page.locator("pre[data-scroll-fixture]");
  await code.scrollIntoViewIfNeeded();
  await nearEdge(code, "data-scrollbar-near-x", (box) => ({
    x: 40,
    y: box.height - 5,
  }));
  expect((await skin(code)).height).toBe("12px");
  await code.evaluate((el) => {
    el.scrollLeft = 100;
  });
  expect(await code.evaluate((el) => el.scrollLeft)).toBe(100);
  await page.getByRole("combobox", { name: "Model", exact: true }).click();
  const choices = page.locator(".setting-options");
  await choices.evaluate((el: HTMLElement) => {
    el.style.maxHeight = "30px";
  });
  const menuArea = page.locator('.setting-menu [data-scroll-area="root"]');
  const menuHandle = menuArea.locator('[data-scroll-area="handle"]');
  await expect(menuArea.getByRole("scrollbar")).toBeVisible();
  await nearEdge(menuArea, "data-near", (box) => ({
    x: box.width - 18,
    y: box.height / 2,
  }));
  await expect(menuHandle).toHaveCSS("width", "4px");
  await expect(menuHandle).toHaveCSS("opacity", "1");
  await expect(menuHandle).toHaveCSS("transform", "matrix(2, 0, 0, 1, 0, 0)");
  const menuColor = await menuHandle.evaluate(
    (el) => getComputedStyle(el).backgroundColor,
  );
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page.getByRole("menuitem", { name: "Settings", exact: true }).click();
  await page.getByRole("button", { name: /^Edit settings\.json/ }).click();
  const editor = page.locator(".settings-file .monaco-editor");
  await expect(editor).toBeVisible({ timeout: 30000 });
  const json = page.getByRole("textbox", {
    name: "settings.json",
    exact: true,
  });
  await page.evaluate(() =>
    navigator.clipboard.writeText(
      "{\n" + '  "editor.tabSize": 4,\n'.repeat(100) + "}",
    ),
  );
  await json.press("Control+a");
  await json.press("Control+v");
  const slider = editor.locator(".scrollbar.vertical > .slider").first();
  await expect(slider).toBeVisible();
  await expect(slider).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await nearEdge(editor, "data-scrollbar-near-y", (box) => ({
    x: box.width - 20,
    y: 40,
  }));
  await expect
    .poll(() =>
      slider.evaluate((el) => getComputedStyle(el, "::before").transform),
    )
    .toBe("matrix(2, 0, 0, 1, 0, 0)");
  expect(
    await slider.evaluate((el) => getComputedStyle(el, "::before").width),
  ).toBe("4px");
  const panelColor = await page.evaluate(() => {
    const probe = document.createElement("span");
    probe.style.backgroundColor = "var(--scroll-handle-color)";
    document.body.append(probe);
    const color = getComputedStyle(probe).backgroundColor;
    probe.remove();
    return color;
  });
  expect(menuColor).toBe(panelColor);
  expect(
    await slider.evaluate(
      (el) => getComputedStyle(el, "::before").backgroundColor,
    ),
  ).toBe(panelColor);
});

test("terminal scrollback uses the same handle and keeps its own wheel scrolling", async ({
  page,
}) => {
  await disableTerminalWebGL(page);
  await ready(page);
  await page.keyboard.press("Control+Backquote");
  const terminal = page.getByRole("region", {
    name: "Workspace terminal",
    exact: true,
  });
  await expect(terminal).toContainText("sandbox@project-1:/workspace$");
  await page.keyboard.insertText("pwd\r".repeat(24) + "echo scroll-ready\r");
  await expect
    .poll(
      async () =>
        ((await terminal.textContent())?.match(/scroll-ready/g) ?? []).length,
    )
    .toBeGreaterThanOrEqual(2);
  const root = terminal.locator(".xterm");
  const slider = root.locator(".scrollbar.vertical > .slider").first();
  await expect(slider).toBeVisible();
  await expect
    .poll(
      async () =>
        (await slider.boundingBox())!.height /
        (await root.boundingBox())!.height,
    )
    .toBeLessThan(0.8);
  await expect(slider).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await nearEdge(root, "data-scrollbar-near-y", (box) => ({
    x: box.width - 20,
    y: 40,
  }));
  await expect
    .poll(() =>
      slider.evaluate((el) => getComputedStyle(el, "::before").transform),
    )
    .toBe("matrix(2, 0, 0, 1, 0, 0)");
  const bottom = (await slider.boundingBox())!.y;
  await page.mouse.wheel(0, -160);
  await expect
    .poll(async () => (await slider.boundingBox())!.y)
    .toBeLessThan(bottom);
});
