import { expect, test } from "@playwright/test";

const story = (name = "task-details") =>
  `/iframe.html?id=resources-tasks--${name}&viewMode=story`;
const detail = (page: import("@playwright/test").Page) =>
  page.getByRole("region", { name: "Task details", exact: true });
const title = (page: import("@playwright/test").Page) =>
  detail(page).getByRole("textbox", { name: "Title", exact: true });

test("dedicated details show neighboring panels and background wheel advances one task per gesture", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(story());
  await expect(title(page)).toHaveValue("Connect the task table");
  await expect(
    page
      .getByLabel("Task toolbar", { exact: true })
      .getByRole("button", { name: /Back/ }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("dialog", { name: "Task details", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("table", { name: "Tasks", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", {
      name: "Previous task: Wait for CI",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: "Next task: Review project isolation",
      exact: true,
    }),
  ).toBeVisible();
  const panels = page.locator("[data-task-detail-panel]");
  expect(await panels.count()).toBeLessThanOrEqual(3);
  await page.clock.install();
  await page.locator("[data-task-deck]").evaluate((root) => {
    for (let i = 0; i < 20; i++)
      root.dispatchEvent(
        new WheelEvent("wheel", {
          deltaY: 100,
          bubbles: true,
          cancelable: true,
        }),
      );
  });
  await expect(title(page)).toHaveValue("Review project isolation");
  await expect(page.getByLabel("Preview route")).toContainText("/task/task-2");
  await page.getByRole("button", { name: "Back", exact: true }).click();
  await expect(
    page.getByRole("table", { name: "Tasks", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Forward", exact: true }).click();
  await expect(title(page)).toHaveValue("Review project isolation");
  await page.clock.fastForward(600);
  await page
    .locator("[data-task-deck]")
    .dispatchEvent("wheel", { deltaY: -100, bubbles: true, cancelable: true });
  await expect(title(page)).toHaveValue("Connect the task table");
  expect(await panels.count()).toBeLessThanOrEqual(3);
  expect(errors).toEqual([]);
});

for (const width of [1280, 390]) {
  test(`detail content scrolls independently and peeks stay clipped at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 844 });
    await page.goto(story("long-task-details"));
    await expect(title(page)).toHaveValue("Connect the task table");
    const viewport = detail(page)
      .getByLabel("Task metadata", { exact: true })
      .locator('[data-scroll-area="viewport"]')
      .first();
    const before = await viewport.evaluate((node) => ({
      height: node.clientHeight,
      total: node.scrollHeight,
    }));
    expect(before.total).toBeGreaterThan(before.height);
    await viewport.hover({ position: { x: 5, y: 5 } });
    await page.mouse.wheel(0, 800);
    await expect
      .poll(() => viewport.evaluate((node) => node.scrollTop))
      .toBeGreaterThan(0);
    await expect(title(page)).toHaveValue("Connect the task table");
    const geometry = await page.locator("[data-task-deck]").evaluate((root) => {
      const panel = root
        .querySelector("[data-task-detail-panel][data-active]")!
        .getBoundingClientRect();
      return [...root.querySelectorAll("[data-task-neighbor]")].map(
        (button) => {
          const full = button.getBoundingClientRect(),
            clip = button.parentElement!.getBoundingClientRect();
          return {
            height: full.height,
            visible: clip.height,
            top: clip.top,
            bottom: clip.bottom,
            panelTop: panel.top,
            panelBottom: panel.bottom,
          };
        },
      );
    });
    expect(geometry).toHaveLength(2);
    for (const edge of geometry) expect(edge.visible).toBeLessThan(edge.height);
    expect(geometry[0].bottom).toBeLessThan(geometry[0].panelTop);
    expect(geometry[1].top).toBeGreaterThan(geometry[1].panelBottom);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  });
}

test("leaving a dirty task requires confirmation and discarded edits do not return", async ({
  page,
}) => {
  await page.goto(story());
  await title(page).fill("Local draft");
  const next = page.getByRole("button", {
    name: "Next task: Review project isolation",
    exact: true,
  });
  await next.click();
  const confirm = page.getByRole("dialog", {
    name: "Discard unsaved changes",
    exact: true,
  });
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(title(page)).toHaveValue("Local draft");
  await next.click();
  await confirm
    .getByRole("button", { name: "Discard changes", exact: true })
    .click();
  await expect(title(page)).toHaveValue("Review project isolation");
  await page
    .getByRole("button", {
      name: "Previous task: Connect the task table",
      exact: true,
    })
    .click();
  await expect(title(page)).toHaveValue("Connect the task table");
});

test("task details use resource panels with information and actions beside the larger content", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 844 });
  await page.goto(story());
  await expect(title(page)).toHaveValue("Connect the task table");
  const information = detail(page).getByRole("region", {
    name: "Task information",
    exact: true,
  });
  const actions = detail(page).getByRole("region", {
    name: "Task actions",
    exact: true,
  });
  const content = detail(page).getByRole("region", {
    name: "Task title",
    exact: true,
  });
  await expect(
    information.getByRole("textbox", { name: "Alias", exact: true }),
  ).toHaveValue("task-3");
  await expect(
    actions.getByRole("button", { name: "Save", exact: true }),
  ).toBeVisible();
  await expect(detail(page).locator(".floating-card")).toHaveCount(0);
  const infoBounds = (await information.boundingBox())!;
  const titleBounds = (await content.boundingBox())!;
  const actionsBounds = (await actions.boundingBox())!;
  expect(titleBounds.x).toBeGreaterThanOrEqual(infoBounds.x + infoBounds.width);
  expect(actionsBounds.x).toBeCloseTo(infoBounds.x);
  expect(actionsBounds.width).toBeCloseTo(infoBounds.width);
  const fontSize = (node: import("@playwright/test").Locator) =>
    node.evaluate((node) => parseFloat(getComputedStyle(node).fontSize));
  const titleSize = await fontSize(title(page));
  const descriptionSize = await fontSize(
    detail(page).getByRole("textbox", { name: "Description", exact: true }),
  );
  const aliasSize = await fontSize(
    information.getByRole("textbox", { name: "Alias", exact: true }),
  );
  expect(titleSize).toBeGreaterThan(descriptionSize);
  expect(descriptionSize).toBeGreaterThan(aliasSize);
});

test("saving keeps the detail page open and updates the alias route", async ({
  page,
}) => {
  await page.goto(story());
  await title(page).fill("Updated task title");
  await detail(page)
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("renamed-task");
  await detail(page).getByRole("button", { name: "Save", exact: true }).click();
  await expect(title(page)).toHaveValue("Updated task title");
  await expect(
    detail(page).getByRole("button", { name: "Save", exact: true }),
  ).toBeDisabled();
  await expect(page.getByLabel("Preview route")).toContainText(
    "/task/renamed-task",
  );
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "Back", exact: true }).click();
  await expect(
    page.getByRole("link", {
      name: "Open task: Updated task title",
      exact: true,
    }),
  ).toHaveAttribute("href", "/c/_/project/workspace/task/renamed-task");
});

test("single-task boundaries remain stable and Add still opens a dialog", async ({
  page,
}) => {
  await page.goto(story("single-task-details"));
  await expect(title(page)).toHaveValue("Implement Task MCP tools");
  await expect(page.locator("[data-task-neighbor]")).toHaveCount(0);
  await page
    .locator("[data-task-deck]")
    .dispatchEvent("wheel", { deltaY: 100, bubbles: true, cancelable: true });
  await expect(title(page)).toHaveValue("Implement Task MCP tools");
  await page.getByRole("button", { name: "Add task", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add task", exact: true });
  await expect(add).toBeVisible();
  await add.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(title(page)).toHaveValue("Implement Task MCP tools");
});

test("keyboard navigation works on the background and missing task links remain recoverable", async ({
  page,
}) => {
  await page.goto(story());
  const deck = page.getByLabel("Task navigation", { exact: true });
  await deck.focus();
  await page.keyboard.press("PageDown");
  await expect(title(page)).toHaveValue("Review project isolation");
  await page.goto(story("missing-task-details"));
  await expect(
    detail(page).getByRole("button", { name: "Retry", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Back", exact: true }).click();
  await expect(
    page.getByRole("table", { name: "Tasks", exact: true }),
  ).toBeVisible();
});
