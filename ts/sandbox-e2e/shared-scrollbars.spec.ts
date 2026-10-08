import {
  expect,
  test,
  devices,
  type Locator,
  type Page,
} from "@playwright/test";

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
  await page.mouse.move(bounds.x + 40, bounds.y + 40);
  await expect
    .poll(async () => (await skin(input)).color)
    .not.toBe("rgba(0, 0, 0, 0)");
  await expect(input).toHaveAttribute("data-scrollbar-near-y", "false");
  await page.mouse.move(bounds.x + bounds.width - 18, bounds.y + 40);
  await expect(input).toHaveAttribute("data-scrollbar-near-y", "true");
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
  await page.locator(".paste-chip").click();
  const card = page.getByRole("dialog", { name: "Paste source" });
  const body = card.locator(".card-body");
  await expect
    .poll(() => body.evaluate((el) => el.scrollHeight - el.clientHeight))
    .toBeGreaterThan(0);
  const bodyBounds = (await body.boundingBox())!;
  await page.mouse.move(
    bodyBounds.x + bodyBounds.width - 18,
    bodyBounds.y + 40,
  );
  await expect(body).toHaveAttribute("data-scrollbar-near-y", "true");
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
  const codeBounds = (await code.boundingBox())!;
  await page.mouse.move(
    codeBounds.x + 40,
    codeBounds.y + codeBounds.height - 5,
  );
  await expect(code).toHaveAttribute("data-scrollbar-near-x", "true");
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
  const choicesBounds = (await choices.boundingBox())!;
  await page.mouse.move(
    choicesBounds.x + choicesBounds.width - 18,
    choicesBounds.y + 5,
  );
  await expect(choices).toHaveAttribute("data-scrollbar-near-y", "true");
  expect((await skin(choices)).width).toBe("12px");
  await page.keyboard.press("Escape");
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
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
  const editorBounds = (await editor.boundingBox())!;
  await page.mouse.move(
    editorBounds.x + editorBounds.width - 20,
    editorBounds.y + 40,
  );
  await expect(editor).toHaveAttribute("data-scrollbar-near-y", "true");
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
  expect(
    await slider.evaluate(
      (el) => getComputedStyle(el, "::before").backgroundColor,
    ),
  ).toBe(panelColor);
});

test("terminal scrollback uses the same handle and keeps its own wheel scrolling", async ({
  page,
}) => {
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
  const bounds = (await root.boundingBox())!;
  await page.mouse.move(bounds.x + bounds.width - 20, bounds.y + 40);
  await expect(root).toHaveAttribute("data-scrollbar-near-y", "true");
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
