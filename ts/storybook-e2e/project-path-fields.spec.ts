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
    const workspace = field("Workspace");
    const copy = workspace.getByRole("button", {
      name: "Copy: Workspace",
      exact: true,
    });
    const display = copy.locator(":scope > span > span").first();
    const leafFits = await copy.evaluate((button) => {
      const measure = button.querySelector(
        ':scope > span > span[aria-hidden="true"]',
      )!;
      const text = measure.querySelector("span")!.firstChild!;
      const value = text.textContent!;
      const range = document.createRange();
      const width = (part: string) => {
        const start = value.indexOf(part);
        range.setStart(text, start);
        range.setEnd(text, start + part.length);
        return range.getBoundingClientRect().width;
      };
      return (
        width("/host/long-parent-directory/") +
          measure.querySelector("svg")!.getBoundingClientRect().width +
          width("/") +
          width("workspace-leaf") <=
        button.clientWidth
      );
    });
    await expect(display.locator("svg")).toHaveCount(leafFits ? 1 : 2);
    await expect(display).toContainText("/host/long-parent-directory/");
    const leaf = display.locator(":scope > span").last();
    if (leafFits) await expect(leaf).toHaveText("workspace-leaf");
    else {
      const text = await leaf.textContent();
      expect(text).toBeTruthy();
      expect("workspace-leaf".startsWith(text!)).toBe(true);
      expect(text!.length).toBeLessThan("workspace-leaf".length);
    }
    expect(
      await leaf.evaluate((node) => node.scrollWidth <= node.clientWidth),
    ).toBe(true);
    const bounds = (await workspace.boundingBox())!;
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
    await expect(field("Configuration")).toHaveCount(0);
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
  await expect(copy).not.toHaveAttribute("aria-busy", "true");
  await page.clock.fastForward(1500);
  await copy.click();
  await expect(copy).not.toHaveAttribute("aria-busy", "true");
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
    const measure = node.querySelector(
      ':scope > span > span[aria-hidden="true"]',
    )!;
    const text = measure.querySelector("span")!.firstChild!;
    const value = text.textContent!;
    const range = document.createRange();
    const width = (part: string) => {
      const start = value.indexOf(part);
      range.setStart(text, start);
      range.setEnd(text, start + part.length);
      return range.getBoundingClientRect().width;
    };
    const mark = measure.querySelector("svg")!.getBoundingClientRect().width;
    node.style.width = `${Math.ceil(width("/home/somnus/workspaces/") + mark + width("/") + width("devcontainer.json"))}px`;
  });
  await expect(display).toHaveText(
    "/home/somnus/workspaces//devcontainer.json",
  );
  await copy.evaluate((node) => {
    const measure = node.querySelector(
      ':scope > span > span[aria-hidden="true"]',
    )!;
    const text = measure.querySelector("span")!.firstChild!;
    const value = text.textContent!;
    const range = document.createRange();
    const width = (part: string) => {
      const start = value.indexOf(part);
      range.setStart(text, start);
      range.setEnd(text, start + part.length);
      return range.getBoundingClientRect().width;
    };
    const mark = measure.querySelector("svg")!.getBoundingClientRect().width;
    node.style.width = `${Math.ceil(width("/home/somnus/") + 2 * mark + width("/") + width("de"))}px`;
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
