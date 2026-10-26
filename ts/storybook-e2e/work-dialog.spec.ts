import { expect, test } from "@playwright/test";

test("all work kinds live in the task dialog while important errors remain readable in the transcript", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-workdialog--mixed-activity&viewMode=story",
  );
  const transcript = page.locator(".transcript");
  const summary = transcript.locator("[data-work-summary]");
  await expect(summary).toHaveCount(1);
  await expect(summary.getByRole("button")).toHaveText("004");
  await expect(transcript.locator(".tool-activity")).toHaveCount(0);
  await expect(transcript).not.toContainText("Context compacted");
  await expect(transcript).not.toContainText("Agent update applied");
  await expect(transcript).not.toContainText("Provider background work");
  const error = transcript.locator('.event-detail[data-seq="8"]');
  await expect(error).toContainText(
    "The workspace connection was interrupted.",
  );
  await error.hover();
  await expect(error.locator(":scope > .button-content")).toHaveCSS(
    "opacity",
    "1",
  );
  await expect(page.locator(".activity-card-preview")).toHaveCount(0);
  await summary.getByRole("button").click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await expect(dialog).toContainText("Context compacted");
  await expect(dialog.locator('[data-dialog-trigger="true"]')).toHaveCount(0);
  await expect(dialog).toContainText("Agent update applied");
  await expect(dialog).toContainText("Provider background work");
  const work = dialog.locator('button[data-seq="7"]');
  await work.hover();
  await expect(work.locator(":scope > .button-content")).toHaveCSS(
    "opacity",
    "1",
  );
  await work.click();
  await expect(
    dialog.getByRole("tab", { name: "Result", exact: true }),
  ).toBeVisible();
  await dialog.getByRole("tab", { name: "Result", exact: true }).click();
  await expect(dialog.getByRole("tabpanel")).toContainText('"status": "done"');
  expect(errors).toEqual([]);
});

test("turn work collapses into one line and its dialog never covers adjacent panes", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-workdialog--contained&viewMode=story",
  );
  const summaries = page.locator("[data-work-summary]");
  await expect(summaries).toHaveCount(2);
  await expect(summaries.first().getByRole("button")).toHaveText("004");
  const icon = summaries.first().getByRole("button").locator("svg");
  await expect(icon).toBeVisible();
  await expect(icon.locator("path")).toBeVisible();
  await expect(icon).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  expect(await icon.evaluate((node) => node.getAttribute("fill"))).toBe(
    "currentColor",
  );
  const count = summaries.first().locator(".button-content > span");
  expect((await count.boundingBox())!.x).toBeGreaterThan(
    (await icon.boundingBox())!.x,
  );
  expect(
    (await summaries.first().getByRole("button").boundingBox())!.width,
  ).toBe((await summaries.nth(1).getByRole("button").boundingBox())!.width);
  const tooltip = summaries
    .first()
    .getByRole("button")
    .locator("..")
    .getByRole("tooltip");
  await expect(tooltip).toBeHidden();
  await summaries.first().getByRole("button").hover();
  await expect(tooltip).toContainText("4 tasks performed · View details");
  await expect(tooltip).toBeVisible();
  await expect(page.locator(".transcript .tool-activity")).toHaveCount(0);
  const pane = page.locator(".transcript");
  const before = await pane.evaluate((node) => ({
    top: node.scrollTop,
    height: node.scrollHeight,
  }));
  await summaries.first().getByRole("button").click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await expect(dialog.locator(".tool-activity")).toHaveCount(4);
  await dialog.evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished),
    );
  });
  const bounds = (await dialog.locator(".floating-card").boundingBox())!;
  const area = (await page.locator(".conversation").boundingBox())!;
  const panel = (await page
    .getByRole("complementary", { name: "Sessions panel", exact: true })
    .boundingBox())!;
  const files = (await page
    .getByRole("complementary", { name: "File explorer", exact: true })
    .boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(panel.x + panel.width);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(files.x);
  expect(area.x + area.width - bounds.x - bounds.width).toBeGreaterThan(0);
  expect(bounds.y - area.y).toBeGreaterThan(0);
  expect(area.y + area.height - bounds.y - bounds.height).toBeGreaterThan(0);
  expect(bounds.height / area.height).toBeGreaterThan(0.9);
  expect(
    await pane.evaluate((node) => ({
      top: node.scrollTop,
      height: node.scrollHeight,
    })),
  ).toEqual(before);
  await dialog
    .getByRole("button", { name: "Close task dialog", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(summaries.first().getByRole("button")).toBeFocused();
  await summaries.nth(1).getByRole("button").click();
  await expect(dialog.locator(".tool-activity")).toHaveCount(1);
  await dialog
    .getByRole("button", { name: "Close task dialog", exact: true })
    .press("Escape");
  await expect(dialog).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("long task lists remain virtualized and scroll independently", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-workdialog--long-turn&viewMode=story",
  );
  await expect(
    page.locator("[data-work-summary]").getByRole("button"),
  ).toHaveText("600");
  await page.getByRole("button", { name: "View details", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await expect(dialog.locator(".tool-activity").first()).toBeVisible();
  expect(await dialog.locator(".tool-activity").count()).toBeLessThan(40);
  const before = await page
    .locator(".transcript")
    .evaluate((node) => node.scrollTop);
  await dialog
    .getByRole("scrollbar", { name: "Panel scroll", exact: true })
    .focus();
  await page.keyboard.press("End");
  await expect(dialog.locator('.tool-activity[data-seq="1200"]')).toBeVisible();
  expect(await dialog.locator(".tool-activity").count()).toBeLessThan(40);
  expect(
    await page.locator(".transcript").evaluate((node) => node.scrollTop),
  ).toBe(before);
  await dialog
    .getByRole("button", { name: "Close task dialog", exact: true })
    .click();
  await page.setViewportSize({ width: 390, height: 620 });
  await page.getByRole("button", { name: "View details", exact: true }).click();
  await dialog.evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished),
    );
  });
  const bounds = (await dialog.locator(".floating-card").boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
});

