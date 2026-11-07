import { expect, test, type Page } from "@playwright/test";

async function menu(page: Page) {
  await page.getByRole("button", { name: "File actions", exact: true }).click();
  return page.getByRole("menu", { name: "File actions", exact: true });
}
async function open(page: Page, story = "tabs", theme = "dark") {
  await page.goto(
    `/iframe.html?id=workspace-file-editor--${story}&viewMode=story&globals=theme:${theme}`,
  );
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  await expect(editor).toBeVisible();
  return editor;
}

test("file tab actions copy the current path, retain the active file and close every tab", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const editor = await open(page);
  await expect(editor.getByRole("tab")).toHaveCount(3);
  await editor
    .getByRole("button", { name: "Copy file path", exact: true })
    .click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("/workspace/README.md");
  await editor.getByRole("tab", { name: "package.json", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText(
    '"workspace-preview"',
  );
  await expect(
    editor.getByRole("button", { name: "Copy file path", exact: true }),
  ).not.toHaveAttribute("aria-busy", "true");
  await editor
    .getByRole("button", { name: "Copy file path", exact: true })
    .focus();
  await page.keyboard.press("Enter");
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("/workspace/package.json");
  await (await menu(page))
    .getByRole("menuitem", { name: "Copy file path", exact: true })
    .click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("/workspace/package.json");
  await (await menu(page))
    .getByRole("menuitem", { name: "Close other tabs", exact: true })
    .click();
  await expect(editor.getByRole("tab")).toHaveCount(1);
  await expect(
    editor.getByRole("tab", { name: "package.json", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  const actions = await menu(page);
  await expect(
    actions.getByRole("menuitem", { name: "Close other tabs", exact: true }),
  ).toBeDisabled();
  await actions
    .getByRole("menuitem", { name: "Close all tabs", exact: true })
    .click();
  await expect(editor.getByRole("tab")).toHaveCount(0);
  await expect(editor).toContainText("Select a file to preview.");
  const empty = await menu(page);
  for (const name of ["Copy file path", "Close all tabs", "Close other tabs"])
    await expect(
      empty.getByRole("menuitem", { name, exact: true }),
    ).toBeDisabled();
  await page.keyboard.press("Escape");
  await editor.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(
    editor.getByRole("tab", { name: "README.md", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(editor.locator(".view-lines")).toContainText(
    "Workspace preview",
  );
});

for (const theme of ["dark", "light"])
  test(`viewer clips Monaco corners and balances tab and language control insets in ${theme}`, async ({
    page,
  }, testInfo) => {
    const editor = await open(page, "tabs", theme);
    await expect(editor.locator(":scope > header")).toHaveCount(0);
    await expect(editor.locator(".file-explorer-heading")).toHaveCount(0);
    await expect(
      editor.getByRole("button", { name: "Refresh files", exact: true }),
    ).toHaveCount(0);
    await expect(editor.locator(".monaco-editor")).toBeVisible();
    const surface = await editor
      .locator("[data-workspace-viewer]")
      .evaluate((viewer) => {
        const content = viewer.querySelector<HTMLElement>(
          "[data-workspace-content]",
        )!;
        const explorer = viewer
          .closest(".file-workbench")!
          .querySelector(".file-explorer")!;
        const monaco = content.querySelector(".monaco-editor")!;
        const css = getComputedStyle(content);
        const outer = getComputedStyle(viewer);
        const source = monaco.closest(".code-editor")!;
        const picker = viewer.querySelector('[role="combobox"]')!;
        const footer = picker.closest("footer")!;
        const pickerBounds = picker.getBoundingClientRect();
        const footerBounds = footer.getBoundingClientRect();
        const path = footer.querySelector("button")!;
        const pathBounds = path.getBoundingClientRect();
        const tabs = content.querySelector('[role="tablist"]')!;
        const entries = tabs.querySelectorAll(".file-tab");
        const first = entries[0].getBoundingClientRect();
        const second = entries[1].getBoundingClientRect();
        const tabBounds = tabs.getBoundingClientRect();
        return {
          radius: getComputedStyle(source).borderRadius,
          monacoRadius: getComputedStyle(monaco).borderRadius,
          guardRadius: getComputedStyle(
            monaco.querySelector(".overflow-guard")!,
          ).borderRadius,
          contentRadius: css.borderRadius,
          editorAreaRadius: getComputedStyle(
            monaco.closest(".code-editor")!.parentElement!,
          ).borderRadius,
          token: css.getPropertyValue("--ui-section-radius").trim(),
          padding: [
            outer.paddingTop,
            outer.paddingRight,
            outer.paddingBottom,
            outer.paddingLeft,
          ],
          inset: css.getPropertyValue("--ui-control-inset").trim(),
          viewerBackground: outer.backgroundColor,
          explorerBackground: getComputedStyle(explorer).backgroundColor,
          contentBackground: css.backgroundColor,
          tabBackground: getComputedStyle(content.firstElementChild!)
            .backgroundColor,
          tabListBackground: getComputedStyle(tabs).backgroundColor,
          tabEntryBackground: getComputedStyle(entries[0]).backgroundColor,
          inactiveTabBackground: getComputedStyle(entries[1]).backgroundColor,
          monacoBackground: getComputedStyle(monaco).backgroundColor,
          monacoLayerBackgrounds: [
            source,
            ...monaco.querySelectorAll(".monaco-editor-background, .margin"),
          ].map((node) => getComputedStyle(node).backgroundColor),
          editorAreaBackground: getComputedStyle(source.parentElement!)
            .backgroundColor,
          sourceBounds: source.getBoundingClientRect().toJSON(),
          pickerBackground: getComputedStyle(picker).backgroundColor,
          pathBackground: getComputedStyle(path).backgroundColor,
          pathOffset: pathBounds.left - source.getBoundingClientRect().left,
          pathHeight: pathBounds.height,
          pickerHeight: pickerBounds.height,
          pathPadding: getComputedStyle(path).padding,
          pickerPadding: getComputedStyle(picker).padding,
          pathRadius: getComputedStyle(path).borderRadius,
          pickerRadius: getComputedStyle(picker).borderRadius,
          pickerWidth: pickerBounds.width,
          pickerGaps: [
            pickerBounds.top - footerBounds.top,
            footerBounds.right - pickerBounds.right,
            footerBounds.bottom - pickerBounds.bottom,
          ],
          tabRadius: getComputedStyle(entries[0]).borderRadius,
          tabGaps: [
            first.left - tabBounds.left,
            first.top - tabBounds.top,
            tabBounds.bottom - first.bottom,
            second.left - first.right,
          ],
          children: content.children.length,
          tabsFirst:
            !!content.firstElementChild!.querySelector('[role="tablist"]'),
          editorLast: content.lastElementChild!.contains(monaco),
          contentBounds: content.getBoundingClientRect().toJSON(),
          viewerBounds: viewer.getBoundingClientRect().toJSON(),
        };
      });
    const editorRadius = `${surface.token} 0px 0px ${surface.token}`;
    expect(surface.radius).toBe(editorRadius);
    expect(surface.monacoRadius).toBe(editorRadius);
    expect(surface.guardRadius).toBe(editorRadius);
    expect(surface.contentRadius).toBe("0px");
    expect(surface.editorAreaRadius).toBe(editorRadius);
    await expect(editor).not.toContainText("Read only");
    await expect(
      editor.getByRole("combobox", {
        name: "Syntax highlighting",
        exact: true,
      }),
    ).toHaveText("Auto");
    expect(surface.padding).toEqual(Array(4).fill(surface.inset));
    expect(surface.viewerBackground).toBe(surface.explorerBackground);
    expect(surface.contentBackground).toBe("rgba(0, 0, 0, 0)");
    expect(surface.tabBackground).toBe("rgba(0, 0, 0, 0)");
    expect(surface.tabListBackground).toBe("rgba(0, 0, 0, 0)");
    expect(surface.monacoBackground).toBe("rgba(0, 0, 0, 0)");
    expect(
      surface.monacoLayerBackgrounds.every(
        (color) => color === "rgba(0, 0, 0, 0)",
      ),
    ).toBe(true);
    expect(surface.tabEntryBackground).toBe(surface.editorAreaBackground);
    expect(surface.inactiveTabBackground).toBe(surface.editorAreaBackground);
    expect(surface.pickerBackground).toBe(surface.editorAreaBackground);
    expect(surface.pickerWidth).toBe(80);
    expect(surface.pathBackground).toBe(surface.pickerBackground);
    expect(surface.pathOffset).toBe(parseFloat(surface.token));
    expect(surface.pathHeight).toBe(surface.pickerHeight);
    expect(surface.pathPadding).toBe(surface.pickerPadding);
    expect(surface.pathRadius).toBe(surface.pickerRadius);
    expect(surface.pickerGaps).toEqual(Array(3).fill(surface.pickerGaps[0]));
    expect(surface.tabRadius).not.toBe("0px");
    expect(surface.tabRadius).toBe(surface.pathRadius);
    expect(surface.tabGaps).toEqual([
      parseFloat(surface.token),
      ...Array(3).fill(parseFloat(surface.inset)),
    ]);
    expect(surface.children).toBe(2);
    expect(surface.tabsFirst).toBe(true);
    expect(surface.editorLast).toBe(true);
    expect(surface.contentBounds.x - surface.viewerBounds.x).toBe(
      parseFloat(surface.inset),
    );
    expect(surface.viewerBounds.right - surface.contentBounds.right).toBe(
      parseFloat(surface.inset),
    );
    expect(surface.contentBounds.y - surface.viewerBounds.y).toBe(
      parseFloat(surface.inset),
    );
    const screenshot = await page.screenshot({
      path: testInfo.outputPath("workspace-editor.png"),
    });
    const corner = await page.evaluate(
      async ({ screenshot, x, y, right }) => {
        const image = new Image();
        image.src = `data:image/png;base64,${screenshot}`;
        await image.decode();
        const canvas = document.createElement("canvas");
        canvas.width = image.width;
        canvas.height = image.height;
        const context = canvas.getContext("2d")!;
        context.drawImage(image, 0, 0);
        return {
          left: [
            ...context.getImageData(Math.floor(x) + 1, Math.floor(y) + 1, 1, 1)
              .data,
          ].slice(0, 3),
          right: [
            ...context.getImageData(
              Math.floor(right) - 2,
              Math.floor(y) + 1,
              1,
              1,
            ).data,
          ].slice(0, 3),
        };
      },
      {
        screenshot: screenshot.toString("base64"),
        x: surface.sourceBounds.x,
        y: surface.sourceBounds.y,
        right: surface.sourceBounds.right,
      },
    );
    expect(corner.left).toEqual(
      surface.viewerBackground.match(/\d+/g)!.slice(0, 3).map(Number),
    );
    expect(corner.right).toEqual(
      surface.editorAreaBackground.match(/\d+/g)!.slice(0, 3).map(Number),
    );
    expect(surface.editorAreaBackground).not.toBe(surface.viewerBackground);
    await (await menu(page))
      .getByRole("menuitem", { name: "Close all tabs", exact: true })
      .click();
    await expect(editor).toContainText("Select a file to preview.");
    const emptyArea = editor.locator("[data-workspace-content] > :last-child");
    await expect(emptyArea).toHaveCSS(
      "background-color",
      surface.editorAreaBackground,
    );
    await expect(emptyArea).toHaveCSS("border-radius", editorRadius);
    await page.screenshot({ path: testInfo.outputPath("empty-editor.png") });
    const divider = editor.getByRole("separator", {
      name: "Resize file explorer",
      exact: true,
    });
    await page.mouse.move(0, 0);
    await expect
      .poll(() =>
        divider.evaluate((node) => getComputedStyle(node, "::before").opacity),
      )
      .toBe("0");
    const handleBounds = (await divider.boundingBox())!;
    const workbenchBounds = (await editor
      .locator(".file-workbench")
      .boundingBox())!;
    expect(handleBounds.height).toBeCloseTo(workbenchBounds.height / 2, 1);
    expect(handleBounds.y - workbenchBounds.y).toBeCloseTo(
      workbenchBounds.height / 4,
      1,
    );
    await divider.hover();
    await expect
      .poll(() =>
        divider.evaluate((node) => getComputedStyle(node, "::before").opacity),
      )
      .toBe("1");
    const explorer = editor.getByRole("navigation", {
      name: "Workspace files",
    });
    const before = (await explorer.boundingBox())!.width;
    await page.mouse.down();
    await page.mouse.move(
      handleBounds.x + 24,
      handleBounds.y + handleBounds.height / 2,
    );
    await page.mouse.up();
    await expect
      .poll(async () => (await explorer.boundingBox())!.width)
      .toBeGreaterThan(before);
    await divider.focus();
    await divider.press("ArrowRight");
    await expect
      .poll(async () => (await explorer.boundingBox())!.width)
      .toBeGreaterThan(before);
    await (await menu(page))
      .getByRole("menuitem", { name: "Connect", exact: true })
      .click();
    await expect(editor).toContainText("Simulated");
    await (await menu(page))
      .getByRole("menuitem", { name: "Disconnect", exact: true })
      .click();
    await expect(editor).not.toContainText("Simulated");
  });

test("the file menu stays on the right while overflowing tabs scroll", async ({
  page,
}) => {
  await page.setViewportSize({ width: 600, height: 800 });
  const editor = await open(page, "overflowing-tabs");
  const trigger = editor.getByRole("button", {
    name: "File actions",
    exact: true,
  });
  const before = (await trigger.boundingBox())!;
  const tabs = editor.getByRole("tablist", { name: "Open files" });
  expect(
    await tabs.evaluate((node) => node.scrollWidth > node.clientWidth),
  ).toBe(true);
  await tabs.evaluate((node) => {
    node.scrollLeft = node.scrollWidth;
  });
  expect(await trigger.boundingBox()).toEqual(before);
  await editor.getByRole("tab").last().click();
  await expect(editor.getByRole("tab").last()).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await (await menu(page))
    .getByRole("menuitem", { name: "Close other tabs", exact: true })
    .click();
  await expect(editor.getByRole("tab")).toHaveCount(1);
});

test("syntax menu keeps its scrollbar outside the left edge and supports drag and keyboard scrolling", async ({
  page,
}, testInfo) => {
  const editor = await open(page);
  const picker = editor.getByRole("combobox", {
    name: "Syntax highlighting",
    exact: true,
  });
  await expect(editor.locator(".monaco-editor")).toBeVisible();
  await picker.click();
  const menu = page.getByRole("group", {
    name: "Syntax highlighting choices",
    exact: true,
  });
  const list = menu.getByRole("listbox");
  await menu.evaluate(async (node) => {
    await Promise.allSettled(
      node
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished),
    );
  });
  const geometry = await menu.evaluate((node) => {
    const input = node.querySelector<HTMLInputElement>(".setting-search")!;
    const trigger = document.querySelector<HTMLElement>(
      '.workspace-editor [role="combobox"]',
    )!;
    const row = input.parentElement!;
    return {
      trigger: trigger.getBoundingClientRect().toJSON(),
      row: row.getBoundingClientRect().toJSON(),
      input: input.getBoundingClientRect().toJSON(),
      radius: getComputedStyle(input).borderRadius,
      padding: [
        getComputedStyle(row).paddingLeft,
        getComputedStyle(row).paddingRight,
      ],
    };
  });
  await testInfo.attach("search-input-geometry", {
    body: JSON.stringify(geometry, null, 2),
    contentType: "application/json",
  });
  expect(geometry.row.width).toBe(geometry.trigger.width);
  expect(geometry.row.height).toBe(geometry.trigger.height);
  expect(geometry.row.x).toBeCloseTo(geometry.trigger.x, 1);
  expect(geometry.row.y).toBeCloseTo(geometry.trigger.y, 1);
  expect(geometry.input.width).toBe(
    geometry.row.width -
      geometry.padding.reduce((sum, value) => sum + parseFloat(value), 0),
  );
  expect(geometry.radius).toBe("0px");
  expect((await menu.boundingBox())!.width).toBeLessThanOrEqual(
    (await picker.boundingBox())!.width + 8,
  );
  await expect(list).toHaveAttribute("data-scrollbar-side", "left");
  await expect(list).toHaveAttribute("data-scrollbar-offset", "-8");
  const viewport = list.locator('[data-scroll-area="viewport"]');
  const rail = list.locator('[data-scroll-area="rail"]');
  const thumb = list.getByRole("scrollbar");
  const handle = list.locator('[data-scroll-area="handle"]');
  await expect(thumb).toBeVisible();
  await handle.hover();
  await expect(handle).toHaveCSS("opacity", "1");
  await expect
    .poll(async () => {
      const surface = (await menu.boundingBox())!;
      const bar = (await handle.boundingBox())!;
      return surface.x - bar.x - bar.width;
    })
    .toBeGreaterThanOrEqual(1);
  await page.screenshot({ path: testInfo.outputPath("syntax-menu.png") });
  const bar = (await thumb.boundingBox())!;
  const track = (await rail.boundingBox())!;
  await page.mouse.move(bar.x + bar.width / 2, bar.y + bar.height / 2);
  await page.mouse.down();
  await page.mouse.move(bar.x + bar.width / 2, track.y + track.height);
  await page.mouse.up();
  await expect
    .poll(() =>
      viewport.evaluate(
        (node) => node.scrollHeight - node.clientHeight - node.scrollTop,
      ),
    )
    .toBeLessThan(1);
  await expect(menu).toBeVisible();
  await thumb.press("Home");
  await expect.poll(() => viewport.evaluate((node) => node.scrollTop)).toBe(0);
  await thumb.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(picker).toBeFocused();
});

test("automatic syntax detection colors Python, Dockerfile and Protobuf and a file override keeps the editor", async ({
  page,
}) => {
  const editor = await open(page, "syntax-highlighting");
  const picker = editor.getByRole("combobox", {
    name: "Syntax highlighting",
    exact: true,
  });
  const colors = () =>
    editor
      .locator(".view-lines")
      .evaluate(
        (node) =>
          new Set(
            [...node.querySelectorAll("span")]
              .filter((span) => span.textContent?.trim())
              .map((span) => getComputedStyle(span).color),
          ).size,
      );
  await expect(picker).toHaveText("Auto");
  for (const name of ["script.py", "Dockerfile", "api.proto"]) {
    await editor.getByRole("tab", { name, exact: true }).click();
    await expect
      .poll(colors, {
        message: `${name} should be tokenized and painted in multiple syntax colors`,
      })
      .toBeGreaterThan(1);
  }
  await editor.getByRole("tab", { name: "scratch", exact: true }).click();
  await expect.poll(colors).toBe(1);
  const element = await editor.locator(".monaco-editor").elementHandle();
  await picker.click();
  await page.getByRole("option", { name: "TypeScript", exact: true }).click();
  await expect.poll(colors).toBeGreaterThan(1);
  expect(
    await editor
      .locator(".monaco-editor")
      .evaluate((node, original) => node === original, element),
  ).toBe(true);
  await expect(editor.locator(".view-lines")).toContainText(
    "export const answer = 42",
  );
  await editor.getByRole("tab", { name: "script.py", exact: true }).click();
  await expect(picker).toHaveText("Auto");
  await editor.getByRole("tab", { name: "scratch", exact: true }).click();
  await expect(picker).toHaveText("TypeScript");
  await picker.click();
  await page.getByRole("option", { name: "Auto", exact: true }).click();
  await expect.poll(colors).toBe(1);
  await picker.click();
  await page.getByRole("option", { name: /plain text/i }).click();
  await expect(picker).toHaveText(/plain text/i);
  await expect.poll(colors).toBe(1);
});

test("Markdown preview toggles per file, shares the rendered surface and disables unsupported files", async ({
  page,
}, testInfo) => {
  const editor = await open(page, "rendered-markdown");
  const toggle = editor.getByRole("button", { name: "Preview", exact: true });
  await expect(toggle).toBeEnabled();
  await expect(toggle).toHaveAttribute("aria-pressed", "true");
  await toggle.click();
  await expect(editor.locator(".monaco-editor")).toBeVisible();
  const bounds = (await toggle.boundingBox())!;
  const more = (await editor
    .getByRole("button", { name: "File actions", exact: true })
    .boundingBox())!;
  expect(bounds.x + bounds.width).toBeLessThan(more.x);
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-pressed", "true");
  const preview = editor.getByRole("region", {
    name: "Markdown preview: /workspace/README.md",
    exact: true,
  });
  await expect(
    preview.getByRole("heading", { name: "Workspace preview", exact: true }),
  ).toBeVisible();
  await expect(preview.getByRole("table")).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("markdown-preview.png") });
  await expect(preview.locator("[data-scroll-area=viewport]")).toHaveCount(1);
  await expect(editor.locator(".monaco-editor")).toHaveCount(0);
  await preview.getByRole("link", { name: "Open source", exact: true }).click();
  await expect(
    editor.getByRole("tab", { name: "main.ts", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(toggle).toBeDisabled();
  await expect(toggle).toHaveAttribute("aria-pressed", "false");
  await editor.getByRole("tab", { name: "README.md", exact: true }).click();
  await expect(preview).toBeVisible();
  await expect(toggle).toHaveAttribute("aria-pressed", "true");
  await toggle.click();
  await expect(preview).toHaveCount(0);
  await expect(editor.locator(".view-lines")).toContainText(
    "# Workspace preview",
  );
  await expect(editor.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    true,
  );
  await editor.getByRole("tab", { name: "package.json", exact: true }).click();
  await expect(toggle).toBeDisabled();
  await (await menu(page))
    .getByRole("menuitem", { name: "Close all tabs", exact: true })
    .click();
  await expect(toggle).toBeDisabled();
});

test("returning from rendered preview restores the source scroll position", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1000, height: 300 });
  const editor = await open(page);
  await expect(editor.locator(".view-lines")).toBeVisible();
  const firstVisibleLine = () =>
    editor.locator(".monaco-editor").evaluate((node) => {
      const bounds = node.getBoundingClientRect();
      return [...node.querySelectorAll(".view-line")].find((line) => {
        const box = line.getBoundingClientRect();
        return box.bottom > bounds.top && box.top < bounds.bottom;
      })?.textContent;
    });
  const first = await firstVisibleLine();
  await editor.locator(".view-lines").hover();
  await page.mouse.wheel(0, 240);
  await expect.poll(firstVisibleLine).not.toBe(first);
  const scrolled = await firstVisibleLine();
  const toggle = editor.getByRole("button", { name: "Preview", exact: true });
  await toggle.click();
  await expect(
    editor.getByRole("heading", { name: "Workspace preview", exact: true }),
  ).toBeVisible();
  await toggle.click();
  await expect.poll(firstVisibleLine).toBe(scrolled);
});

test("closing tabs cancels file loads so a late response cannot reopen them", async ({
  page,
}) => {
  const editor = await open(page);
  await editor.getByRole("button", { name: "slow.txt", exact: true }).click();
  await expect(editor.getByRole("status")).toHaveText("Loading…");
  await (await menu(page))
    .getByRole("menuitem", { name: "Close all tabs", exact: true })
    .click();
  await expect(editor.getByRole("tab")).toHaveCount(0);
  // The fixture intentionally ignores cancellation and responds after this delay.
  await page.waitForTimeout(1700);
  await expect(editor.getByRole("tab")).toHaveCount(0);
  await editor.getByRole("button", { name: "slow.txt", exact: true }).click();
  await expect(
    editor.getByRole("tab", { name: "slow.txt", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(editor.locator(".view-lines")).toContainText(
    "A delayed file preview.",
  );
});

test("narrow preview closes and reopens from the tab bar without losing tabs", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const editor = await open(page, "narrow-preview");
  await expect(editor.getByRole("tab")).toHaveCount(3);
  await editor
    .getByRole("button", { name: "Close file preview", exact: true })
    .click();
  await expect(editor).toBeHidden();
  await page
    .getByRole("button", { name: "Open file preview", exact: true })
    .click();
  await expect(editor).toBeVisible();
  await expect(editor.getByRole("tab")).toHaveCount(3);
  await (await menu(page))
    .getByRole("menuitem", { name: "Close other tabs", exact: true })
    .click();
  await expect(editor.getByRole("tab")).toHaveCount(1);
});
