import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("projects use the full workspace with a persistent project picker and scoped sessions", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/projects");
  const main = page.getByRole("main", { name: "Project workspace" });
  await expect(main).toBeVisible({ timeout: 45000 });
  const picker = main.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(main.locator("header svg")).toBeVisible();
  await expect(main.locator("header svg")).toHaveCSS("width", "14px");
  await expect(
    page
      .getByRole("link", { name: "Projects view", exact: true })
      .locator("svg"),
  ).toHaveCSS("width", "20px");
  await expect(picker).toHaveText("Design playground");
  await expect(main.locator("section").first()).toHaveAttribute(
    "aria-label",
    "Project information",
  );
  await expect(
    main.getByRole("region", { name: "Session overview", exact: true }),
  ).toHaveCount(0);
  await expect(
    main.getByRole("region", { name: "Project tasks", exact: true }),
  ).toHaveCount(0);
  const diagnostic = main
    .getByRole("region", { name: "Diagnostics", exact: true })
    .locator("dl > div");
  const firstDiagnostic = (await diagnostic.first().boundingBox())!;
  const secondDiagnostic = (await diagnostic.nth(1).boundingBox())!;
  expect(secondDiagnostic.y).toBe(firstDiagnostic.y);
  expect(secondDiagnostic.x).toBeGreaterThan(
    firstDiagnostic.x + firstDiagnostic.width,
  );
  await page.mouse.move(0, 0);
  expect(
    await main.locator(".icon-value-menu").evaluate((el) => {
      const style = getComputedStyle(el, "::before");
      return { opacity: style.opacity, background: style.backgroundColor };
    }),
  ).toEqual({ opacity: "1", background: "rgb(32, 32, 32)" });
  await expect(page.getByRole("complementary")).toHaveCount(0);
  const sidebar = (await page
    .getByRole("navigation", { name: "Resources", exact: true })
    .boundingBox())!;
  expect((await main.boundingBox())!.x).toBe(sidebar.x + sidebar.width);
  const sessions = main.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const toolbar = (await main.locator(":scope > header").boundingBox())!;
  const sectionGap = await sessions
    .locator("..")
    .evaluate((el) => parseFloat(getComputedStyle(el).rowGap));
  expect((await sessions.boundingBox())!.y).toBe(
    toolbar.y + toolbar.height + sectionGap,
  );
  const cards = main
    .getByRole("region", { name: "Session list", exact: true })
    .locator("tbody tr");
  await expect(cards).toHaveCount(8);
  await picker.click();
  const popup = page.getByRole("group", {
    name: "Select project choices",
    exact: true,
  });
  const motion = await popup.evaluate((el) => ({
    duration: 1000 * parseFloat(getComputedStyle(el).animationDuration),
    animations: el
      .getAnimations({ subtree: true })
      .filter((animation) => animation instanceof CSSAnimation)
      .map((animation) => animation.effect!.getTiming().duration),
  }));
  expect(motion.animations).toEqual([motion.duration, motion.duration]);
  await page
    .getByRole("option", { name: "Secondary project", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/projects\?project=project-2$/);
  await expect(picker).toHaveText("Secondary project");
  await expect(cards).toHaveCount(1);
  await expect(cards).toContainText("session-8");
  await page.goBack();
  await expect(picker).toHaveText("Design playground");
  await page.goForward();
  await expect(picker).toHaveText("Secondary project");
  await page.reload();
  await expect(picker).toHaveText("Secondary project", { timeout: 45000 });
  await cards.getByRole("checkbox").focus();
  await cards.getByRole("checkbox").press("Space");
  await expect(page).toHaveURL(/#\/projects\?project=project-2$/);
  await expect(cards).toHaveAttribute("data-selected", "true");
  await expect(
    main.getByRole("checkbox", { name: "Select all sessions", exact: true }),
  ).toBeChecked();
  await expect(
    main.getByRole("region", { name: "Session details", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toHaveCount(0);
  await picker.click();
  await page
    .getByRole("option", { name: "Design playground", exact: true })
    .click();
  await expect(
    main.getByRole("region", { name: "Session details", exact: true }),
  ).toHaveCount(0);
});

test("project dropdown respects reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/sandbox.html#/projects");
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toBeEnabled({ timeout: 45000 });
  await picker.click();
  const popup = page.getByRole("group", {
    name: "Select project choices",
    exact: true,
  });
  await expect(popup).toBeVisible();
  expect(
    await popup.evaluate(
      (el) =>
        el
          .getAnimations({ subtree: true })
          .filter((animation) => animation instanceof CSSAnimation).length,
    ),
  ).toBe(0);
  await page
    .getByRole("option", { name: "Secondary project", exact: true })
    .click();
  await expect(picker).toHaveText("Secondary project");
});

test("project toolbar stays above the scrolling body on narrow screens", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 400 });
  await page.goto("/sandbox.html#/projects");
  const header = page.locator('main > header[aria-label="Project toolbar"]');
  await expect(header).toBeVisible({ timeout: 45000 });
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Design playground");
  const before = await header.boundingBox();
  const viewport = page.locator(
    "main > .panel-scroll-region > .panel-scroll-content",
  );
  await viewport.evaluate((el) => (el.scrollTop = el.scrollHeight));
  expect(await viewport.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  expect(await header.boundingBox()).toEqual(before);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});

