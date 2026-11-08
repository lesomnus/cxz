import { expect, test } from "@playwright/test";

const story = (name = "created-tasks") =>
  `/iframe.html?id=conversation-worksummary--${name}&viewMode=story`;
const menuName = { name: "Created tasks", exact: true };

test("Task links use shared menu geometry without a card header", async ({
  page,
}) => {
  for (const [width, radius] of [
    [1440, 6],
    [360, 12],
  ]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(story());
    const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
    await expect(trigger).toBeEnabled();
    await trigger.evaluate((node, radius) => {
      node.style.borderRadius = `${radius}px`;
    }, radius);
    await trigger.click();
    const menu = page.getByRole("menu", menuName);
    await expect(menu).toBeVisible();
    await expect(menu.locator("header")).toHaveCount(0);
    await expect(
      menu.getByRole("button", { name: "Close", exact: true }),
    ).toHaveCount(0);
    await expect(menu.locator('a[role="menuitem"]')).toHaveCount(15);
    await expect
      .poll(async () =>
        menu.evaluate((surface) => {
          const item =
            surface.querySelector<HTMLElement>('a[role="menuitem"]')!;
          const frame = surface.getBoundingClientRect();
          const row = item.getBoundingClientRect();
          const outer = getComputedStyle(surface);
          const inner = getComputedStyle(item);
          const top = row.top - frame.top - parseFloat(outer.borderTopWidth);
          const left =
            row.left - frame.left - parseFloat(outer.borderLeftWidth);
          const right =
            frame.right - row.right - parseFloat(outer.borderRightWidth);
          const outerRadius = parseFloat(outer.borderTopLeftRadius);
          const innerRadius = parseFloat(inner.borderTopLeftRadius);
          return Math.max(
            Math.abs(top - left),
            Math.abs(top - right),
            Math.abs(frame.left + outerRadius - (row.left + innerRadius)),
            Math.abs(frame.top + outerRadius - (row.top + innerRadius)),
          );
        }),
      )
      .toBeLessThan(0.05);
    await page.keyboard.press("ArrowDown");
    await expect(menu.locator('a[role="menuitem"]').nth(1)).toBeFocused();
    await page.keyboard.press("End");
    await expect(menu.locator('a[role="menuitem"]').last()).toBeFocused();
    await page.keyboard.press("Home");
    await expect(menu.locator('a[role="menuitem"]').first()).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
  }
});

test("WorkSummary opens created Tasks above the trigger without shifting the conversation", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(story());
  const summaries = page.locator("[data-work-summary]");
  await expect(summaries).toHaveCount(2);
  const trigger = summaries.last().locator('[data-work-service="cxz_tasks"]');
  await expect(trigger).toHaveText("015");
  await expect(trigger).toHaveAccessibleName("Created tasks: 15");
  await expect(
    summaries.first().locator('[data-work-service="cxz_tasks"]'),
  ).toBeDisabled();
  await expect(
    summaries.last().locator('[data-work-service="cxz_memory"]'),
  ).toBeDisabled();
  const before = (await trigger.boundingBox())!;
  await trigger.hover();
  await expect
    .poll(() =>
      trigger.evaluate((node) =>
        node
          .getAnimations()
          .every((animation) => animation.playState !== "running"),
      ),
    )
    .toBe(true);
  const hoverBackground = await trigger.evaluate(
    (node) => getComputedStyle(node).backgroundColor,
  );
  await trigger.click();
  await page.mouse.move(1, 1);
  const menu = page.getByRole("menu", menuName);
  await expect(menu).toBeVisible();
  await expect(menu.locator('a[role="menuitem"]')).toHaveCount(15);
  await expect(
    menu.getByRole("menuitem", { name: /Implement Task MCP tools/ }),
  ).toHaveCount(1);
  await expect(menu.locator('a[href$="/task/task-1"]')).toHaveCount(0);
  const box = (await menu.boundingBox())!;
  expect(box.y + box.height).toBeLessThanOrEqual(before.y);
  expect(Math.abs((await trigger.boundingBox())!.y - before.y)).toBeLessThan(1);
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(trigger).not.toBeFocused();
  await expect(trigger).toHaveCSS("outline-style", "none");
  await expect(trigger).toHaveCSS("background-color", hoverBackground);
  const viewport = menu.locator(".created-tasks-menu-viewport");
  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(menu).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(trigger).toBeFocused();

  await trigger.click();
  await page.locator("textarea").click();
  await expect(page.getByRole("menu", menuName)).toHaveCount(0);

  await trigger.click();
  const link = page
    .getByRole("menu", menuName)
    .locator('a[role="menuitem"]')
    .first();
  await expect(link).toHaveAttribute(
    "href",
    "/c/_/project/workspace/task/task-4",
  );
  await link.click();
  await expect(page.getByRole("menu", menuName)).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveText("Wait for CI");
  expect(errors).toEqual([]);
});

