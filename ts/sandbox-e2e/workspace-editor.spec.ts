import { test, expect, devices } from "@playwright/test";

test.use({
  userAgent: devices["Desktop Chrome"].userAgent,
  viewport: { width: 1843, height: 1000 },
  isMobile: false,
  hasTouch: false,
  deviceScaleFactor: 1,
});

test("workspace split, readonly files and simulated connections keep project state across resize", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.stack || error.message));
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await expect(
    page.getByRole("complementary", { name: "Workspace editor" }),
  ).toHaveCount(0);
  await page.setViewportSize({ width: 1844, height: 1000 });
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  await expect(editor).toBeVisible();
  expect(
    await page
      .locator(".conversation")
      .evaluate((el) => el.getBoundingClientRect().width),
  ).toBe(800);
  const divider = page.getByRole("separator", {
    name: "Resize workspace editor",
  });
  await expect(divider).toBeVisible();
  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  await composer.fill("Keep this draft");
  await editor.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(editor.locator(".monaco-editor")).toBeVisible({
    timeout: 30000,
  });
  await expect(editor.locator(".view-lines")).toContainText("project-1");
  await expect(editor.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    true,
  );
  await editor.getByRole("button", { name: "src" }).click();
  await editor.getByRole("button", { name: "main.go", exact: true }).click();
  await expect(
    editor.getByRole("tab", { name: "main.go", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(editor.locator(".view-lines")).toContainText("package main");
  // Resize mounted panes; keep the same Monaco instance, active file and draft.
  await editor.locator(".monaco-editor").evaluate((el) => {
    el.setAttribute("data-resize-marker", "keep");
  });
  const before = (await page.locator(".conversation").boundingBox())!.width;
  const dividerBox = (await divider.boundingBox())!;
  await page.mouse.move(
    dividerBox.x + dividerBox.width / 2,
    dividerBox.y + dividerBox.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    dividerBox.x + 160,
    dividerBox.y + dividerBox.height / 2,
    { steps: 6 },
  );
  await page.mouse.up();
  await expect
    .poll(
      async () => (await page.locator(".conversation").boundingBox())!.width,
    )
    .toBeGreaterThan(before + 150);
  await expect(composer).toHaveValue("Keep this draft");
  await expect(editor.locator(".monaco-editor")).toHaveAttribute(
    "data-resize-marker",
    "keep",
  );
  const explorer = editor.getByRole("navigation", { name: "Workspace files" });
  const explorerDivider = editor.getByRole("separator", {
    name: "Resize file explorer",
  });
  const beforeExplorer = (await explorer.boundingBox())!.width;
  const explorerBox = (await explorerDivider.boundingBox())!;
  await page.mouse.move(
    explorerBox.x + explorerBox.width / 2,
    explorerBox.y + explorerBox.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    explorerBox.x + 64,
    explorerBox.y + explorerBox.height / 2,
    { steps: 6 },
  );
  await page.mouse.up();
  await expect
    .poll(async () => (await explorer.boundingBox())!.width)
    .toBeGreaterThan(beforeExplorer + 60);
  await explorerDivider.focus();
  await page.keyboard.press("Home");
  await expect
    .poll(async () => (await explorer.boundingBox())!.width)
    .toBe(Number(await explorerDivider.getAttribute("aria-valuemin")));
  await divider.focus();
  await page.keyboard.press("End");
  await expect
    .poll(
      async () => (await page.locator(".conversation").boundingBox())!.width,
    )
    .toBe(Number(await divider.getAttribute("aria-valuemax")));
  await expect(editor.locator(".monaco-editor")).toHaveAttribute(
    "data-resize-marker",
    "keep",
  );
  await divider.dblclick();
  await expect
    .poll(
      async () => (await page.locator(".conversation").boundingBox())!.width,
    )
    .toBe(before);
  await editor.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(editor).toContainText("Simulated connection");
  await expect(
    editor.getByRole("button", { name: "Disconnect", exact: true }),
  ).toBeVisible();
  await expect(page.locator("iframe")).toHaveCount(0);
  await divider.focus();
  await page.keyboard.press("Shift+ArrowLeft");
  const chosenConversationWidth = (await page
    .locator(".conversation")
    .boundingBox())!.width;
  await page.setViewportSize({ width: 1440, height: 1000 });
  await expect(editor).toBeHidden();
  await expect(divider).toBeHidden();
  await expect(composer).toHaveValue("Keep this draft");
  await page.setViewportSize({ width: 2104, height: 1000 });
  await expect(editor).toBeVisible();
  await expect
    .poll(
      async () => (await page.locator(".conversation").boundingBox())!.width,
    )
    .toBe(chosenConversationWidth);
  await expect(editor).toContainText("Simulated connection");
  await expect(
    editor.getByRole("tab", { name: "main.go", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-8");
  await expect(editor).toContainText("Secondary project");
  await expect(
    editor.getByRole("button", { name: "Connect", exact: true }),
  ).toBeVisible();
  await editor.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText("project-2");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(editor).toContainText("Simulated connection");
  await expect(
    editor.getByRole("tab", { name: "main.go", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await editor.getByRole("button", { name: "Disconnect", exact: true }).click();
  await expect(editor).toContainText("File preview");
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    2104,
  );
  expect(errors).toEqual([]);
  await page.screenshot({
    path: "test-results/workspace-editor.png",
    fullPage: true,
  });
});