test("project row and bulk controls use the sandbox session lifecycle", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/projects");
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  await expect(table).toBeVisible({ timeout: 45000 });
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(8);
  const first = rows.first(),
    second = rows.nth(1);
  const state = first.locator("td:nth-child(6)");
  await first.getByRole("button", { name: /Controls for/ }).click();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  await page
    .getByRole("dialog", { name: "Stop session", exact: true })
    .getByRole("button", { name: "Stop", exact: true })
    .click();
  await expect(state).toHaveText("Stopped");
  await first.getByRole("button", { name: /Controls for/ }).click();
  await page.getByRole("menuitem", { name: "Resume", exact: true }).click();
  await expect(state).toHaveText("Idle");

  await first.getByRole("checkbox").check();
  await second.getByRole("checkbox").check();
  const actions = page.getByRole("button", {
    name: /^\d+ selected$/,
    exact: true,
  });
  await actions.click();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  const stop = page.getByRole("dialog", { name: "Stop sessions", exact: true });
  await expect(stop).toContainText("2 sessions will be affected.");
  await stop.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(first.locator("td:nth-child(6)")).toHaveText("Stopped");
  await expect(second.locator("td:nth-child(6)")).toHaveText("Stopped");
  await expect(actions).toBeDisabled();

  await first.getByRole("checkbox").check();
  await second.getByRole("checkbox").check();
  await actions.click();
  await page.getByRole("menuitem", { name: "Purge", exact: true }).click();
  const purge = page.getByRole("dialog", {
    name: "Purge sessions",
    exact: true,
  });
  await expect(purge).toContainText("/sandbox/session-1/journal");
  await purge.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(rows).toHaveCount(8);
  await actions.click();
  await page.getByRole("menuitem", { name: "Purge", exact: true }).click();
  await purge
    .getByRole("button", { name: "Purge permanently", exact: true })
    .click();
  await expect(purge).toHaveCount(0);
  await expect(rows).toHaveCount(6);
  await expect(table).not.toContainText("Project checklist");
});

test("project session identity edits use Session.Patch in the sandbox", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/projects");
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  await expect(table).toBeVisible({ timeout: 45000 });
  const row = table.locator("tbody tr").first();
  await row.getByRole("checkbox").check();
  const before = await row.boundingBox();
  const editName = row.getByRole("button", { name: "Edit Name", exact: true });
  const originalAlias = await row
    .getByRole("button", { name: "Edit Alias", exact: true })
    .textContent();
  await editName.click();
  const dialog = page.getByRole("dialog", { name: "Edit Name", exact: true });
  const input = dialog.getByRole("textbox", { name: "Name", exact: true });
  await input.fill("Updated project session");
  await input.press("Enter");
  await expect(dialog).toHaveCount(0);
  await expect(editName).toHaveText("Updated project session");
  const editAlias = row.getByRole("button", {
    name: "Edit Alias",
    exact: true,
  });
  await expect(editAlias).toHaveText(originalAlias!);
  await editAlias.click();
  const aliasDialog = page.getByRole("dialog", {
    name: "Edit Alias",
    exact: true,
  });
  await aliasDialog
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("project-edited");
  await aliasDialog
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(aliasDialog).toHaveCount(0);
  await expect(editAlias).toHaveText("project-edited");
  await expect(editName).toHaveText("Updated project session");
  await expect(row.getByRole("checkbox")).toBeChecked();
  expect(await row.boundingBox()).toEqual(before);
});

test("project Session add opens the shared creation dialog scoped to the selected project", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/projects?project=project-2");
  const main = page.getByRole("main", { name: "Project workspace" });
  const add = main.getByRole("button", { name: "Add session", exact: true });
  await expect(add).toBeEnabled({ timeout: 45000 });
  await expect(
    main.getByRole("button", { name: "0 selected", exact: true }),
  ).toBeDisabled();
  await add.click();
  const dialog = page.getByRole("dialog", { name: "Add session", exact: true });
  await expect(
    dialog.getByRole("combobox", { name: "Project", exact: true }),
  ).toHaveText("Secondary project");
  await expect
    .poll(() =>
      dialog.evaluate((node) => node.contains(document.activeElement)),
    )
    .toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(add).toBeFocused();
  await expect(page).toHaveURL(/projects\?project=project-2$/);
});
