import { expect, test } from "@playwright/test";

const taskPage =
  "/iframe.html?id=resources-tasks--project-table&viewMode=story";
test("task table creates and edits work, and releases a stopped owner's task only after confirmation", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(taskPage);
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Workspace");
  await expect(
    page.getByText("Session stopped", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Previous run", { exact: true })).toHaveCount(1);
  const updated = page
    .locator('[data-task-alias="task-1"] td')
    .last()
    .locator("time");
  await expect(updated).toHaveAttribute("datetime", /T.*Z$/);
  await expect(page.getByRole("link", { name: /elm/ })).toHaveAttribute(
    "href",
    "/c/_/session/elm",
  );
  await page.getByRole("button", { name: "Add task", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add task", exact: true });
  const priority = add.getByRole("radiogroup", {
    name: "Priority",
    exact: true,
  });
  await expect(priority.getByRole("radio")).toHaveCount(5);
  await expect(
    priority.getByRole("radio", { name: "Normal", exact: true }),
  ).toBeChecked();
  await priority.getByRole("radio", { name: "High", exact: true }).check();
  await page.keyboard.press("ArrowRight");
  await expect(
    priority.getByRole("radio", { name: "Normal", exact: true }),
  ).toBeChecked();
  await page.keyboard.press("ArrowLeft");
  await expect(
    priority.getByRole("radio", { name: "High", exact: true }),
  ).toBeChecked();
  const widths = await priority
    .locator("label")
    .evaluateAll((labels) =>
      labels.map((label) => label.getBoundingClientRect().width),
    );
  expect(Math.max(...widths) - Math.min(...widths)).toBeLessThan(0.1);
  const groupBox = (await priority.boundingBox())!;
  const titleBox = (await add
    .getByRole("textbox", { name: "Title", exact: true })
    .boundingBox())!;
  expect(Math.abs(groupBox.width - titleBox.width)).toBeLessThan(0.1);
  expect(Math.abs(groupBox.x - titleBox.x)).toBeLessThan(0.1);
  await add.screenshot({ path: "/tmp/task-add-priority.png" });
  await add
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Ship the task table");
  await add
    .getByRole("textbox", { name: "Description", exact: true })
    .fill("Verify the project workflow.");
  await add.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(add).toHaveCount(0);
  const tasks = page.getByRole("table", { name: "Tasks", exact: true });
  await tasks
    .getByRole("button", {
      name: "Open task: Ship the task table",
      exact: true,
    })
    .click();
  const detail = page.getByRole("dialog", {
    name: "Task details",
    exact: true,
  });
  await expect(
    detail.getByRole("combobox", { name: "Priority", exact: true }),
  ).toHaveText("High");
  await expect(
    detail.getByRole("textbox", { name: "Description", exact: true }),
  ).toHaveValue("Verify the project workflow.");
  await detail
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Ship the verified task table");
  await detail.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    tasks.getByRole("button", {
      name: "Open task: Ship the verified task table",
      exact: true,
    }),
  ).toBeVisible();
  await page
    .getByRole("button", {
      name: "Open task: Connect the task table",
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
    tasks.getByRole("button", {
      name: "Open task: Connect the task table",
      exact: true,
    }),
  ).toBeVisible();
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
  await page.goto(taskPage);
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
  await page.goto(taskPage);
  await expect(
    page.getByRole("table", { name: "Tasks", exact: true }).locator("tbody tr"),
  ).toHaveCount(9);
  expect(
    await page
      .locator("main .panel-scroll-content")
      .evaluate((viewport) => viewport.scrollWidth > viewport.clientWidth),
  ).toBe(true);
  await page.screenshot({ path: "/tmp/tasks-table-narrow.png" });
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
  await expect(page.getByText("No tasks", { exact: true })).toHaveCount(1);
  await page.getByRole("button", { name: "Add task", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add task", exact: true });
  await add
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Documentation only");
  await add.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(
    page.getByRole("button", {
      name: "Open task: Documentation only",
      exact: true,
    }),
  ).toBeVisible();
  await project.click();
  await page.getByRole("option", { name: "Workspace", exact: true }).click();
  await expect(page.getByText("No tasks", { exact: true })).toHaveCount(1);
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

test("task rows stay newest-created first with Done last after a live update", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(taskPage);
  const tasks = page.getByRole("table", { name: "Tasks", exact: true });
  const rows = tasks.locator("tbody tr[data-task-alias]");
  await expect(rows).toHaveCount(9);
  const aliases = () =>
    rows.evaluateAll((rows) =>
      rows.map((row) => row.getAttribute("data-task-alias")),
    );
  const expected = [
    "task-9",
    "task-8",
    "task-7",
    "task-4",
    "task-3",
    "task-2",
    "task-1",
    "task-6",
    "task-5",
  ];
  expect(await aliases()).toEqual(expected);
  const tableBounds = (await tasks.boundingBox())!;
  expect(tableBounds.width).toBeLessThan(900);
  expect(tableBounds.x + tableBounds.width / 2).toBeCloseTo(720, 0);
  await page.screenshot({ path: "/tmp/tasks-table-desktop.png" });
  await page
    .getByRole("button", { name: "Simulate agent update", exact: true })
    .click();
  await expect(
    tasks.getByRole("button", {
      name: "Open task: Agent updated the task",
      exact: true,
    }),
  ).toBeVisible();
  expect(await aliases()).toEqual(
    expected.map((alias) =>
      alias === "task-1" ? "agent-renamed-task" : alias,
    ),
  );
});

test("many tasks remain one scrollable table with every completion at the end", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-tasks--many-tasks&viewMode=story");
  const tasks = page.getByRole("table", { name: "Tasks", exact: true });
  const rows = tasks.locator("tbody tr[data-task-alias]");
  await expect(rows).toHaveCount(90);
  await expect(tasks.locator("tbody tr[data-done]")).toHaveCount(20);
  expect(
    await rows.evaluateAll((rows) =>
      rows
        .slice(-20)
        .every((row) => row.getAttribute("data-task-status") === "Done"),
    ),
  ).toBe(true);
  await rows.last().getByRole("button").first().scrollIntoViewIfNeeded();
  await expect(rows.last()).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollHeight <= innerHeight,
    ),
  ).toBe(true);
});

test("task columns show status icons, distinct priority symbols and relative times", async ({
  page,
}) => {
  await page.goto(taskPage);
  const tasks = page.getByRole("table", { name: "Tasks", exact: true });
  await expect(tasks.getByRole("columnheader")).toHaveText([
    "",
    "Task",
    "Assignee",
    "",
    "Updated",
  ]);
  const first = tasks.locator('[data-task-alias="task-9"]');
  await expect(tasks.getByRole("checkbox")).toHaveCount(0);
  expect((await first.locator("td").nth(0).boundingBox())!.width).toBe(32);
  await expect(
    first.locator("td").nth(0).getByRole("img", { name: "Todo", exact: true }),
  ).toBeVisible();
  const highest = tasks
    .locator('[data-task-alias="task-1"]')
    .getByRole("img", { name: "Highest priority", exact: true });
  const low = first.getByRole("img", { name: "Low priority", exact: true });
  const glyph = (indicator: import("@playwright/test").Locator) =>
    indicator.locator(":scope > svg");
  expect((await glyph(highest).boundingBox())!.width).toBe(
    (await glyph(low).boundingBox())!.width,
  );
  expect(
    await glyph(highest).evaluate((el) => getComputedStyle(el).color),
  ).not.toBe(await glyph(low).evaluate((el) => getComputedStyle(el).color));
  const symbols = tasks.locator("tbody td:nth-child(4) svg");
  const names = await symbols.evaluateAll((icons) =>
    icons.map((icon) => icon.getAttribute("data-icon")),
  );
  expect(new Set(names).size).toBe(5);
  const normalCircle = tasks
    .getByRole("img", { name: "Normal priority", exact: true })
    .first()
    .locator("svg > :first-child");
  const smallCircle = tasks
    .getByRole("img", { name: "Lowest priority", exact: true })
    .first()
    .locator("svg > :first-child");
  expect((await normalCircle.boundingBox())!.width).toBeGreaterThan(
    (await smallCircle.boundingBox())!.width,
  );
  await highest.hover();
  await expect(page.getByRole("tooltip")).toHaveText("Highest priority");
  await page.mouse.move(0, 0);
  const updated = first.locator("td").last().locator("time");
  await expect(first.locator("time")).toHaveCount(1);
  await expect(updated).toHaveText(/minute/);
  await updated.hover();
  await expect(page.getByRole("tooltip").locator("time")).toHaveAttribute(
    "datetime",
    (await updated.getAttribute("datetime"))!,
  );
  await page.mouse.move(0, 0);
});

for (const story of ["task-details", "task-by-id"]) {
  test(`nested task details accept ${story === "task-by-id" ? "UUIDs" : "aliases"}`, async ({
    page,
  }) => {
    await page.goto(`/iframe.html?id=resources-tasks--${story}&viewMode=story`);
    const detail = page.getByRole("dialog", {
      name: "Task details",
      exact: true,
    });
    await expect(
      detail.getByRole("textbox", { name: "Title", exact: true }),
    ).toHaveValue("Connect the task table");
    await detail.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(detail).toHaveCount(0);
    await expect(
      page.getByRole("table", { name: "Tasks", exact: true }),
    ).toBeVisible();
  });
}

test("existing project-query bookmarks redirect and preserve session/run filters", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-tasks--existing-bookmark&viewMode=story",
  );
  const detail = page.getByRole("dialog", {
    name: "Task details",
    exact: true,
  });
  await expect(
    detail.getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveValue("Connect the task table");
  await detail.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(detail).toHaveCount(0);
  await expect(
    page
      .getByRole("table", { name: "Tasks", exact: true })
      .locator("tbody tr[data-task-alias]"),
  ).toHaveCount(1);
  const session = page.getByRole("combobox", {
    name: "Select session",
    exact: true,
  });
  await expect(session).toHaveText("UI refinement · This run");
  await session.click();
  await page.getByRole("option", { name: "All sessions", exact: true }).click();
  await expect(
    page
      .getByRole("table", { name: "Tasks", exact: true })
      .locator("tbody tr[data-task-alias]"),
  ).toHaveCount(9);
});

test("paged initial loading publishes all sorted tasks together", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=resources-tasks--paged-loading&viewMode=story",
  );
  await expect(page.getByText("Loading tasks…", { exact: true })).toBeVisible();
  await page.evaluate(() => {
    const counts: number[] = [];
    const observe = () =>
      counts.push(document.querySelectorAll("tr[data-task-alias]").length);
    const observer = new MutationObserver(observe);
    observer.observe(document.body, { subtree: true, childList: true });
    Object.assign(window, {
      taskLoadCounts: counts,
      taskLoadObserver: observer,
    });
    observe();
  });
  const rows = page
    .getByRole("table", { name: "Tasks", exact: true })
    .locator("tbody tr[data-task-alias]");
  await expect(rows).toHaveCount(12);
  const observed = await page.evaluate(() => {
    const state = window as typeof window & {
      taskLoadCounts: number[];
      taskLoadObserver: MutationObserver;
    };
    state.taskLoadObserver.disconnect();
    return state.taskLoadCounts;
  });
  expect(observed).toContain(0);
  expect(observed).toContain(12);
  expect(observed.every((count) => count === 0 || count === 12)).toBe(true);
  expect(
    await rows.evaluateAll((rows) =>
      rows.map((row) => row.getAttribute("data-task-alias")),
    ),
  ).toEqual([
    "task-12",
    "task-11",
    "task-10",
    "task-9",
    "task-8",
    "task-7",
    "task-4",
    "task-3",
    "task-2",
    "task-1",
    "task-6",
    "task-5",
  ]);
  expect(errors).toEqual([]);
});

