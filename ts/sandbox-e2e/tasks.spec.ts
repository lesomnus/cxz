import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});
test("Tasks routes and versioned forms work against the real WASM Task service", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/sandbox.html#/sessions/session-1");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  await page.getByRole("link", { name: "Tasks", exact: true }).click();
  await expect(page).toHaveURL(/#\/tasks/);
  await expect(page.getByRole("complementary")).toHaveCount(0);
  await expect(
    page.locator('header[aria-label="Task toolbar"] svg'),
  ).toHaveCount(2);
  await page.getByRole("button", { name: "Add task", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add task", exact: true });
  await add
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Verify WASM tasks");
  await add
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("wasm-tasks");
  await add.getByRole("button", { name: "Add", exact: true }).click();
  await page
    .getByRole("button", { name: "Open task: Verify WASM tasks", exact: true })
    .click();
  const detail = page.getByRole("dialog", {
    name: "Task details",
    exact: true,
  });
  await detail
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("WASM task verified");
  await detail.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    page.getByRole("button", {
      name: "Open task: WASM task verified",
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(input).toBeVisible();
  await page.getByRole("link", { name: "Tasks", exact: true }).click();
  await expect(
    page.getByRole("button", {
      name: "Open task: WASM task verified",
      exact: true,
    }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("Tasks project toolbar switches projects and keeps Add accessible on mobile", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/sandbox.html#/tasks?project=project-2");
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("Secondary project", { timeout: 45000 });
  await expect(page.getByRole("complementary")).toHaveCount(0);
  await picker.click();
  await page
    .getByRole("option", { name: "Design playground", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/tasks\?project=project-1$/);
  await expect(picker).toHaveText("Design playground");
  const add = page.getByRole("button", { name: "Add task", exact: true });
  await expect(add).toBeEnabled();
  const bounds = (await add.boundingBox())!;
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});
