import { expect, test } from "@playwright/test";

const story = (name = "created-memories") =>
  `/iframe.html?id=conversation-worksummary-memory--${name}&viewMode=story`;
const menuName = { name: "Created memories", exact: true };

test("Memory counters list the same created notes and link to their detail pages", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(story());
  const triggers = page.locator('[data-work-service="cxz_memory"]');
  await expect(triggers).toHaveCount(2);
  await expect(triggers.first()).toHaveAccessibleName("Created memories: 0");
  await expect(triggers.first()).toBeDisabled();
  const trigger = triggers.last();
  await expect(trigger).toHaveAccessibleName("Created memories: 23");
  const before = await page.locator(".composer").boundingBox();
  await trigger.click();
  const menu = page.getByRole("menu", menuName);
  await expect(menu).toBeVisible();
  await expect(menu.locator("header")).toHaveCount(0);
  const items = menu.getByRole("menuitem");
  await expect(items).toHaveCount(23);
  expect(await page.locator(".composer").boundingBox()).toEqual(before);
  await expect(items.first()).toContainText("Memory note 1");
  await expect(items.first()).toHaveAttribute(
    "href",
    /\/c\/_\/project\/web\/memory\//,
  );
  await expect(items.first().locator("svg")).toBeVisible();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await page.keyboard.press("ArrowDown");
  await expect(items.nth(1)).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await trigger.click();
  await items.first().click();
  await expect(
    page.getByRole("main", { name: "Shared memory workspace" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Edit Memory name", exact: true }),
  ).toContainText("Memory note 1");
  expect(errors).toEqual([]);
});

test("Memory menu follows its visible trigger and scrolls independently", async ({
  page,
}) => {
  await page.goto(story("scrolling-conversation"));
  const trigger = page.locator('[data-work-service="cxz_memory"]').last();
  await expect(trigger).toBeEnabled();
  await trigger.click();
  const menu = page.getByRole("menu", menuName);
  await expect(menu).toBeVisible();
  const list = menu.locator(".created-memories-menu-viewport");
  expect(
    await list.evaluate((node) => node.scrollHeight > node.clientHeight),
  ).toBe(true);
  const root = page.locator(".transcript");
  await root.evaluate((node) => {
    node.scrollTop -= 24;
    node.dispatchEvent(new Event("scroll"));
  });
  await expect(menu).toBeVisible();
  await expect
    .poll(async () => {
      const a = (await trigger.boundingBox())!;
      const b = (await menu.boundingBox())!;
      return Math.abs(b.x - a.x);
    })
    .toBeLessThan(1);
  const position = await root.evaluate((node) => node.scrollTop);
  await list.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  expect(await root.evaluate((node) => node.scrollTop)).toBe(position);
  await page.locator(".composer textarea").click();
  await expect(menu).toHaveCount(0);
});

test("empty and initially loading Memory counters remain disabled", async ({
  page,
}) => {
  await page.goto(story("no-created-memories"));
  await expect(page.locator('[data-work-service="cxz_memory"]')).toHaveCount(2);
  for (const button of await page
    .locator('[data-work-service="cxz_memory"]')
    .all()) {
    await expect(button).toBeDisabled();
    await expect(button).toHaveAccessibleName("Created memories: 0");
  }
  await page.goto(story("loading-memories"));
  const trigger = page.locator('[data-work-service="cxz_memory"]').last();
  await expect(trigger).toHaveAttribute("aria-busy", "true");
  await expect(trigger).toBeDisabled();
  await expect(trigger).toHaveAccessibleName("Created memories: 23");
  await expect(trigger).toBeEnabled();
});