test("older completions load in batches of 50 and reset when switching projects", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-tasks--completed-history&viewMode=story",
  );
  const tasks = page.getByRole("table", { name: "Tasks", exact: true });
  const rows = tasks.locator("tbody tr[data-task-alias]");
  const more = tasks.getByRole("button", { name: "Load more", exact: true });
  await expect(rows).toHaveCount(9);
  await expect(tasks.locator("tbody tr[data-done]")).toHaveCount(2);
  await expect(
    tasks.getByRole("button", {
      name: "Open task: Completed task 1",
      exact: true,
    }),
  ).toHaveCount(0);
  for (const count of [59, 109, 134]) {
    await more.click();
    await expect(rows).toHaveCount(count);
  }
  await expect(more).toHaveCount(0);
  await expect(tasks.locator("tbody tr[data-done]")).toHaveCount(127);
  await tasks
    .getByRole("button", { name: "Open task: Completed task 125", exact: true })
    .click();
  const detail = page.getByRole("dialog", {
    name: "Task details",
    exact: true,
  });
  await expect(
    detail.getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveValue("Completed task 125");
  await detail.getByRole("button", { name: "Cancel", exact: true }).click();
  const project = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await project.click();
  await page
    .getByRole("option", { name: "Documentation", exact: true })
    .click();
  await expect(page.getByText("No tasks", { exact: true })).toBeVisible();
  await expect(more).toHaveCount(0);
  await project.click();
  await page.getByRole("option", { name: "Workspace", exact: true }).click();
  await expect(rows).toHaveCount(9);
  await expect(more).toBeVisible();
});

