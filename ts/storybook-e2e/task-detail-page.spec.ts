import { expect, test } from "@playwright/test";

const story = (name = "task-details") =>
  `/iframe.html?id=resources-tasks--${name}&viewMode=story`;
const detail = (page: import("@playwright/test").Page) =>
  page.getByRole("region", { name: "Task details", exact: true });
const title = (page: import("@playwright/test").Page) =>
  detail(page).getByRole("textbox", { name: "Title", exact: true });

test("title reserves two lines and metadata groups assignment, activity and final identity rows", async ({
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
  const rows = information.locator("dl");
  const defaultGaps = await information.evaluate((panel) => {
    const alias = panel.querySelector("input")!;
    const aliasLabel = alias.parentElement!.previousElementSibling!;
    return {
      alias:
        alias.getBoundingClientRect().top -
        aliasLabel.getBoundingClientRect().bottom,
      fields: [...panel.querySelectorAll("dl > div")]
        .filter((field) =>
          ["Assigned to", "Priority", "Project", "Created from"].includes(
            field.querySelector("dt")!.textContent!.trim(),
          ),
        )
        .map((field) => ({
          label: field.querySelector("dt")!.textContent!.trim(),
          gap:
            field
              .querySelector("dd")!
              .firstElementChild!.getBoundingClientRect().top -
            field.querySelector("dt")!.getBoundingClientRect().bottom,
        })),
    };
  });
  expect(defaultGaps.fields).toHaveLength(4);
  for (const field of defaultGaps.fields)
    expect(field.gap, field.label).toBeCloseTo(defaultGaps.alias, 1);
  const rowLabels = (index: number) =>
    rows.nth(index).locator(":scope > div > dt").allTextContents();
  expect(await rowLabels(0)).toEqual(["Assigned to", "Priority"]);
  const secondLabels = await rowLabels(1);
  expect(secondLabels[0]).toBe("Status");
  expect(secondLabels[1]).toMatch(/^Updated/);
  expect(await rowLabels(2)).toEqual(["Project", "Created from"]);
  const finalLabels = await rows
    .last()
    .locator(":scope > div > dt")
    .allTextContents();
  expect(finalLabels[0]).toBe("Task ID");
  expect(finalLabels[1]).toMatch(/^Created/);
  for (const [row, expectedRatio] of [
    [rows.nth(0), 2],
    [rows.nth(1), 0.5],
    [rows.last(), 0.5],
  ] as const) {
    const boxes = await row.locator(":scope > div").evaluateAll((fields) =>
      fields.map((field) => ({
        width: field.getBoundingClientRect().width,
        y: field.getBoundingClientRect().y,
      })),
    );
    expect(boxes[0].y).toBe(boxes[1].y);
    expect(boxes[0].width / boxes[1].width).toBeCloseTo(expectedRatio, 1);
  }
  const assignment = information.getByRole("group", {
    name: "Assigned session controls",
    exact: true,
  });
  const geometry = await assignment.evaluate((group) => {
    const frame = group.getBoundingClientRect();
    const button = group.querySelector("a")!.getBoundingClientRect();
    const action = group.querySelector("button")!.getBoundingClientRect();
    return {
      width: frame.width,
      buttonWidth: button.width,
      buttonHeight: button.height,
      leftInset: button.left - frame.left,
      rightInset: frame.right - action.right,
      gap: action.left - button.right,
    };
  });
  const project = (await rows.nth(2).getByRole("link").first().boundingBox())!;
  expect(geometry.buttonWidth).toBeCloseTo(project.width, 1);
  expect(geometry.buttonHeight).toBe(40);
  expect(geometry.width).toBeGreaterThan(geometry.buttonWidth);
  expect(geometry.gap).toBeCloseTo(geometry.leftInset, 1);
  expect(geometry.rightInset).toBeCloseTo(geometry.leftInset, 1);
  const inactiveAction = assignment.getByRole("button", {
    name: "No assignment action",
    exact: true,
  });
  await expect(inactiveAction).toBeDisabled();
  await expect(inactiveAction.locator('[data-icon="dot"]')).toBeVisible();
  await expect(
    information.locator("dt").filter({ hasText: /^Assignment$/ }),
  ).toHaveCount(0);
  await expect(title(page)).toHaveValue(long);
  const navigation = rows.nth(2).getByRole("link");
  const inputPadding = await information
    .getByRole("textbox", { name: "Alias", exact: true })
    .evaluate((input) => parseFloat(getComputedStyle(input).paddingLeft));
  for (const link of await navigation.all()) {
    const inset = await link.evaluate(
      (anchor) =>
        anchor.querySelector(".button-content > span")!.getBoundingClientRect()
          .left - anchor.getBoundingClientRect().left,
    );
    expect(inset).toBeCloseTo(inputPadding, 1);
  }
  for (const time of await information.locator("dd > time").all())
    await expect(time).toHaveText(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/);
  const links = information.getByRole("link");
  await expect(links).toHaveCount(3);
  for (const link of await links.all())
    await expect(link.locator('[data-icon="open-external"]')).toBeVisible();
  expect(
    await links.evaluateAll((nodes) =>
      nodes.map((node) => node.getAttribute("href")),
    ),
  ).toEqual(["/c/_/session/elm", "/c/_/project/workspace", "/c/_/session/oak"]);
  const sidebar = (await information.locator("..").boundingBox())!;
  const description = (await detail(page)
    .getByRole("region", { name: "Task description", exact: true })
    .boundingBox())!;
  expect(sidebar.y + sidebar.height).toBeLessThan(
    description.y + description.height,
  );
});

test("compact task field values align with Account editable labels", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(
    "/iframe.html?id=accounts-workflows--signed-in-account&viewMode=story",
  );
  const account = page.getByRole("button", {
    name: "Edit Display name",
    exact: true,
  });
  await expect(account).toBeVisible();
  const reference = await account.evaluate((button) => {
    const label = button.closest("dd")!.previousElementSibling!;
    const walker = document.createTreeWalker(button, NodeFilter.SHOW_TEXT);
    let node: Node | null;
    while ((node = walker.nextNode())) {
      if (!node.textContent?.trim()) continue;
      const range = document.createRange();
      range.selectNodeContents(node);
      return {
        gap:
          range.getBoundingClientRect().top -
          label.getBoundingClientRect().bottom,
        height: button.getBoundingClientRect().height,
      };
    }
    throw new Error("Account value text is missing");
  });
  await page.goto(story());
  await expect(title(page)).toHaveValue("Connect the task table");
  const values = await detail(page)
    .getByRole("region", { name: "Task information", exact: true })
    .locator("dl > div")
    .evaluateAll((rows) =>
      rows
        .filter((row) =>
          ["Status", "Task ID"].includes(
            row.querySelector("dt")!.textContent!.trim(),
          ),
        )
        .map((row) => {
          const label = row.querySelector("dt")!,
            value = row.querySelector("dd")!;
          const walker = document.createTreeWalker(value, NodeFilter.SHOW_TEXT);
          let node: Node | null;
          while ((node = walker.nextNode())) {
            if (!node.textContent?.trim()) continue;
            const range = document.createRange();
            range.selectNodeContents(node);
            return {
              label: label.textContent!.trim(),
              gap:
                range.getBoundingClientRect().top -
                label.getBoundingClientRect().bottom,
              height: value.getBoundingClientRect().height,
            };
          }
          throw new Error("Task value text is missing");
        }),
    );
  expect(values.map((value) => value.label)).toEqual(["Status", "Task ID"]);
  for (const value of values) {
    expect(value.gap).toBeCloseTo(reference.gap, 1);
    expect(value.height).toBe(reference.height);
  }
  await expect(
    detail(page).getByRole("button", { name: "Copy: Task ID", exact: true }),
  ).toBeVisible();
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
  ).toContainText("Working");
  const priority = detail(page).getByRole("combobox", {
    name: "Priority",
    exact: true,
  });
  const trigger = (await priority.boundingBox())!;
  const assigned = (await detail(page)
    .getByRole("group", { name: "Assigned session controls", exact: true })
    .boundingBox())!;
  expect(trigger.height).toBe(assigned.height);
  await expect(priority.locator('svg[data-priority="2"]')).toBeVisible();
  await priority.click();
  const selectedOption = page.getByRole("option", { selected: true });
  await expect(selectedOption).toBeVisible();
  const selectedGeometry = await selectedOption.evaluate((option) => ({
    selected: option.getBoundingClientRect().width,
    content: option
      .closest('[data-scroll-area="content"]')!
      .getBoundingClientRect().width,
    height: option.getBoundingClientRect().height,
  }));
  expect(selectedGeometry.selected).toBeCloseTo(selectedGeometry.content, 1);
  expect(selectedGeometry.height).toBe(trigger.height);
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
  ).toContainText("Working");
  await expect(priority).toHaveText("High");
  await wrap.click();
  await expect(description).toHaveAttribute("wrap", "soft");
  await expect(description).toHaveCSS("word-break", "keep-all");
  await expect(save).toBeDisabled();
});

