import { expect, test } from "@playwright/test";

for (const resource of ["tasks", "memories"] as const) {
  test(`${resource} search overlays the page, navigates results and restores focus`, async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.setViewportSize({ width: 1440, height: 1000 });
    const story =
      resource === "tasks"
        ? "resources-tasks--project-table"
        : "pages-memory--default";
    await page.goto(`/iframe.html?id=${story}&viewMode=story`);
    const label = `Search ${resource}`;
    const trigger = page.getByRole("button", { name: label, exact: true });
    await expect(trigger).toBeEnabled();
    const before = await trigger.boundingBox();
    await trigger.click();
    const overlay = page.getByRole("search", { name: label, exact: true });
    const input = overlay.getByRole("searchbox", { name: label, exact: true });
    await expect(input).toBeFocused();
    expect(await trigger.boundingBox()).toEqual(before);
    await input.fill(resource === "tasks" ? "task" : "notes");
    const hits = overlay.locator("[data-search-index]");
    await expect(hits.first()).toBeVisible();
    await expect(overlay.locator("mark").first()).toBeVisible();
    if (resource === "memories")
      await expect(hits.filter({ hasText: "API notes" })).toHaveCount(0);
    await page.mouse.click(10, 200);
    await expect(overlay).toBeVisible();
    await input.focus();
    await page.keyboard.press("ArrowDown");
    await expect(hits.nth(1)).toHaveAttribute("aria-pressed", "true");
    await page.keyboard.press("Enter");
    await expect(
      page.getByRole("region", {
        name: resource === "tasks" ? "Task details" : "Memory document",
        exact: true,
      }),
    ).toBeVisible();
    await expect(overlay).toBeVisible();
    await expect(input).toBeFocused();
    await overlay
      .getByRole("button", { name: "Close search", exact: true })
      .click();
    await expect(overlay).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await page.keyboard.press("Shift+F");
    await expect(input).toBeFocused();
    await input.fill("prefix");
    await page.keyboard.press("Shift+F");
    await expect(input).toHaveValue("prefixF");
    await page.keyboard.press("Control+Shift+F");
    await expect(input).toHaveValue("prefixF");
    expect(
      await input.evaluate((node) => [
        (node as HTMLInputElement).selectionStart,
        (node as HTMLInputElement).selectionEnd,
      ]),
    ).toEqual([0, 7]);
    await page.keyboard.press("Escape");
    await expect(overlay).toHaveCount(0);
    await expect(trigger).toBeFocused();
    expect(errors).toEqual([]);
  });
}

test("task search includes cached older completions and reports invalid regex without closing", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-tasks--completed-history&viewMode=story",
  );
  const trigger = page.getByRole("button", {
    name: "Search tasks",
    exact: true,
  });
  await expect(
    page.getByRole("table", { name: "Tasks", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", {
      name: "Open task: Completed task 125",
      exact: true,
    }),
  ).toHaveCount(0);
  await trigger.click();
  const overlay = page.getByRole("search", {
    name: "Search tasks",
    exact: true,
  });
  const input = overlay.getByRole("searchbox");
  await input.fill("Completed task 125");
  await expect(overlay.locator("[data-search-index]")).toHaveCount(1);
  await input.press("Enter");
  await expect(
    page.getByRole("region", { name: "Task details", exact: true }),
  ).toContainText("Completed task 125");
  await expect(overlay).toBeVisible();
  await overlay
    .getByRole("button", { name: "Use regular expression", exact: true })
    .click();
  await input.fill("[");
  await expect(overlay.getByRole("alert")).toHaveText(
    "Invalid regular expression",
  );
  await input.fill("completed task 125");
  await expect(overlay.locator("[data-search-index]")).toHaveCount(1);
  await overlay
    .getByRole("button", { name: "Match case", exact: true })
    .click();
  await expect(overlay.getByText("No matches", { exact: true })).toBeVisible();
  await input.fill("Completed task 125");
  await expect(overlay.locator("[data-search-index]")).toHaveCount(1);
});

test("search shortcuts do not steal text input or active modal focus", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-tasks--project-table&viewMode=story",
  );
  await page.getByRole("button", { name: "Add task", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add task", exact: true });
  const title = dialog.getByRole("textbox", { name: "Title", exact: true });
  await title.fill("Keep ");
  await title.press("Shift+F");
  await expect(title).toHaveValue("Keep F");
  await title.press("Control+Shift+F");
  await expect(title).toBeFocused();
  await expect(
    page.getByRole("search", { name: "Search tasks", exact: true }),
  ).toHaveCount(0);
});