test("narrow viewport keeps the created Tasks popup inside the screen", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 740 });
  await page.goto(story());
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await trigger.click();
  const menu = page.getByRole("menu", menuName);
  await expect(menu.locator('a[role="menuitem"]')).toHaveCount(15);
  const box = (await menu.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(360);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.y + box.height).toBeLessThanOrEqual(740);
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("zero Task counts stay disabled while loading and after read-only calls", async ({
  page,
}) => {
  await page.goto(story("loading-tasks"));
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await expect(trigger).toHaveAttribute("aria-busy", "true");
  await expect(trigger).toHaveText("000");
  await expect(trigger).toBeDisabled();
  const before = (await trigger.boundingBox())!;
  await expect(trigger).toHaveText("015");
  await expect(trigger).toBeEnabled();
  const after = (await trigger.boundingBox())!;
  expect(after.width).toBe(before.width);
  expect(after.height).toBe(before.height);
  await page.goto(story("no-created-tasks"));
  for (const button of await page
    .locator('[data-work-service="cxz_tasks"]')
    .all()) {
    await expect(button).toHaveText("000");
    await expect(button).toBeDisabled();
  }
  await expect(page.getByRole("menu", menuName)).toHaveCount(0);
});

test("cached Task counts and open lists update together and preserve trigger styling", async ({
  page,
}) => {
  await page.goto(story("refreshing-tasks"));
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await expect(trigger).toHaveText("001");
  await trigger.click();
  const menu = page.getByRole("menu", menuName);
  await expect(menu.getByRole("status")).toHaveText("Loading…");
  await expect(menu.locator('a[role="menuitem"]')).toHaveCount(1);
  const loading = (await menu.boundingBox())!;
  await expect(menu.locator('a[role="menuitem"]')).toHaveCount(15);
  await expect(trigger).toHaveText("015");
  expect((await menu.boundingBox())!.height).toBeGreaterThan(loading.height);
  expect((await menu.boundingBox())!.height).toBeLessThanOrEqual(
    page.viewportSize()!.height / 2,
  );
  await expect(trigger).toHaveCSS("outline-style", "none");
  await page.keyboard.press("Escape");
  await page.locator('[data-work-service="cxz_tasks"]').last().click();
  await expect(menu.locator('a[role="menuitem"]')).toHaveCount(15);
  await expect(menu.getByRole("status")).toHaveCount(0);
});

test("cached Tasks remain accessible after a refresh fails", async ({
  page,
}) => {
  await page.goto(story("unavailable-tasks"));
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await expect(trigger).toHaveText("001");
  await trigger.click();
  await expect(
    page.getByRole("menu", menuName).locator('a[role="menuitem"]'),
  ).toHaveCount(1);
  await expect(
    page.getByRole("menu", menuName).getByRole("alert"),
  ).toContainText("Update the Manager and project runtime");
  await expect(
    page.getByRole("menu", menuName).getByRole("menuitem", { name: "Retry" }),
  ).toBeVisible();
  await expect(trigger).toHaveCSS("outline-style", "none");
});

test("the menu follows conversation scrolling until its trigger leaves the visible viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1000, height: 600 });
  await page.goto(story("scrolling-conversation"));
  const transcript = page.locator(".transcript");
  const trigger = page.locator('[data-work-service="cxz_tasks"]').last();
  await expect(trigger).toBeEnabled();
  await trigger.click();
  await page.mouse.move(1, 1);
  const menu = page.getByRole("menu", menuName);
  await expect(menu.locator('a[role="menuitem"]')).toHaveCount(15);
  const triggerBefore = (await trigger.boundingBox())!;
  const popupBefore = (await menu.boundingBox())!;
  const above = popupBefore.y + popupBefore.height <= triggerBefore.y;
  const gapBefore = above
    ? triggerBefore.y - popupBefore.y - popupBefore.height
    : popupBefore.y - triggerBefore.y - triggerBefore.height;
  await transcript.evaluate((node) => {
    node.scrollTop -= 16;
  });
  await expect
    .poll(async () => (await trigger.boundingBox())!.y)
    .toBeGreaterThan(triggerBefore.y + 10);
  await expect(menu).toBeVisible();
  await expect
    .poll(async () => {
      const anchor = (await trigger.boundingBox())!;
      const popup = (await menu.boundingBox())!;
      const gap = above
        ? anchor.y - popup.y - popup.height
        : popup.y - anchor.y - anchor.height;
      return Math.abs(gap - gapBefore);
    })
    .toBeLessThan(1);
  await transcript.evaluate((node) => {
    node.scrollTop = 0;
  });
  await expect(menu).toHaveCount(0);
});
