import { expect, test } from "@playwright/test";

for (const width of [1440, 390]) {
  test(`project paths preserve their leaf, expand and copy at ${width}px`, async ({
    page,
    context,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/iframe.html?id=pages-projects--long-paths&viewMode=story",
    );
    const info = page.getByRole("region", { name: "Information", exact: true });
    const field = (label: string) =>
      info.locator("dl > div").filter({
        has: page.locator("dt > span:first-child", {
          hasText: new RegExp(`^${label}$`),
        }),
      });
    const workspace = field("Workspace"),
      configuration = field("Configuration");
    const copy = workspace.getByRole("button", {
      name: "Copy: Workspace",
      exact: true,
    });
    const display = copy.locator(":scope > span > span").first();
    await expect(display.locator("svg")).toHaveCount(1);
    await expect(display).toContainText("/host/long-parent-directory/");
    const leaf = copy.getByText("workspace-leaf", { exact: true });
    expect(
      await leaf.evaluate((node) => node.scrollWidth <= node.clientWidth),
    ).toBe(true);
    const configDisplay = configuration
      .getByRole("button")
      .locator(":scope > span > span")
      .first();
    await expect(configDisplay.locator("svg")).toHaveCount(1);
    await expect(configDisplay).toContainText(".devcontainer/very-long-leaf-");
    expect((await configDisplay.textContent())!.length).toBeLessThan(100);
    const bounds = (await workspace.boundingBox())!;
    const configBounds = (await configuration.boundingBox())!;
    expect(configBounds.x).toBe(bounds.x);
    expect(configBounds.width).toBe(bounds.width);
    expect(configBounds.y).toBeGreaterThanOrEqual(bounds.y + bounds.height);
    expect(bounds.width).toBe((await info.locator("dl").boundingBox())!.width);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);

    const complete =
      "/host/" + "long-parent-directory/".repeat(10) + "workspace-leaf";
    await copy.hover();
    const tooltip = page.getByRole("tooltip", { name: complete, exact: true });
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toHaveAttribute("data-placement", "in-place");
    const origin = await tooltip.evaluate((node) => {
      const range = document.createRange();
      range.setStart(node.firstChild!, 0);
      range.setEnd(node.firstChild!, 1);
      return range.getBoundingClientRect().left;
    });
    expect(Math.abs(origin - (await copy.boundingBox())!.x)).toBeLessThan(1);
    await page.keyboard.press("Escape");
    await expect(tooltip).toHaveCount(0);
    await copy.focus();
    await expect(tooltip).toBeVisible();
    await copy.press("Enter");
    await expect(copy).toBeFocused();
    await expect(workspace.getByRole("status")).toHaveText("copied!");
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      complete,
    );
    await expect(configuration.getByRole("status")).toBeEmpty();
    await expect(workspace.getByRole("status")).toBeEmpty();
  });
}

test("path copy feedback resets its timeout and reports clipboard failure", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.clock.install();
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const field = page
    .getByRole("region", { name: "Information", exact: true })
    .locator("dl > div")
    .filter({
      has: page.getByRole("button", { name: "Copy: Workspace", exact: true }),
    });
  const copy = field.getByRole("button");
  await copy.click();
  await expect(field.getByRole("status")).toHaveText("copied!");
  await page.clock.fastForward(1500);
  await copy.click();
  await page.clock.fastForward(1000);
  await expect(field.getByRole("status")).toHaveText("copied!");
  await page.clock.fastForward(1100);
  await expect(field.getByRole("status")).toBeEmpty();
  await page.evaluate(() =>
    Object.defineProperty(navigator.clipboard, "writeText", {
      configurable: true,
      value: async () => {
        throw new Error("Clipboard unavailable");
      },
    }),
  );
  await copy.click();
  await expect(field.getByRole("status")).toHaveText("Copy failed");
});

test("middle elision preserves leading entries across resizing and uses centered one-character marks", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/iframe.html?id=pages-projects--home-paths&viewMode=story");
  const info = page.getByRole("region", { name: "Information", exact: true });
  const copy = info.getByRole("button", {
    name: "Copy: Workspace",
    exact: true,
  });
  const display = copy.locator(":scope > span > span").first();
  await expect(display).toContainText("/home/somnus/workspaces/");
  await expect(display).toContainText("devcontainer.json");
  await expect(display.locator("svg")).toHaveCount(1);
  // A smaller container drops more middle entries before touching the leaf.
  await copy.evaluate((node) => {
    node.style.width = "270px";
  });
  await expect(display).toHaveText(
    "/home/somnus/workspaces//devcontainer.json",
  );
  await copy.evaluate((node) => {
    node.style.width = "125px";
  });
  await expect(display.locator("svg")).toHaveCount(2);
  await expect(display).toContainText("/home/somnus/");
  await expect(display).not.toContainText("devcontainer.json");
  const size = await display.evaluate((node) => {
    const mark = node.querySelector("svg")!;
    const box = mark.getBoundingClientRect();
    const range = document.createRange();
    const prefix = node.firstChild!.firstChild!;
    range.setStart(prefix, 0);
    range.setEnd(prefix, 1);
    const glyph = range.getBoundingClientRect();
    const line = node.getBoundingClientRect();
    return {
      markWidth: box.width,
      characterWidth: glyph.width,
      center: box.y + box.height / 2,
      lineCenter: line.y + line.height / 2,
      scroll: node.scrollWidth,
      width: node.clientWidth,
    };
  });
  expect(Math.abs(size.markWidth - size.characterWidth)).toBeLessThan(1);
  expect(Math.abs(size.center - size.lineCenter)).toBeLessThan(1);
  expect(size.scroll).toBeLessThanOrEqual(size.width);
  // Widening restores the entire original path instead of leaving stale elision.
  await copy.evaluate((node) => {
    node.style.width = "1000px";
    node.style.flexShrink = "0";
  });
  await expect(display.locator("svg")).toHaveCount(0);
  await expect(display).toHaveText(
    "/home/somnus/workspaces/github.com/lesomnus/cxz/.devcontainer/devcontainer.json",
  );
});
