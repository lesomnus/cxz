import { expect, test } from "@playwright/test";

const story = (name = "task-details") =>
  `/iframe.html?id=resources-tasks--${name}&viewMode=story`;
const detail = (page: import("@playwright/test").Page) =>
  page.getByRole("region", { name: "Task details", exact: true });
const title = (page: import("@playwright/test").Page) =>
  detail(page).getByRole("textbox", { name: "Title", exact: true });

test("title reserves two clamped lines and identity fields share one row with resource buttons", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(story());
  await expect(title(page)).toHaveValue("Connect the task table");
  await page.evaluate(() => document.fonts.ready);
  const titlePanel = detail(page).getByRole("region", {
    name: "Task title",
    exact: true,
  });
  const before = (await titlePanel.boundingBox())!.height;
  const long =
    "Explain the task navigation and resource metadata with a clear title that remains readable across two lines. ".repeat(
      2,
    );
  await title(page).fill(long);
  await detail(page)
    .getByRole("textbox", { name: "Description", exact: true })
    .focus();
  const preview = titlePanel.locator('textarea + span[aria-hidden="true"]');
  await expect(preview).toHaveCSS("-webkit-line-clamp", "2");
  expect((await titlePanel.boundingBox())!.height).toBeCloseTo(before, 2);
  expect(
    await preview.evaluate((node) => node.scrollHeight > node.clientHeight),
  ).toBe(true);
  const information = detail(page).getByRole("region", {
    name: "Task information",
    exact: true,
  });
  const fields = await information
    .locator("dl")
    .nth(1)
    .locator(":scope > div")
    .evaluateAll((nodes) =>
      nodes.map((node) => ({
        label: node.querySelector("dt")!.textContent!.trim(),
        x: node.getBoundingClientRect().x,
        y: node.getBoundingClientRect().y,
      })),
    );
  expect(fields.map((field) => field.label)).toEqual([
    "Project",
    "Assigned to",
    "Created from",
  ]);
  expect(new Set(fields.map((field) => field.y)).size).toBe(1);
  expect(fields[1].x).toBeGreaterThan(fields[0].x);
  expect(fields[2].x).toBeGreaterThan(fields[1].x);
  const controls = await information
    .getByRole("group", { name: "Task fields", exact: true })
    .locator(":scope > *")
    .evaluateAll((nodes) =>
      nodes.slice(0, 3).map((node) => ({
        y: node.getBoundingClientRect().y,
        x: node.getBoundingClientRect().x,
        text: node.textContent,
      })),
    );
  expect(controls[0].text).toContain("Priority");
  expect(controls[1].text).toContain("Status");
  expect(controls[2].text).toContain("Task ID");
  expect(new Set(controls.map((control) => control.y)).size).toBe(1);
  expect(controls[2].x).toBeGreaterThan(controls[1].x);
  const dates = await information
    .locator("dl")
    .last()
    .locator(":scope > div")
    .evaluateAll((nodes) =>
      nodes.map((node) => ({
        label: node.querySelector("dt")!.textContent!,
        y: node.getBoundingClientRect().y,
      })),
    );
  expect(dates[0].label).toContain("Created");
  expect(dates[1].label).toContain("Updated");
  expect(dates[0].y).toBe(dates[1].y);
  const links = information.getByRole("link");
  await expect(links).toHaveCount(3);
  for (const link of await links.all())
    await expect(link.locator('[data-icon="open-external"]')).toBeVisible();
  expect(
    await links.evaluateAll((nodes) =>
      nodes.map((node) => node.getAttribute("href")),
    ),
  ).toEqual(["/c/_/project/workspace", "/c/_/session/elm", "/c/_/session/oak"]);
  const sidebar = (await information.locator("..").boundingBox())!;
  const description = (await detail(page)
    .getByRole("region", { name: "Task description", exact: true })
    .boundingBox())!;
  expect(sidebar.y + sidebar.height).toBeLessThan(
    description.y + description.height,
  );
});

