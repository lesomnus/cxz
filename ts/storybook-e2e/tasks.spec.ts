import { expect, test } from "@playwright/test";

const board = "/iframe.html?id=resources-tasks--project-board&viewMode=story";
test("task board creates and edits work, and releases a stopped owner's task only after confirmation", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(board);
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Workspace");
  await expect(
    page.getByText("Session stopped", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Previous run", { exact: true })).toHaveCount(0);
  const updated = page.locator('[data-task-alias="task-1"] footer time');
  await updated.hover();
  await expect(page.getByRole("tooltip").locator("time")).toHaveAttribute(
    "datetime",
    (await updated.getAttribute("datetime"))!,
  );
  await page.mouse.move(0, 0);
  await expect(page.getByRole("tooltip")).toHaveCount(0);
  await expect(page.getByRole("link", { name: /elm/ })).toHaveAttribute(
    "href",
    "/c/_/session/elm",
  );
  await page.getByRole("button", { name: "Add task", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add task", exact: true });
  await add
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Ship the task board");
  await add
    .getByRole("textbox", { name: "Description", exact: true })
    .fill("Verify the project workflow.");
  await add.getByRole("button", { name: "Add", exact: true }).click();
  await expect(add).toHaveCount(0);
  const todo = page.getByRole("region", { name: "Todo tasks", exact: true });
  await todo
    .getByRole("button", {
      name: "Open task: Ship the task board",
      exact: true,
    })
    .click();
  const detail = page.getByRole("dialog", {
    name: "Task details",
    exact: true,
  });
  await expect(
    detail.getByRole("textbox", { name: "Description", exact: true }),
  ).toHaveValue("Verify the project workflow.");
  await detail
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Ship the verified task board");
  await detail.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    todo.getByRole("button", {
      name: "Open task: Ship the verified task board",
      exact: true,
    }),
  ).toBeVisible();
  await page
    .getByRole("button", {
      name: "Open task: Connect the task board",
      exact: true,
    })
    .click();
  await detail.getByRole("button", { name: "Release", exact: true }).click();
  const confirm = page.getByRole("dialog", {
    name: "Release task?",
    exact: true,
  });
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(detail).toBeVisible();
  await detail.getByRole("button", { name: "Release", exact: true }).click();
  await confirm.getByRole("button", { name: "Release", exact: true }).click();
  await expect(
    todo.getByRole("button", {
      name: "Open task: Connect the task board",
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: /Show canceled/ }).click();
  await expect(
    page.getByRole("button", {
      name: "Open task: Clean up the old experiment",
      exact: true,
    }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("a remote change preserves the draft and requires an explicit reload after a version conflict", async ({
  page,
}) => {
  await page.goto(board);
  await page
    .getByRole("button", {
      name: "Open task: Implement Task MCP tools",
      exact: true,
    })
    .click();
  const detail = page.getByRole("dialog", {
    name: "Task details",
    exact: true,
  });
  const title = detail.getByRole("textbox", { name: "Title", exact: true });
  await title.fill("Keep this local draft");
  await page
    .getByRole("button", { name: "Simulate agent update", exact: true })
    .evaluate((button) => (button as HTMLButtonElement).click());
  await expect(
    detail.getByText(
      "This task was updated elsewhere. Reload to edit its latest version.",
    ),
  ).toBeVisible();
  await expect(title).toHaveValue("Keep this local draft");
  await detail.getByRole("button", { name: "Save", exact: true }).click();
  await expect(detail.getByRole("alert")).toContainText(
    "Your edits are preserved",
  );
  await expect(title).toHaveValue("Keep this local draft");
  await detail
    .getByRole("button", { name: "Reload latest task", exact: true })
    .click();
  await expect(title).toHaveValue("Agent updated the task");
  await page.keyboard.press("Escape");
  await expect(detail).toHaveCount(0);
});

test("project selection isolates work and remains available in the narrow layout", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(
    "/iframe.html?id=resources-tasks--empty-project&viewMode=story",
  );
  const project = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await project.click();
  await page
    .getByRole("option", { name: "Documentation", exact: true })
    .click();
  await expect(page.getByText("No tasks", { exact: true })).toHaveCount(4);
  await page.getByRole("button", { name: "Add task", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add task", exact: true });
  await add
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Documentation only");
  await add.getByRole("button", { name: "Add", exact: true }).click();
  await expect(
    page.getByRole("button", {
      name: "Open task: Documentation only",
      exact: true,
    }),
  ).toBeVisible();
  await project.click();
  await page.getByRole("option", { name: "Workspace", exact: true }).click();
  await expect(page.getByText("No tasks", { exact: true })).toHaveCount(4);
  await expect(
    page.getByRole("button", {
      name: "Open task: Documentation only",
      exact: true,
    }),
  ).toHaveCount(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
