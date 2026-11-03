import { expect, test, type Locator } from "@playwright/test";

const rect = async (node: Locator) => (await node.boundingBox())!;
async function reachesPane(block: Locator, surface: Locator) {
  await expect
    .poll(async () =>
      surface.evaluate((el) => {
        const frame = el
          .closest("[data-code-expand-boundary]")!
          .getBoundingClientRect();
        const box = el.getBoundingClientRect();
        return Math.max(
          Math.abs(box.left - frame.left),
          Math.abs(box.right - frame.right),
        );
      }),
    )
    .toBeLessThan(1);
  // Bounding boxes alone do not prove the virtualized column painted the overhang.
  expect(
    await surface.evaluate((el) => {
      const box = el.getBoundingClientRect();
      const y = Math.max(box.top + 4, 4);
      return [box.left + 4, box.right - 4].every((x) =>
        el.contains(document.elementFromPoint(x, y)),
      );
    }),
  ).toBe(true);
  await expect(
    block.getByRole("button", {
      name: "Restore code block width",
      exact: true,
    }),
  ).toHaveAttribute("aria-pressed", "true");
}

for (const nested of [false, true]) {
  test(`code expansion preserves toolbar and text${nested ? " inside a list" : ""}`, async ({
    page,
    context,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.setViewportSize({ width: 1280, height: 1000 });
    await page.goto(
      "/iframe.html?id=conversation-eventcards--wide-code-block&viewMode=story",
    );
    const pre = page.locator(".transcript pre").nth(nested ? 1 : 0);
    const block = pre.locator("..");
    await pre.scrollIntoViewIfNeeded();
    const text = await pre.textContent();
    await block.hover();
    const expand = block.getByRole("button", {
      name: "Expand code block",
      exact: true,
    });
    const copy = block.getByRole("button", { name: "Copy", exact: true });
    const toolbar = copy.locator("../..");
    const original = await rect(pre),
      before = await rect(toolbar);
    expect((await rect(expand)).x).toBeLessThan((await rect(copy)).x);
    await expand.click();
    await reachesPane(block, pre);
    const after = await rect(toolbar);
    expect(after.x).toBeCloseTo(before.x, 1);
    expect(after.y).toBeCloseTo(before.y, 1);
    expect(after.width).toBeCloseTo(before.width, 1);
    expect((await rect(pre)).width).toBeGreaterThan(original.width + 200);
    await copy.click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      text,
    );
    await block
      .getByRole("button", { name: "Restore code block width", exact: true })
      .click();
    expect((await rect(pre)).width).toBeCloseTo(original.width, 1);
    expect((await rect(pre)).x).toBeCloseTo(original.x, 1);
    await expect(expand).toHaveAttribute("aria-pressed", "false");
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(1280);
  });
}

test("expanded code follows viewport changes without widening the conversation text column", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 1000 });
  await page.goto(
    "/iframe.html?id=conversation-eventcards--wide-code-block&viewMode=story",
  );
  const pre = page.locator(".transcript pre").first(),
    block = pre.locator("..");
  await block.hover();
  const expand = block.getByRole("button", {
    name: "Expand code block",
    exact: true,
  });
  await expand.focus();
  await page.keyboard.press("Enter");
  for (const width of [1000, 760, 390, 1400]) {
    await page.setViewportSize({ width, height: 1000 });
    await reachesPane(block, pre);
    expect(
      (await rect(page.locator(".transcript-content"))).width,
    ).toBeLessThanOrEqual(600);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
    const toolbar = await rect(
      block
        .getByRole("button", { name: "Restore code block width", exact: true })
        .locator(".."),
    );
    const anchor = await rect(block);
    expect(toolbar.x + toolbar.width).toBeCloseTo(anchor.x + anchor.width, 1);
  }
  await block
    .getByRole("button", { name: "Restore code block width", exact: true })
    .focus();
  await page.keyboard.press("Space");
  await expect(expand).toHaveAttribute("aria-pressed", "false");
});

test("Mermaid expansion applies to both the diagram and its source", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 1000 });
  await page.goto(
    "/iframe.html?id=conversation-eventcards--mermaid-response&viewMode=story",
  );
  const image = page.getByRole("img", {
    name: "Conversation routing",
    exact: true,
  });
  await expect(image).toBeVisible();
  const block = image.locator(
    "xpath=ancestor::div[@data-code-expanded or ./*[self::pre]][1]",
  );
  await block.hover();
  const expand = block.getByRole("button", {
    name: "Expand code block",
    exact: true,
  });
  const toolbar = block
    .getByRole("button", { name: "Copy", exact: true })
    .locator("../..");
  const before = await rect(toolbar);
  await expand.click();
  await reachesPane(
    block,
    block.locator(":scope > div").filter({ has: image }),
  );
  expect((await rect(toolbar)).x).toBeCloseTo(before.x, 1);
  await block.getByRole("button", { name: "View source", exact: true }).click();
  await expect(block.locator("pre")).toBeVisible();
  await reachesPane(block, block.locator("pre"));
  await block
    .getByRole("button", { name: "Restore code block width", exact: true })
    .click();
  await expect(expand).toHaveAttribute("aria-pressed", "false");
});