test("detail controls retain wrapping, show priority symbols and preserve read-only status on save", async ({
  page,
}) => {
  await page.goto(story());
  const description = detail(page).getByRole("textbox", {
    name: "Description",
    exact: true,
  });
  const wrap = detail(page).getByRole("button", {
    name: "Break words",
    exact: true,
  });
  const save = detail(page).getByRole("button", { name: "Save", exact: true });
  await expect(wrap).toHaveAttribute("aria-pressed", "false");
  await expect(description).toHaveCSS("word-break", "keep-all");
  await wrap.click();
  await expect(description).toHaveAttribute("wrap", "soft");
  await expect(description).toHaveCSS("word-break", "break-all");
  await expect(save).toBeDisabled();
  await description.fill("x".repeat(600));
  expect(
    await description.evaluate((node) => node.scrollWidth <= node.clientWidth),
  ).toBe(true);
  await expect(
    detail(page).getByRole("combobox", { name: "Status", exact: true }),
  ).toHaveCount(0);
  await expect(
    detail(page).getByLabel("Status", { exact: true }),
  ).toContainText("In progress");
  const priority = detail(page).getByRole("combobox", {
    name: "Priority",
    exact: true,
  });
  await expect(priority.locator('svg[data-priority="2"]')).toBeVisible();
  await priority.click();
  for (let value = 0; value < 5; value++)
    await expect(
      page.getByRole("listbox").locator(`svg[data-priority="${value}"]`),
    ).toBeVisible();
  await page.getByRole("option", { name: "High", exact: true }).click();
  await expect(priority.locator('svg[data-priority="1"]')).toBeVisible();
  await title(page).focus();
  await title(page).press("Enter");
  await expect(save).toBeDisabled();
  await expect(description).toHaveAttribute("wrap", "soft");
  await expect(description).toHaveCSS("word-break", "break-all");
  await expect(
    detail(page).getByLabel("Status", { exact: true }),
  ).toContainText("In progress");
  await expect(priority).toHaveText("High");
  await wrap.click();
  await expect(description).toHaveAttribute("wrap", "soft");
  await expect(description).toHaveCSS("word-break", "keep-all");
  await expect(save).toBeDisabled();
});

test("description keeps words whole and control markers preserve text, selection and scroll", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(story());
  const description = detail(page).getByRole("textbox", {
    name: "Description",
    exact: true,
  });
  const controls = detail(page).getByRole("button", {
    name: "Show control characters",
    exact: true,
  });
  const words = detail(page).getByRole("button", {
    name: "Break words",
    exact: true,
  });
  const save = detail(page).getByRole("button", { name: "Save", exact: true });
  await expect(description).toHaveCSS("word-break", "keep-all");
  await controls.click();
  await expect(save).toBeDisabled();
  const value = (
    "wordboundary 단어경계를유지합니다 mango pear banana ".repeat(4) + "\tend\n"
  ).repeat(20);
  await description.fill(value);
  const overlay = detail(page).locator("[data-control-character-overlay]");
  await expect(overlay).toBeVisible();
  expect(await overlay.evaluate((node) => node.textContent)).toBe(value);
  for (const symbol of ["·", "→", "¶"])
    expect(
      await overlay.locator(`[data-symbol="${symbol}"]`).count(),
    ).toBeGreaterThan(0);
  const splitWords = () =>
    overlay.evaluate((root) => {
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
      let node: Node | null,
        split = 0;
      while ((node = walker.nextNode())) {
        if (
          !/^\S+$/u.test(node.textContent ?? "") ||
          node.parentElement?.hasAttribute("data-symbol")
        )
          continue;
        const range = document.createRange();
        range.selectNodeContents(node);
        if (range.getClientRects().length > 1) split++;
      }
      return split;
    });
  expect(await splitWords()).toBe(0);
  await description.evaluate((node) =>
    (node as HTMLTextAreaElement).setSelectionRange(5, 19),
  );
  await words.click();
  await expect(description).toHaveCSS("word-break", "break-all");
  await expect.poll(splitWords).toBeGreaterThan(0);
  await expect(description).toHaveAttribute("wrap", "soft");
  await expect(description).toHaveValue(value);
  expect(
    await description.evaluate((node) => [
      (node as HTMLTextAreaElement).selectionStart,
      (node as HTMLTextAreaElement).selectionEnd,
    ]),
  ).toEqual([5, 19]);
  const top = await description.evaluate((node) => {
    node.scrollTop = 120;
    node.dispatchEvent(new Event("scroll"));
    return node.scrollTop;
  });
  expect(top).toBeGreaterThan(0);
  const translated = await overlay
    .locator(":scope > div")
    .evaluate(
      (node) => new DOMMatrixReadOnly(getComputedStyle(node).transform).m42,
    );
  expect(translated).toBe(-top);
  const measured = await overlay.evaluate((node) => {
    const input = node.parentElement!.querySelector("textarea")!;
    const a = node.getBoundingClientRect(),
      b = input.getBoundingClientRect();
    return {
      x: a.x - b.x,
      y: a.y - b.y,
      width: a.width,
      nativeWidth:
        parseFloat(getComputedStyle(input).width) -
        input.offsetWidth +
        input.clientWidth,
    };
  });
  expect(measured.x).toBeCloseTo(0, 1);
  expect(measured.y).toBeCloseTo(0, 1);
  expect(measured.width).toBeCloseTo(measured.nativeWidth, 1);
  await save.click();
  await expect(save).toBeDisabled();
  await expect(description).toHaveValue(value);
  await expect(controls).toHaveAttribute("aria-pressed", "true");
  await expect(words).toHaveAttribute("aria-pressed", "true");
  await controls.click();
  await expect(overlay).toHaveCount(0);
  await expect(description).toHaveValue(value);
  await expect(save).toBeDisabled();
});

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
      .getByLabel(width >= 892 ? "Task metadata" : "Task content", {
        exact: true,
      })
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
