import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("projects use the full workspace with a persistent project picker and scoped sessions", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-1");
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
    "Information",
  );
  await expect(
    main.getByRole("region", { name: "Session overview", exact: true }),
  ).toHaveCount(0);
  await expect(
    main.getByRole("region", { name: "Project tasks", exact: true }),
  ).toHaveCount(0);
  const information = main.getByRole("region", {
    name: "Information",
    exact: true,
  });
  const diagnostic = information.locator("dl > div");
  const firstDiagnostic = (await diagnostic.first().boundingBox())!;
  const secondDiagnostic = (await diagnostic.nth(1).boundingBox())!;
  expect(secondDiagnostic.y).toBe(firstDiagnostic.y);
  expect(secondDiagnostic.x).toBeGreaterThan(
    firstDiagnostic.x + firstDiagnostic.width,
  );
  await page.mouse.move(0, 0);
  expect(
    await main.locator(".prefix-value-menu").evaluate((el) => {
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
  const pageInset = await main
    .locator(
      '[data-sliding-page="detail"][data-active] > .panel-scroll-region > .panel-scroll-content > .panel-scroll-inner',
    )
    .evaluate((el) => parseFloat(getComputedStyle(el).paddingTop));
  const informationBounds = (await information.boundingBox())!;
  expect(informationBounds.y).toBe(toolbar.y + toolbar.height + pageInset);
  expect((await sessions.boundingBox())!.y).toBe(
    informationBounds.y + informationBounds.height + sectionGap,
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
  await expect(page).toHaveURL(new RegExp(`#/c/_/project/project-2$`));
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
  await expect(page).toHaveURL(new RegExp(`#/c/_/project/project-2$`));
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
  await page.goto("/sandbox.html#/c/_/project/project-1");
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
  await page.goto("/sandbox.html#/c/_/project/project-1");
  const header = page.locator('main > header[aria-label="Project toolbar"]');
  await expect(header).toBeVisible({ timeout: 45000 });
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Design playground");
  const before = await header.boundingBox();
  const viewport = page.locator(
    'main [data-sliding-page="detail"][data-active] > .panel-scroll-region > .panel-scroll-content',
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
  await page.goto("/sandbox.html#/c/_/project/project-1");
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  await expect(table).toBeVisible({ timeout: 45000 });
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(8);
  const first = rows.first(),
    second = rows.nth(1);
  const state = first.getByRole("img");
  await first.getByRole("button", { name: /Controls for/ }).click();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  await page
    .getByRole("dialog", { name: "Stop session", exact: true })
    .getByRole("button", { name: "Stop", exact: true })
    .click();
  await expect(state).toHaveAccessibleName("Stopped");
  await first.getByRole("button", { name: /Controls for/ }).click();
  await page.getByRole("menuitem", { name: "Resume", exact: true }).click();
  await expect(state).toHaveAccessibleName("Idle");

  await first.getByRole("checkbox").check();
  await second.getByRole("checkbox").check();
  const actions = page.getByRole("button", {
    name: "Selected session actions",
    exact: true,
  });
  await actions.click();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  const stop = page.getByRole("dialog", { name: "Stop sessions", exact: true });
  await expect(stop).toContainText("2 sessions will be affected.");
  await stop.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(first.getByRole("img")).toHaveAccessibleName("Stopped");
  await expect(second.getByRole("img")).toHaveAccessibleName("Stopped");
  await expect(first.getByRole("checkbox")).not.toBeChecked();
  await expect(second.getByRole("checkbox")).not.toBeChecked();
  await expect(actions).toBeEnabled();

  await first.getByRole("checkbox").check();
  await second.getByRole("checkbox").check();
  await actions.click();
  await page.getByRole("menuitem", { name: "Erase", exact: true }).click();
  const erase = page.getByRole("dialog", {
    name: "Erase sessions",
    exact: true,
  });
  await expect(erase).toContainText("This cannot be undone.");
  await erase.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(rows).toHaveCount(8);
  await actions.click();
  await page.getByRole("menuitem", { name: "Erase", exact: true }).click();
  await erase.getByRole("button", { name: "Erase", exact: true }).click();
  await expect(erase).toHaveCount(0);
  await expect(rows).toHaveCount(6);
  await expect(table).not.toContainText("Project checklist");
});

test("project session identities use stacked navigation buttons in the sandbox", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-1");
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  await expect(table).toBeVisible({ timeout: 45000 });
  const row = table.locator("tbody tr").first();
  await expect(table.locator(".editable-value-trigger")).toHaveCount(0);
  const session = row.locator('a[href*="/session/"]');
  await expect(session).toBeVisible();
  const href = (await session.getAttribute("href"))!;
  const target = href.slice(href.indexOf("/c/"));
  await row.getByRole("checkbox").check();
  await expect(row.getByRole("checkbox")).toBeChecked();
  await session.click();
  await expect(page).toHaveURL((url) => url.hash.endsWith(target));
  await expect(page.locator("form.composer")).toBeVisible();
});

test("project Session add opens the shared creation dialog scoped to the selected project", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-2");
  const main = page.getByRole("main", { name: "Project workspace" });
  const actions = main.getByRole("button", {
    name: "Selected session actions",
    exact: true,
  });
  await expect(actions).toBeEnabled({ timeout: 45000 });
  await actions.click();
  await expect(
    page.getByRole("menuitem", { name: "Stop", exact: true }),
  ).toBeDisabled();
  const add = page.getByRole("menuitem", { name: "Add session", exact: true });
  await expect(add).toBeEnabled();
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
  await expect(actions).toBeFocused();
  await expect(page).toHaveURL(/\/c\/_\/project\/project-2$/);
});

test("project metadata persists through Project.Patch and inventory Watch", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-1");
  const section = page.getByRole("region", {
    name: "Information",
    exact: true,
  });
  await expect(section).toBeVisible({ timeout: 45000 });
  for (const [label, value] of [
    ["Name", "Renamed sandbox"],
    ["Alias", "sandbox-edited"],
    ["Description", "Sandbox description"],
  ] as const) {
    await section
      .getByRole("button", { name: `Edit ${label}`, exact: true })
      .click();
    const dialog = page.getByRole("dialog", {
      name: `Edit ${label}`,
      exact: true,
    });
    await dialog.getByRole("textbox").fill(value);
    await dialog.getByRole("textbox").press("Enter");
    await expect(dialog).toHaveCount(0);
    await expect(
      section.getByRole("button", { name: `Edit ${label}`, exact: true }),
    ).toHaveText(value);
  }
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await picker.click();
  await page
    .getByRole("option", { name: "Secondary project", exact: true })
    .click();
  await picker.click();
  await page
    .getByRole("option", { name: "Renamed sandbox", exact: true })
    .click();
  await expect(
    section.getByRole("button", { name: "Edit Alias", exact: true }),
  ).toHaveText("sandbox-edited");
  await expect(
    section.getByRole("button", { name: "Edit Description", exact: true }),
  ).toHaveText("Sandbox description");
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(8);
  await expect(page).toHaveURL(new RegExp(`/c/_/project/sandbox-edited$`));
});

