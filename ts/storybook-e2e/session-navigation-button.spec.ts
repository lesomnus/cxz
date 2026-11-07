import { expect, test, type Locator } from "@playwright/test";

async function metrics(button: Locator) {
  return button.evaluate((node) => {
    const content = node.querySelector(".button-content")!;
    const logo = content.querySelector(".agent-brand")!;
    const identity = content.children[1];
    const title = identity.children[0];
    const alias = identity.children[1];
    return {
      height: node.getBoundingClientRect().height,
      gap:
        title.getBoundingClientRect().left - logo.getBoundingClientRect().right,
      logoSize: logo.getBoundingClientRect().height,
      nameFont: getComputedStyle(title).fontSize,
      aliasFont: getComputedStyle(alias).fontSize,
      aliasFamily: getComputedStyle(alias).fontFamily,
      padding: getComputedStyle(node).paddingInlineStart,
      radius: getComputedStyle(node).borderRadius,
    };
  });
}

test("session cards share logo spacing and identity styles across resource pages", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  const samples: Awaited<ReturnType<typeof metrics>>[] = [];
  const tableSamples: Awaited<ReturnType<typeof metrics>>[] = [];
  for (const [story, container] of [
    ["resources-tasks--task-details", '[aria-label="Task information"]'],
    ["resources-tasks--project-table", 'table[aria-label="Tasks"]'],
    ["pages-projects--list", 'table[aria-label="Projects"]'],
    ["pages-projects--default", 'table[aria-label="Sessions"]'],
    ["accounts-workflows--signed-in-account", 'table[aria-label="Sessions"]'],
  ]) {
    await page.goto(`/iframe.html?id=${story}&viewMode=story`);
    const buttons = page
      .locator(container)
      .locator("[data-session-navigation]:visible");
    await expect(buttons.first(), story).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    for (const button of await buttons.all()) {
      await expect(button.locator(".agent-brand")).toHaveCount(1);
      await expect(button.locator('[data-icon="open-external"]')).toHaveCount(
        1,
      );
      const sample = await metrics(button);
      samples.push(sample);
      if (container.startsWith("table")) tableSamples.push(sample);
    }
  }
  expect(samples.length).toBeGreaterThan(5);
  for (const sample of samples) {
    expect(sample.gap).toBeCloseTo(8, 2);
    expect(sample.height).toBeCloseTo(40, 2);
    expect(sample.logoSize).toBeCloseTo(14, 2);
    expect(sample.nameFont).toBe("14px");
    expect(sample.aliasFont).toBe("10px");
    expect(sample.aliasFamily).toBe(samples[0].aliasFamily);
  }
  // Form fields retain their shared input alignment/concentric inset geometry.
  // Standalone table cards share the complete box treatment as well.
  for (const sample of tableSamples) {
    expect(sample.padding).toBe(tableSamples[0].padding);
    expect(sample.radius).toBe(tableSamples[0].radius);
  }
});

test("shared card examples keep logos and alias while truncating names and retaining real links", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-sessionnavigationbutton--widths&viewMode=story",
  );
  const buttons = page.locator("[data-session-navigation]");
  await expect(buttons).toHaveCount(6);
  for (const button of await buttons.all()) {
    const text = button.locator(".button-content > span").nth(1);
    await expect(text.locator(":scope > span").first()).toHaveText(
      "Review shared project work",
    );
    expect((await metrics(button)).gap).toBeCloseTo(8, 2);
  }
  await expect(buttons.first()).toHaveAttribute("href", "/c/_/session/oak");
  await expect(buttons.nth(1)).toHaveAttribute("href", "/c/_/session/elm");
  expect(
    await buttons
      .first()
      .locator(".button-content > span")
      .nth(1)
      .locator(":scope > span")
      .first()
      .evaluate((node) => node.scrollWidth > node.clientWidth),
  ).toBe(true);
  await expect(buttons.last()).toHaveAttribute("title", /Previous run/);
});
