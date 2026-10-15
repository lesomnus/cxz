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
  const tasks = page.getByRole("link", { name: /^Project tasks:/ });
  await expect(tasks).toBeVisible();
  await tasks.click();
  await expect(page).toHaveURL(/#\/tasks\?/);
  await page
    .getByRole("button", { name: "Show all project tasks", exact: true })
    .click();
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
