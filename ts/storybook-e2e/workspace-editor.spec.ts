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
  await editor.getByRole("tab", { name: "package.json", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText(
    '"workspace-preview"',
  );
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
          monacoBackground: getComputedStyle(monaco).backgroundColor,
          editorAreaBackground: getComputedStyle(source.parentElement!)
            .backgroundColor,
          sourceBounds: source.getBoundingClientRect().toJSON(),
          pickerBackground: getComputedStyle(picker).backgroundColor,
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
    expect(surface.radius).toBe(surface.token);
    expect(surface.monacoRadius).toBe(surface.token);
    expect(surface.guardRadius).toBe(surface.token);
    expect(surface.contentRadius).toBe("0px");
    expect(surface.editorAreaRadius).toBe("0px");
    await expect(editor).not.toContainText("Read only");
    await expect(
      editor.getByRole("combobox", {
        name: "Syntax highlighting",
        exact: true,
      }),
    ).toHaveText("Auto");
    expect(surface.padding).toEqual(Array(4).fill(surface.inset));
    expect(surface.viewerBackground).toBe(surface.explorerBackground);
    expect(surface.contentBackground).toBe(surface.monacoBackground);
    expect(surface.tabBackground).toBe(surface.monacoBackground);
    expect(surface.pickerBackground).toBe(surface.monacoBackground);
    expect(surface.pickerGaps).toEqual(Array(3).fill(surface.pickerGaps[0]));
    expect(surface.tabRadius).not.toBe("0px");
    expect(surface.tabGaps).toEqual(Array(4).fill(parseFloat(surface.inset)));
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
      async ({ screenshot, x, y }) => {
        const image = new Image();
        image.src = `data:image/png;base64,${screenshot}`;
        await image.decode();
        const canvas = document.createElement("canvas");
        canvas.width = image.width;
        canvas.height = image.height;
        const context = canvas.getContext("2d")!;
        context.drawImage(image, 0, 0);
        return [
          ...context.getImageData(Math.floor(x) + 1, Math.floor(y) + 1, 1, 1)
            .data,
        ].slice(0, 3);
      },
      {
        screenshot: screenshot.toString("base64"),
        x: surface.sourceBounds.x,
        y: surface.sourceBounds.y,
      },
    );
    expect(corner).toEqual(
      surface.editorAreaBackground.match(/\d+/g)!.slice(0, 3).map(Number),
    );
    expect(surface.editorAreaBackground).not.toBe(surface.monacoBackground);
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