test("Done is read-only while Working keeps its assignment locked", async ({
  page,
}) => {
  await page.goto(story());
  await expect(title(page)).toHaveValue("Connect the task table");
  await expect(
    detail(page).getByRole("button", { name: "Release", exact: true }),
  ).toBeDisabled();
  await expect(title(page)).toHaveJSProperty("readOnly", false);
  await expect(
    detail(page).getByRole("combobox", { name: "Priority", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Back", exact: true }).click();
  await page
    .getByRole("link", {
      name: "Open task: Verify claim concurrency",
      exact: true,
    })
    .click();
  await expect(
    detail(page).getByLabel("Status", { exact: true }),
  ).toContainText("Done");
  await expect(title(page)).toHaveAttribute("aria-readonly", "true");
  for (const label of ["Alias", "Description"])
    await expect(
      detail(page).getByRole("textbox", { name: label, exact: true }),
    ).toHaveJSProperty("readOnly", true);
  await expect(
    detail(page).getByRole("combobox", { name: "Priority", exact: true }),
  ).toBeDisabled();
  for (const label of ["Release", "Cancel", "Save", "Discard changes"])
    await expect(
      detail(page).getByRole("button", { name: label, exact: true }),
    ).toBeDisabled();
  await title(page).focus();
  await page.keyboard.type("Cannot edit Done");
  await expect(title(page)).toHaveText("Verify claim concurrency");
  await detail(page)
    .getByRole("button", { name: "Show control characters", exact: true })
    .click();
  await expect(
    detail(page).locator("[data-control-character-overlay]"),
  ).toBeVisible();
  await expect(
    detail(page).getByRole("button", { name: "Copy: Task ID", exact: true }),
  ).toBeEnabled();
});

for (const width of [1280, 390]) {
  test(`readonly long titles keep their line breaks and copy full text at ${width}px`, async ({
    page,
    context,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(story("read-only-long-title"));
    const field = title(page);
    await expect(field).toHaveAttribute("aria-readonly", "true");
    await expect(field).toHaveText(
      /^Verify that a completed task title remains stable/,
    );
    await page.evaluate(() => document.fonts.ready);
    await field.scrollIntoViewIfNeeded();
    const value = await field.textContent();
    const text = field.locator("span");
    await expect(text).toHaveCSS("-webkit-line-clamp", "2");
    expect(
      await text.evaluate((node) => node.scrollHeight > node.clientHeight),
    ).toBe(true);
    const layout = () =>
      field.evaluate((node) => {
        const frame = node.getBoundingClientRect();
        const range = document.createRange();
        range.selectNodeContents(node);
        const rounded = (value: number) => Math.round(value * 10) / 10;
        return {
          height: rounded(frame.height),
          width: rounded(frame.width),
          lines: [...range.getClientRects()].map((rect) => ({
            top: rounded(rect.top - frame.top),
            left: rounded(rect.left - frame.left),
            width: rounded(rect.width),
            height: rounded(rect.height),
          })),
        };
      });
    const before = await layout();
    await field.focus();
    await expect(field).toBeFocused();
    expect(await layout()).toEqual(before);
    const word = await text.evaluate((node) => {
      const range = document.createRange();
      range.setStart(node.firstChild!, 0);
      range.setEnd(node.firstChild!, 6);
      const rect = range.getBoundingClientRect();
      return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
    });
    await page.mouse.dblclick(word.x, word.y);
    await expect
      .poll(() => page.evaluate(() => window.getSelection()?.toString()))
      .toBe("Verify");
    expect(await layout()).toEqual(before);
    await field.focus();
    await page.keyboard.press("Control+a");
    await expect
      .poll(() => page.evaluate(() => window.getSelection()?.toString()))
      .toBe(value);
    await context.grantPermissions(["clipboard-read", "clipboard-write"], {
      origin: new URL(page.url()).origin,
    });
    await page.keyboard.press("Control+c");
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(value);
    expect(await layout()).toEqual(before);
    await page.keyboard.type("Cannot replace this title");
    await expect(field).toHaveText(value!);
    const description = detail(page).getByRole("textbox", {
      name: "Description",
      exact: true,
    });
    await expect(description).toHaveJSProperty("readOnly", true);
    const original = await description.inputValue();
    await description.focus();
    await expect(description).toHaveValue(original);
  });
}

test("cancel requires confirmation, discards unsaved edits and preserves assignment", async ({
  page,
}) => {
  await page.goto(story());
  await expect(title(page)).toHaveValue("Connect the task table");
  const status = detail(page).getByLabel("Status", { exact: true });
  const cancel = detail(page).getByRole("button", {
    name: "Cancel",
    exact: true,
  });
  await title(page).fill("Unsaved draft");
  await cancel.click();
  const confirmation = page.getByRole("dialog", {
    name: "Cancel task?",
    exact: true,
  });
  await expect(confirmation).toContainText("Unsaved edits will be discarded");
  await confirmation
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await expect(status).toContainText("Working");
  await expect(title(page)).toHaveValue("Unsaved draft");
  await cancel.click();
  await confirmation
    .getByRole("button", { name: "Cancel task", exact: true })
    .click();
  await expect(confirmation).toHaveCount(0);
  await expect(status).toContainText("Canceled");
  await expect(title(page)).toHaveValue("Connect the task table");
  await expect(cancel).toBeDisabled();
  await expect(
    detail(page).getByRole("link", { name: /UI refinement/ }),
  ).toBeVisible();
  await expect(
    detail(page).getByRole("button", { name: "Release", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Back", exact: true }).click();
  await page
    .getByRole("link", {
      name: "Open task: Connect the task table",
      exact: true,
    })
    .click();
  await expect(status).toContainText("Canceled");
});

test("cancel refuses stale versions and preserves the draft for reload", async ({
  page,
}) => {
  await page.goto(story());
  await expect(title(page)).toHaveValue("Connect the task table");
  await page.getByRole("button", { name: "Back", exact: true }).click();
  await page
    .getByRole("link", {
      name: "Open task: Implement Task MCP tools",
      exact: true,
    })
    .click();
  await title(page).fill("Keep this draft");
  await page
    .getByRole("button", { name: "Simulate agent update", exact: true })
    .click();
  await expect(
    detail(page).getByText(
      "This task was updated elsewhere. Reload to edit its latest version.",
    ),
  ).toBeVisible();
  await detail(page)
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  const confirmation = page.getByRole("dialog", {
    name: "Cancel task?",
    exact: true,
  });
  await confirmation
    .getByRole("button", { name: "Cancel task", exact: true })
    .click();
  await expect(confirmation.getByRole("alert")).toContainText(
    "Your edits are preserved",
  );
  await expect(title(page)).toHaveValue("Keep this draft");
  await confirmation
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await detail(page)
    .getByRole("button", { name: "Reload latest task", exact: true })
    .click();
  await expect(title(page)).toHaveValue("Agent updated the task");
  await expect(
    detail(page).getByLabel("Status", { exact: true }),
  ).toContainText("Working");
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
  for (const label of ["Release", "Cancel", "Save", "Discard changes"]) {
    const button = actions.getByRole("button", { name: label, exact: true });
    await expect(button).toHaveAttribute("data-section-action", "button");
    await expect(button).toHaveCSS("border-radius", "16px");
    const bounds = (await button.boundingBox())!;
    expect(bounds.width).toBe(64);
    expect(bounds.height).toBe(64);
    await expect(button.locator("[data-icon]")).toHaveCount(1);
  }
  await expect(detail(page).locator(".floating-card")).toHaveCount(0);
  const { infoBounds, titleBounds, actionsBounds, historyBounds } =
    await detail(page).evaluate((root) => {
      const bounds = (label: string) => {
        const rect = root
          .querySelector(`[aria-label="${label}"]`)!
          .getBoundingClientRect();
        return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
      };
      return {
        infoBounds: bounds("Task information"),
        titleBounds: bounds("Task title"),
        actionsBounds: bounds("Task actions"),
        historyBounds: bounds("Task change history"),
      };
    });
  expect(infoBounds.x).toBeGreaterThanOrEqual(
    titleBounds.x + titleBounds.width,
  );
  expect(titleBounds.width).toBeGreaterThan(infoBounds.width);
  expect(actionsBounds.x).toBeCloseTo(infoBounds.x);
  expect(actionsBounds.width).toBeCloseTo(infoBounds.width);
  expect(actionsBounds.y + actionsBounds.height).toBeLessThanOrEqual(
    infoBounds.y,
  );
  expect(historyBounds.y).toBeGreaterThanOrEqual(
    infoBounds.y + infoBounds.height,
  );
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