test("a project containing only older completions still offers Load more", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-tasks--completed-history&viewMode=story&args=count:0",
  );
  const tasks = page.getByRole("table", { name: "Tasks", exact: true });
  await expect(
    tasks.getByRole("button", { name: "Load more", exact: true }),
  ).toBeVisible();
  await expect(tasks.locator("tbody tr[data-task-alias]")).toHaveCount(0);
  await expect(page.getByText("No tasks", { exact: true })).toHaveCount(0);
  await tasks.getByRole("button", { name: "Load more", exact: true }).click();
  await expect(tasks.locator("tbody tr[data-task-alias]")).toHaveCount(50);
});

test("toolbar session filtering scopes choices to the project and can be cleared", async ({
  page,
}) => {
  await page.goto(taskPage);
  const rows = page
    .getByRole("table", { name: "Tasks", exact: true })
    .locator("tbody tr[data-task-alias]");
  const session = page.getByRole("combobox", {
    name: "Select session",
    exact: true,
  });
  await expect(session).toHaveText("All sessions");
  await expect(rows).toHaveCount(9);
  await session.click();
  await page.getByRole("option", { name: "Task API", exact: true }).click();
  await expect(rows).toHaveCount(5);
  await expect(session).toHaveText("Task API");
  await session.click();
  await page
    .getByRole("option", { name: "UI refinement", exact: true })
    .click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute("data-task-alias", "task-3");
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Documentation", exact: true })
    .click();
  await expect(session).toHaveText("All sessions");
  await session.click();
  await expect(page.getByRole("option")).toHaveCount(1);
  await expect(
    page.getByRole("option", { name: "All sessions", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page.getByRole("option", { name: "Workspace", exact: true }).click();
  await expect(rows).toHaveCount(9);
});

test("Projects, Tasks and Memory toolbars share the reference alignment", async ({
  page,
}) => {
  for (const width of [1440, 800]) {
    await page.setViewportSize({ width, height: 1000 });
    const boxes = [];
    for (const story of [
      "pages-projects--default",
      "resources-tasks--project-table",
      "pages-memory--default",
    ]) {
      await page.goto(`/iframe.html?id=${story}&viewMode=story`);
      const trigger = page.getByRole("combobox", {
        name: "Select project",
        exact: true,
      });
      await expect(trigger).toBeVisible();
      const box = (await trigger.boundingBox())!;
      const header = (await page.locator("main > header").boundingBox())!;
      boxes.push({
        x: box.x,
        y: box.y - header.y,
        width: box.width,
        height: header.height,
      });
    }
    for (const box of boxes.slice(1)) {
      expect(Math.abs(box.x - boxes[0].x)).toBeLessThan(0.5);
      expect(Math.abs(box.width - boxes[0].width)).toBeLessThan(0.5);
      expect(Math.abs(box.y - boxes[0].y)).toBeLessThan(0.5);
      if (width === 1440) expect(box.height).toBe(boxes[0].height);
    }
  }
});
