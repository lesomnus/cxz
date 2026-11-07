import { expect, test, type Page } from "@playwright/test";

async function menu(page: Page) {
  await page.getByRole("button", { name: "File actions", exact: true }).click();
  return page.getByRole("menu", { name: "File actions", exact: true });
}
async function open(page: Page, story = "tabs") {
  await page.goto(
    `/iframe.html?id=workspace-file-editor--${story}&viewMode=story`,
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

test("file editor uses an inset section surface without a header and keeps resizing available", async ({
  page,
}, testInfo) => {
  const editor = await open(page);
  await expect(editor.locator(":scope > header")).toHaveCount(0);
  await expect(editor.locator(".monaco-editor")).toBeVisible();
  const surface = await editor.locator(".code-editor").evaluate((node) => {
    const parent = node.parentElement!;
    const css = getComputedStyle(parent);
    return {
      radius: css.borderRadius,
      token: css.getPropertyValue("--ui-section-radius").trim(),
      margins: [
        css.marginTop,
        css.marginRight,
        css.marginBottom,
        css.marginLeft,
      ],
      inset: css.getPropertyValue("--ui-control-inset").trim(),
    };
  });
  expect(surface.radius).toBe(surface.token);
  expect(surface.margins).toEqual(Array(4).fill(surface.inset));
  await page.screenshot({ path: testInfo.outputPath("workspace-editor.png") });
  const divider = editor.getByRole("separator", {
    name: "Resize file explorer",
    exact: true,
  });
  await page.mouse.move(0, 0);
  await expect
    .poll(() =>
      divider.evaluate(
        (node) => getComputedStyle(node, "::before").backgroundColor,
      ),
    )
    .toBe("rgba(0, 0, 0, 0)");
  const explorer = editor.getByRole("navigation", { name: "Workspace files" });
  const before = (await explorer.boundingBox())!.width;
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