test("paging earlier tasks preserves the open execution and its detail editor", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-workdialog--paged-turn&viewMode=story",
  );
  await expect(
    page.locator("[data-work-summary]").getByRole("button"),
  ).toHaveText("001+");
  const summary = page.locator("[data-work-summary]").getByRole("button");
  const summaryWidth = (await summary.boundingBox())!.width;
  const iconX = (await summary.locator("svg").boundingBox())!.x;
  await page.getByRole("button", { name: "View details", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Bash · Details", exact: true })
    .click();
  await dialog.getByRole("tab", { name: "Output", exact: true }).click();
  await expect(dialog.getByRole("tabpanel")).toContainText("main");
  await dialog
    .getByRole("button", { name: "Load earlier tasks", exact: true })
    .click();
  await expect(
    page.locator("[data-work-summary]").getByRole("button"),
  ).toHaveText("002");
  await expect(dialog.locator(".tool-activity")).toHaveCount(2);
  expect((await summary.boundingBox())!.width).toBe(summaryWidth);
  expect((await summary.locator("svg").boundingBox())!.x).toBe(iconX);
  await expect(dialog.getByRole("tabpanel")).toContainText("main");
  await expect(
    dialog.getByRole("button", { name: "Load earlier tasks", exact: true }),
  ).toHaveCount(0);
});

test("work batches split at assistant replies and summaries align right", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-workdialog--between-responses&viewMode=story",
  );
  const summaries = page.locator("[data-work-summary]");
  await expect(summaries).toHaveCount(2);
  await expect(summaries.first().getByRole("button")).toHaveText("001");
  await expect(summaries.nth(1).getByRole("button")).toHaveText("002");
  await expect(summaries.first().locator(".elapsed-time")).toHaveText(
    "00:00:06",
  );
  await expect(summaries.nth(1).locator(".elapsed-time")).toHaveText(
    "00:00:10",
  );
  expect(
    await summaries.first().evaluate((node) => {
      const time = node.querySelector(".elapsed-time")!.getBoundingClientRect();
      const button = node.querySelector("button")!.getBoundingClientRect();
      return time.right <= button.left;
    }),
  ).toBe(true);
  await expect(summaries.first()).toHaveCSS("justify-content", "flex-end");
  await summaries.first().getByRole("button").click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await expect(dialog.locator(".tool-activity")).toHaveCount(1);
  await dialog
    .getByRole("button", { name: "Close task dialog", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await summaries.nth(1).getByRole("button").click();
  await expect(dialog.locator(".tool-activity")).toHaveCount(2);
  expect(
    await dialog.evaluate(
      (node) => getComputedStyle(node, "::backdrop").backgroundColor,
    ),
  ).toBe("rgba(0, 0, 0, 0)");
  await expect(dialog.locator('[data-drawer="backdrop"]')).not.toHaveCSS(
    "background-color",
    "rgba(0, 0, 0, 0)",
  );
});

test("MCP counts share reply boundaries, align left and keep the total details control on the right", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(
    "/iframe.html?id=conversation-workdialog--memory-and-task-calls&viewMode=story",
  );
  const summaries = page.locator("[data-work-summary]");
  await expect(summaries).toHaveCount(2);
  for (const [index, memory, tasks, total] of [
    [0, 2, 1, 4],
    [1, 1, 2, 3],
  ]) {
    const summary = summaries.nth(index);
    await expect(
      summary.getByRole("img", {
        name: `Memory calls: ${memory}`,
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      summary.getByRole("img", { name: `Task calls: ${tasks}`, exact: true }),
    ).toBeVisible();
    await expect(
      summary.getByRole("button", { name: "View details", exact: true }),
    ).toHaveText(String(total).padStart(3, "0"));
  }
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const first = summaries.first();
    await first.scrollIntoViewIfNeeded();
    const bounds = await first.evaluate((el) => {
      const box = el.getBoundingClientRect();
      const memory = el
        .querySelector('[data-work-service="cxz_memory"]')!
        .getBoundingClientRect();
      const tasks = el
        .querySelector('[data-work-service="cxz_tasks"]')!
        .getBoundingClientRect();
      const duration = el
        .querySelector(".elapsed-time")!
        .getBoundingClientRect();
      const button = el.querySelector("button")!.getBoundingClientRect();
      return {
        left: box.left,
        right: box.right,
        memory: memory.toJSON(),
        tasks: tasks.toJSON(),
        duration: duration.toJSON(),
        button: button.toJSON(),
      };
    });
    expect(bounds.memory.x).toBeGreaterThanOrEqual(bounds.left);
    expect(bounds.memory.right).toBeLessThanOrEqual(bounds.tasks.x);
    expect(bounds.tasks.right).toBeLessThanOrEqual(bounds.duration.x);
    expect(bounds.duration.right).toBeLessThanOrEqual(bounds.button.x);
    expect(bounds.button.right).toBeLessThanOrEqual(bounds.right);
  }
  const memory = summaries
    .first()
    .getByRole("img", { name: "Memory calls: 2", exact: true });
  await memory.hover();
  await expect(memory.getByRole("tooltip")).toHaveText("cxz_memory · 2");
  await summaries.first().getByRole("button").click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
    exact: true,
  });
  await expect(dialog.locator(".tool-activity")).toHaveCount(4);
});
