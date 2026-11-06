import { expect, test, type Locator, type Page } from "@playwright/test";

const resources = [
  {
    name: "Projects",
    story: "pages-projects--navigation",
    item: "Web workspace web",
    back: "button",
  },
  {
    name: "Tasks",
    story: "resources-tasks--project-table",
    item: "Open task: Connect the task table",
    back: "button",
  },
  {
    name: "Memory",
    story: "pages-memory--default",
    item: "Architecture notes",
    back: "link",
  },
] as const;

const frame = (page: Page) => page.locator("[data-resource-detail-pages]");
const slot = (root: Locator, value: string) =>
  root.locator(`:scope > [data-sliding-page="${value}"]`);
async function finish(root: Locator) {
  await root.evaluate((root) => {
    for (const page of root.children)
      for (const animation of page.getAnimations()) animation.finish();
  });
}
async function midpoint(root: Locator) {
  return root.evaluate((root) => {
    const pages = [...root.children] as HTMLElement[];
    const animations = pages.flatMap((page) => page.getAnimations());
    for (const animation of animations) {
      animation.pause();
      animation.currentTime = 250;
    }
    return {
      count: animations.length,
      width: root.clientWidth,
      offsets: pages.map(
        (page) => new DOMMatrixReadOnly(getComputedStyle(page).transform).m41,
      ),
    };
  });
}
async function leave(page: Page, resource: (typeof resources)[number]) {
  if (resource.back === "link")
    await page.getByRole("link", { name: "Memory", exact: true }).click();
  else await page.getByRole("button", { name: "Back", exact: true }).click();
}

for (const resource of resources) {
  test(`${resource.name} slides between list and details with the shared curve and retains outgoing content`, async ({
    page,
  }) => {
    await page.goto(`/iframe.html?id=${resource.story}&viewMode=story`);
    const root = frame(page);
    await expect(root).toHaveAttribute("data-value", "list");
    const listing = await slot(root, "list")
      .locator(":scope > *")
      .first()
      .elementHandle();
    await page.getByRole("link", { name: resource.item, exact: true }).click();
    await expect(root).toHaveAttribute("data-value", "detail");
    const policy = await root.evaluate((node) => ({
      duration: getComputedStyle(node)
        .getPropertyValue("--ui-sliding-pages-duration")
        .trim(),
      easing: getComputedStyle(node)
        .getPropertyValue("--ui-sliding-pages-easing")
        .trim(),
    }));
    expect(policy).toEqual({
      duration: "500ms",
      easing: "cubic-bezier(0.4, 0.85, 0, 0.95)",
    });
    const entering = await midpoint(root);
    expect(entering.count).toBeGreaterThan(0);
    expect(entering.offsets[0]).toBeLessThan(0);
    expect(entering.offsets[1]).toBeGreaterThan(0);
    expect(entering.offsets[1]).toBeLessThan(entering.width);
    await expect(slot(root, "list")).toHaveAttribute("inert", "");
    await finish(root);
    await leave(page, resource);
    await expect(root).toHaveAttribute("data-value", "list");
    expect(
      await slot(root, "detail").locator(":scope > *").count(),
    ).toBeGreaterThan(0);
    const leaving = await midpoint(root);
    expect(leaving.count).toBeGreaterThan(0);
    expect(leaving.offsets[0]).toBeLessThan(0);
    expect(leaving.offsets[1]).toBeGreaterThan(0);
    expect(leaving.offsets[1]).toBeLessThan(leaving.width);
    await expect(slot(root, "detail")).toHaveAttribute("inert", "");
    await finish(root);
    await expect(slot(root, "detail").locator(":scope > *")).toHaveCount(0);
    expect(await listing!.evaluate((node) => node.isConnected)).toBe(true);
    await expect(
      page.getByRole("link", { name: resource.item, exact: true }),
    ).toBeVisible();
  });

  test(`${resource.name} respects reduced motion for detail page navigation`, async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(`/iframe.html?id=${resource.story}&viewMode=story`);
    const root = frame(page);
    await page.getByRole("link", { name: resource.item, exact: true }).click();
    await expect(root).toHaveAttribute("data-value", "detail");
    expect(
      await root.evaluate(
        (root) =>
          [...root.children].flatMap((page) => page.getAnimations()).length,
      ),
    ).toBe(0);
    await leave(page, resource);
    await expect(root).toHaveAttribute("data-value", "list");
    await expect(slot(root, "detail").locator(":scope > *")).toHaveCount(0);
  });
}

test("returning to a Memory document during its exit does not restore discarded edits", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-memory--default&viewMode=story");
  await page
    .getByRole("link", { name: "Architecture notes", exact: true })
    .click();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await page
    .getByRole("textbox", { name: "Memory name", exact: true })
    .fill("Discard this draft");
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  await page
    .getByRole("dialog", { name: "Discard unsaved changes", exact: true })
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(frame(page)).toHaveAttribute("data-value", "list");
  await page
    .getByRole("link", { name: "Architecture notes", exact: true })
    .evaluate((node) => (node as HTMLAnchorElement).click());
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Memory name", exact: true }),
  ).toHaveValue("Architecture notes");
});
