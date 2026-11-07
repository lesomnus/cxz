import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});
test("Tasks routes and versioned forms work against the real WASM Task service", async ({
  page,
}, testInfo) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/sandbox.html#/c/_/session/session-1");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  await page.getByRole("link", { name: "Tasks", exact: true }).click();
  await expect(page).toHaveURL(/#\/c\/_\/project\/[^/]+\/task/);
  await expect(page.getByRole("complementary")).toHaveCount(0);
  await expect(
    page.locator('header[aria-label="Task toolbar"] svg'),
  ).toHaveCount(3);
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page.getByRole("option", { name: "All sessions", exact: true }).click();
  await page.getByRole("button", { name: "Add task", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add task", exact: true });
  await add
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Verify WASM tasks");
  await add
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("wasm-tasks");
  await add.getByRole("button", { name: "Confirm", exact: true }).click();
  await page
    .getByRole("link", { name: "Open task: Verify WASM tasks", exact: true })
    .click();
  const detail = page.getByRole("region", {
    name: "Task details",
    exact: true,
  });
  const priority = detail.getByRole("combobox", {
    name: "Priority",
    exact: true,
  });
  await expect(
    detail.getByRole("status", { name: "Status", exact: true }),
  ).toHaveText("Todo");
  await expect(
    detail.getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveText("Verify WASM tasks");
  await detail.screenshot({ path: testInfo.outputPath("task-details.png") });
  for (const trigger of [priority]) {
    const anchor = (await trigger.locator(".meta-value").boundingBox())!;
    await trigger.click();
    const popup = page.locator(".setting-menu");
    await expect(popup).toHaveAttribute("data-item-alignment", "selected");
    const selected = popup.locator(
      '[aria-selected="true"] .setting-match-label',
    );
    await expect
      .poll(async () => {
        const box = (await selected.boundingBox())!;
        return Math.max(
          Math.abs(box.x - anchor.x),
          Math.abs(box.y + box.height / 2 - anchor.y - anchor.height / 2),
        );
      })
      .toBeLessThan(0.5);
    await page.keyboard.press("Escape");
  }
  await priority.click();
  await page.getByRole("option", { name: "High", exact: true }).click();
  await detail
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("WASM task verified");
  await detail.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    detail.getByRole("button", { name: "Save", exact: true }),
  ).toBeDisabled();
  await page.goBack();
  await expect(
    page.getByRole("link", {
      name: "Open task: WASM task verified",
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(input).toBeVisible();
  await page.getByRole("link", { name: "Tasks", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page.getByRole("option", { name: "All sessions", exact: true }).click();
  await expect(
    page.getByRole("link", {
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
  await page.goto("/sandbox.html#/c/_/project/project-2/task");
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
  await expect(page).toHaveURL(new RegExp(`#/c/_/project/project-1/task$`));
  await expect(picker).toHaveText("Design playground");
  const add = page.getByRole("button", { name: "Add task", exact: true });
  await expect(add).toBeEnabled();
  const bounds = (await add.boundingBox())!;
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});