test("project Erase removes only the confirmed sandbox project", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-1");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  await expect(actions).toBeVisible({ timeout: 45000 });
  await actions.getByRole("button", { name: "Erase", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Erase project",
    exact: true,
  });
  await expect(dialog).toContainText("This cannot be undone.");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  await expect(table.locator("tbody tr")).toHaveCount(8);
  await actions.getByRole("button", { name: "Erase", exact: true }).click();
  await dialog
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("Secondary project");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await expect(page).toHaveURL(new RegExp(`/c/_/project/project-2$`));
  await picker.click();
  await expect(
    page.getByRole("option", { name: "Design playground", exact: true }),
  ).toHaveCount(0);
});

test("project Stop retains the sandbox project and conversation records", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-1");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  const stop = actions.getByRole("button", { name: "Stop", exact: true });
  await expect(stop).toBeEnabled({ timeout: 45000 });
  await stop.click();
  const dialog = page.getByRole("dialog", {
    name: "Stop project",
    exact: true,
  });
  await expect(dialog).toContainText("The project remains listed");
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(stop).toBeDisabled();
  const rows = page
    .getByRole("table", { name: "Sessions", exact: true })
    .locator("tbody tr");
  await expect(rows).toHaveCount(8);
  await expect
    .poll(() =>
      rows
        .getByRole("img")
        .evaluateAll((nodes) =>
          nodes.map((node) => node.getAttribute("aria-label")),
        ),
    )
    .toEqual(Array(8).fill("Stopped"));
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("Design playground");
  await picker.click();
  await page
    .getByRole("option", { name: "Secondary project", exact: true })
    .click();
  await expect(stop).toBeEnabled();
  await picker.click();
  await page
    .getByRole("option", { name: "Design playground", exact: true })
    .click();
  await expect(
    actions.getByRole("button", { name: "Stop", exact: true }),
  ).toBeDisabled();
});

// The buttons say what they call, and what they call decides which of them is
// available: Stop while it runs, Start once it is stopped, Remove while there
// is a container. See docs/plans/project-lifecycle-api.md.
test("project Stop, Start and Remove move the container and keep the project", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-1");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  const button = (name: string) =>
    actions.getByRole("button", { name, exact: true });
  const confirm = async (action: string, dialogName: string) => {
    await button(action).click();
    const dialog = page.getByRole("dialog", { name: dialogName, exact: true });
    await dialog.getByRole("button", { name: action, exact: true }).click();
    await expect(dialog).toHaveCount(0);
  };

  await expect(button("Stop")).toBeEnabled({ timeout: 45000 });
  await expect(button("Start")).toBeDisabled();
  await expect(button("Remove")).toBeEnabled();

  await confirm("Stop", "Stop project");
  await expect(button("Remove")).toBeEnabled();
  await expect(button("Stop")).toBeDisabled();
  await expect(button("Start")).toBeEnabled();

  // Stopped containers are hidden from the session panel too.
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  const panel = page.getByRole("complementary", { name: "Session list" });
  const group = panel.getByRole("region", {
    name: "Design playground",
    exact: true,
  });
  await expect(group).toHaveCount(0);
  await page.getByRole("link", { name: "Projects view", exact: true }).click();

  await confirm("Start", "Start project");
  await expect(button("Stop")).toBeEnabled();
  await expect(button("Start")).toBeDisabled();
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(group).toBeVisible();
  await page.getByRole("link", { name: "Projects view", exact: true }).click();

  await confirm("Remove", "Remove container");
  await expect(button("Stop")).toBeDisabled();
  await expect(button("Remove")).toBeDisabled();
  // Start is what makes a project run, so it is offered with no container as
  // well: it builds one. The sandbox answers it the same way either way.
  await expect(button("Start")).toBeEnabled();

  // The project and its sessions are still there; only the container went.
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Design playground");
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(8);

  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(group).toHaveCount(0);
  await expect(
    panel.getByRole("region", {
      name: "Secondary project",
      exact: true,
    }),
  ).toBeVisible();

  // Rebuilding the container publishes an inventory update and restores the
  // group, including its retained sessions, without reloading the workspace.
  await page.getByRole("link", { name: "Projects view", exact: true }).click();
  await confirm("Start", "Start project");
  await expect(button("Stop")).toBeEnabled();
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(group).toBeVisible();
  await expect(group.locator(".tree-session")).toHaveCount(8);
});
