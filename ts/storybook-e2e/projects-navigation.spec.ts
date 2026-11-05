import { expect, test, type Locator } from "@playwright/test";

const navigationStory =
  "/iframe.html?id=pages-projects--navigation&viewMode=story";
const offset = (node: Locator) =>
  node.evaluate(
    (element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).m41,
  );

test("project details slide in from the right below a stable toolbar and reverse on Back", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(navigationStory);
  const main = page.getByRole("main", { name: "Project workspace" });
  const root = main.locator(":scope > [data-sliding-pages]");
  const list = root.locator(':scope > [data-sliding-page="list"]');
  const detail = root.locator(':scope > [data-sliding-page="detail"]');
  const toolbar = main.locator(":scope > header");
  await expect(root).toHaveAttribute("data-value", "list");
  const before = (await root.boundingBox())!;
  const header = await toolbar.boundingBox();
  await root.evaluate((element) =>
    (element as HTMLElement).style.setProperty(
      "--ui-sliding-pages-duration",
      "1000ms",
    ),
  );
  await list
    .getByRole("link", { name: "Web workspace web", exact: true })
    .click();
  await expect(root).toHaveAttribute("data-value", "detail");
  await expect.poll(() => offset(list)).toBeLessThan(-1);
  expect(await offset(list)).toBeGreaterThan(-before.width);
  const entering = await offset(detail);
  expect(entering).toBeGreaterThan(0);
  expect(entering).toBeLessThan(before.width);
  await expect(list).toHaveAttribute("inert", "");
  await expect(detail).toBeFocused();
  await expect.poll(() => offset(detail)).toBe(0);
  expect(await root.boundingBox()).toEqual(before);
  expect(await toolbar.boundingBox()).toEqual(header);
  await expect(
    detail.getByRole("region", { name: "Information", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("navigation", { name: "Preview history" })
    .getByRole("button", { name: "Back", exact: true })
    .click();
  await expect(root).toHaveAttribute("data-value", "list");
  await expect.poll(() => offset(detail)).toBeGreaterThan(1);
  expect(await offset(list)).toBeLessThan(0);
  await expect(
    detail.getByRole("region", {
      name: "Information",
      exact: true,
      includeHidden: true,
    }),
  ).toBeAttached();
  await expect(detail).toHaveAttribute("inert", "");
  await expect.poll(() => offset(list)).toBe(0);
  expect(await root.boundingBox()).toEqual(before);
  expect(await toolbar.boundingBox()).toEqual(header);
  expect(errors).toEqual([]);
});

test("project page navigation retains each scroll position and index selection without document overflow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 650 });
  await page.goto(navigationStory);
  const root = page.locator("main > [data-sliding-pages]");
  const list = root.locator(':scope > [data-sliding-page="list"]');
  const detail = root.locator(':scope > [data-sliding-page="detail"]');
  const listScroll = list.locator(".panel-scroll-content");
  const history = page.getByRole("navigation", { name: "Preview history" });
  const selected = list.getByRole("checkbox", {
    name: "Select Web workspace",
    exact: true,
  });
  await selected.check();
  await listScroll.evaluate((element) => (element.scrollLeft = 32));
  await expect
    .poll(() => listScroll.evaluate((element) => element.scrollLeft))
    .toBe(32);
  await list
    .getByRole("link", { name: "Web workspace web", exact: true })
    .click();
  await expect(root).toHaveAttribute("data-value", "detail");
  await expect.poll(() => offset(detail)).toBe(0);
  const detailScroll = detail.locator(
    ":scope > .panel-scroll-region > .panel-scroll-content",
  );
  await detailScroll.evaluate((element) => (element.scrollTop = 180));
  await expect
    .poll(() => detailScroll.evaluate((element) => element.scrollTop))
    .toBe(180);
  await history.getByRole("button", { name: "Back", exact: true }).click();
  await expect(root).toHaveAttribute("data-value", "list");
  await expect.poll(() => offset(list)).toBe(0);
  await expect(selected).toBeChecked();
  expect(await listScroll.evaluate((element) => element.scrollLeft)).toBe(32);
  await history.getByRole("button", { name: "Forward", exact: true }).click();
  await expect(root).toHaveAttribute("data-value", "detail");
  await expect.poll(() => offset(detail)).toBe(0);
  expect(await detailScroll.evaluate((element) => element.scrollTop)).toBe(180);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollHeight <= innerHeight,
    ),
  ).toBe(true);
});

test("direct project links start in place and reduced motion disables route sliding", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const root = page.locator("main > [data-sliding-pages]");
  const detail = root.locator(':scope > [data-sliding-page="detail"]');
  await expect(root).toHaveAttribute("data-value", "detail");
  await expect(
    detail.getByRole("table", { name: "Sessions", exact: true }),
  ).toBeVisible();
  expect(await offset(detail)).toBe(0);
  expect(
    await detail.evaluate((element) => element.getAnimations().length),
  ).toBe(0);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(navigationStory);
  await root
    .getByRole("link", { name: "Web workspace web", exact: true })
    .click();
  await expect(root).toHaveAttribute("data-value", "detail");
  expect(await offset(detail)).toBe(0);
  expect(
    await detail.evaluate((element) => element.getAnimations().length),
  ).toBe(0);
});
