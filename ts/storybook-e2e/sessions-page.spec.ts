import { expect, test, type Locator } from "@playwright/test";

const story = "/iframe.html?id=pages-sessions--default&viewMode=story";
const table = (page: import("@playwright/test").Page) =>
  page.getByRole("table", { name: "Sessions", exact: true });

for (const width of [390, 1280]) {
  test(`Sessions overview reuses table controls without a navigation panel at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(story);
    await expect(table(page)).toBeVisible();
    await expect(table(page).locator("tbody tr")).toHaveCount(10);
    expect(
      await table(page).getByRole("columnheader").allTextContents(),
    ).toEqual(["", "", "Project", "Name", "Account", ""]);
    const firstRow = table(page).locator("tbody tr").first();
    const cells = firstRow.getByRole("cell");
    await expect(cells.nth(2).locator('a[href*="/project/"]')).toHaveCount(1);
    await expect(
      cells.nth(3).locator("[data-session-navigation] .agent-brand"),
    ).toHaveCount(1);
    await expect(cells.nth(4).locator('a[href*="/account/"]')).toHaveCount(1);
    await expect(cells.nth(4).locator(".agent-brand")).toHaveCount(0);
    await expect(
      cells.nth(3).locator('[data-icon="open-project"]'),
    ).toHaveCount(1);
    await expect(
      cells.nth(2).locator('[data-icon="open-external"]'),
    ).toHaveCount(1);
    await expect(
      cells.nth(4).locator('[data-icon="open-external"]'),
    ).toHaveCount(1);
    const geometry = await firstRow.evaluate((row) => {
      const identities = [...row.querySelectorAll("td")].slice(2, 5);
      return identities.map((cell) => {
        const column = cell.getBoundingClientRect();
        const button = cell.querySelector("a")!.getBoundingClientRect();
        return {
          columnWidth: column.width,
          buttonWidth: button.width,
          leftInset: button.left - column.left,
          rightInset: column.right - button.right,
        };
      });
    });
    expect(geometry[0]).toEqual(geometry[1]);
    expect(geometry[1]).toEqual(geometry[2]);
    expect(geometry[0]!.leftInset).toBe(geometry[0]!.rightInset);
    await expect(
      page.getByRole("complementary", { name: "Session list", exact: true }),
    ).toHaveCount(0);
    await expect(page.locator(".session-panel-slot")).toHaveAttribute(
      "inert",
      "",
    );
    expect(
      await page
        .locator(".session-panel-slot")
        .evaluate((node) => node.getBoundingClientRect().width),
    ).toBe(0);
    await table(page).getByRole("checkbox").nth(1).check();
    await table(page).getByRole("checkbox").nth(2).check();
    await table(page)
      .getByRole("button", { name: "Selected session actions", exact: true })
      .click();
    await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
    const confirmation = page.getByRole("dialog", {
      name: "Stop sessions",
      exact: true,
    });
    await expect(confirmation).toBeVisible();
    await confirmation
      .getByRole("button", { name: "Cancel", exact: true })
      .click();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
  });
}

test("inactive session cards share project tones and preserve hover, selection and resume feedback", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  const palettes = (row: Locator) =>
    row.locator("a[data-row-aware]").evaluateAll((nodes) =>
      nodes.map((node) => {
        const style = getComputedStyle(node);
        return { background: style.backgroundColor, color: style.color };
      }),
    );
  const settle = (row: Locator) =>
    row.evaluate(async (node) => {
      await Promise.all(
        node
          .getAnimations({ subtree: true })
          .filter((animation) => animation instanceof CSSTransition)
          .map((animation) => animation.finished.catch(() => {})),
      );
    });
  await page.goto("/iframe.html?id=pages-projects--list-states&viewMode=story");
  const project = page
    .getByRole("table", { name: "Projects", exact: true })
    .locator("tbody tr[data-inactive]")
    .first();
  await expect(project).toBeVisible();
  const reference = (await palettes(project))[0]!;

  await page.goto(story);
  const rows = table(page).locator("tbody tr");
  await expect(rows).toHaveCount(10);
  await expect(rows.locator("[data-session-navigation]")).toHaveCount(10);
  await expect(table(page).locator("tbody tr[data-inactive]")).toHaveCount(4);
  const normal = await palettes(rows.first());
  expect(normal[0]).not.toEqual(reference);
  for (const index of [2, 4, 7, 9]) {
    const row = rows.nth(index);
    await expect(row).toHaveAttribute("data-inactive", "true");
    expect(await palettes(row)).toEqual([reference, reference, reference]);
  }

  const stopped = rows.nth(2);
  await stopped.locator("td").last().hover();
  await settle(stopped);
  const hovered = await palettes(stopped);
  expect(
    hovered.every((tone) => tone.background !== reference.background),
  ).toBe(true);
  const checkbox = stopped.getByRole("checkbox");
  await checkbox.focus();
  await checkbox.press("Space");
  await expect(stopped).toHaveAttribute("data-selected", "true");
  await expect
    .poll(async () =>
      (await palettes(stopped)).every(
        (tone, index) => tone.background !== hovered[index]!.background,
      ),
    )
    .toBe(true);
  await checkbox.press("Space");
  await expect.poll(() => palettes(stopped)).toEqual(hovered);
  await stopped.locator("[data-session-navigation]").hover();
  await expect
    .poll(async () => (await palettes(stopped))[1]!.background)
    .not.toBe(hovered[1]!.background);

  await stopped.getByRole("button", { name: /Controls for/ }).click();
  await page.getByRole("menuitem", { name: "Resume", exact: true }).click();
  await expect(stopped).not.toHaveAttribute("data-inactive", "true");
  await expect(
    stopped.getByRole("img", { name: "Idle", exact: true }),
  ).toBeVisible();
  await page.mouse.move(0, 0);
  await settle(stopped);
  expect(await palettes(stopped)).toEqual(normal);
});

test("session navigation panel slides in with its reserved space and reverses on Back", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(story);
  await expect(table(page)).toBeVisible();
  const slot = page.locator(".session-panel-slot");
  await page
    .locator(".workspace")
    .evaluate((node) =>
      (node as HTMLElement).style.setProperty(
        "--session-panel-motion-duration",
        "1500ms",
      ),
    );
  await table(page).locator("[data-session-navigation]").first().click();
  await expect(page.locator(".session-workspace")).toBeVisible();
  await expect(slot).not.toHaveAttribute("inert", "");
  await expect
    .poll(() => slot.evaluate((node) => node.getBoundingClientRect().width))
    .toBeGreaterThan(0);
  const entering = await slot.evaluate(
    (node) => node.getBoundingClientRect().width,
  );
  expect(entering).toBeLessThan(180);
  await expect
    .poll(() => slot.evaluate((node) => node.getBoundingClientRect().width))
    .toBe(180);
  await expect(
    page.getByRole("complementary", { name: "Session list", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Latest response", { exact: true }),
  ).toBeVisible();
  const conversation = await page.locator(".session-workspace").boundingBox();
  expect(conversation!.x).toBe(244);
  await page
    .getByRole("navigation", { name: "Preview history" })
    .getByRole("button", { name: "Back", exact: true })
    .click();
  await expect(slot).toHaveAttribute("inert", "");
  await expect(table(page)).toBeVisible();
  await expect
    .poll(() => slot.evaluate((node) => node.getBoundingClientRect().width))
    .toBe(0);
  expect(errors).toEqual([]);
});

test("narrow conversations hide navigation and browser Back restores the overview", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 800 });
  await page.goto(story);
  await expect(table(page)).toBeVisible();
  await table(page).locator("[data-session-navigation]").first().click();
  await expect(page.locator(".session-workspace")).toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "Session list", exact: true }),
  ).toHaveCount(0);
  const bounds = await page.locator(".session-workspace").boundingBox();
  expect(bounds!.x).toBe(0);
  expect(bounds!.width).toBe(390);
  await page
    .getByRole("navigation", { name: "Preview history" })
    .getByRole("button", { name: "Back", exact: true })
    .click();
  await expect(table(page)).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});

for (const width of [390, 1280]) {
  test(`Sessions overview shares transparent table surfaces and sticks its header to page scrolling at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 600 });
    await page.goto("/iframe.html?id=pages-projects--list&viewMode=story");
    const projectTable = page.getByRole("table", {
      name: "Projects",
      exact: true,
    });
    await expect(projectTable).toBeVisible();
    const surface = await projectTable.evaluate((node) => ({
      table: getComputedStyle(node).backgroundColor,
      header: getComputedStyle(node.querySelector("thead")!).backgroundColor,
      blur: getComputedStyle(node.querySelector("thead")!).backdropFilter,
    }));

    await page.goto(
      "/iframe.html?id=pages-sessions--many-sessions&viewMode=story",
    );
    await expect(table(page).locator("tbody tr")).toHaveCount(50);
    await expect(table(page)).toHaveCSS("background-color", surface.table);
    const header = table(page).locator("thead");
    await expect(header).toHaveCSS("background-color", surface.header);
    await expect(header).toHaveCSS("backdrop-filter", surface.blur);
    await expect(header).toHaveCSS("position", "sticky");
    await expect(
      page
        .getByRole("region", { name: "Session list", exact: true })
        .locator(".panel-scroll-content"),
    ).toHaveCount(0);
    const viewport = page.locator(
      "[data-resource-page] > .panel-scroll-region > .panel-scroll-content",
    );
    await viewport.evaluate((node) => {
      node.scrollTop = 350;
    });
    await expect
      .poll(() => viewport.evaluate((node) => node.scrollTop))
      .toBe(350);
    expect(
      Math.abs(
        (await header.boundingBox())!.y - (await viewport.boundingBox())!.y,
      ),
    ).toBeLessThan(1);
    await table(page).locator("tbody tr").last().scrollIntoViewIfNeeded();
    await expect(table(page).locator("tbody tr").last()).toBeInViewport();
    expect(
      await page.evaluate(() => document.documentElement.scrollHeight),
    ).toBe(600);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
  });
}

test("reduced motion opens the panel without transitioning the workspace", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(story);
  await expect(table(page)).toBeVisible();
  await table(page).locator("[data-session-navigation]").first().click();
  await expect(page.locator(".workspace")).toHaveCSS(
    "transition-duration",
    "0s",
  );
  await expect(page.locator(".session-panel-slot > .session-panel")).toHaveCSS(
    "transition-duration",
    "0s",
  );
});

test("Sessions sort by conversation recency and preserve selection through live reordering", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(
    "/iframe.html?id=pages-sessions--recent-activity&viewMode=story",
  );
  const aliases = () =>
    table(page)
      .locator("tbody [data-session-navigation]")
      .evaluateAll((nodes) =>
        nodes.map((node) => node.getAttribute("href")!.split("/").at(-1)),
      );
  const expected = [
    "api-session-5",
    "api-session-4",
    "api-session-3",
    "api-elm",
    "api-oak",
    "web-session-5",
    "web-session-4",
    "web-session-3",
    "web-elm",
    "web-oak",
  ];
  await expect.poll(aliases).toEqual(expected);
  const oldest = table(page)
    .locator("tbody tr")
    .filter({
      has: page.locator('[data-session-navigation][href$="/web-oak"]'),
    });
  await oldest.getByRole("checkbox").check();
  await page
    .getByRole("button", { name: "Rename oldest session", exact: true })
    .click();
  await expect(oldest).toContainText("Renamed oldest conversation");
  expect(await aliases()).toEqual(expected);
  await page
    .getByRole("button", { name: "Record latest conversation", exact: true })
    .click();
  await expect.poll(aliases).toEqual(["web-oak", ...expected.slice(0, -1)]);
  await expect(oldest).toHaveAttribute("data-selected", "true");
  await expect(oldest.getByRole("checkbox")).toBeChecked();
});

test("unread completion dots share the Name button corner center and clear with read state", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-sessions--unread-completions&viewMode=story",
  );
  const names = table(page).locator("[data-session-navigation]");
  await expect(
    table(page).locator("[data-session-navigation][data-unread]"),
  ).toHaveCount(2);
  const first = names.filter({
    has: page.getByText("web-oak", { exact: true }),
  });
  await expect(first).toHaveAttribute("aria-description", "Unread response");
  const geometry = await first.evaluate((node) => {
    const button = getComputedStyle(node),
      dot = getComputedStyle(node, "::after");
    return {
      radius: parseFloat(button.borderTopRightRadius),
      top: parseFloat(dot.top) + parseFloat(dot.height) / 2,
      right: parseFloat(dot.right) + parseFloat(dot.width) / 2,
      content: dot.content,
      pointerEvents: dot.pointerEvents,
    };
  });
  expect(geometry.top).toBe(geometry.radius);
  expect(geometry.right).toBe(geometry.radius);
  expect(geometry.content).toBe('""');
  expect(geometry.pointerEvents).toBe("none");
  await page
    .getByRole("button", { name: "Mark first completion read", exact: true })
    .click();
  await expect(first).not.toHaveAttribute("data-unread");
  await expect(
    table(page).locator("[data-session-navigation][data-unread]"),
  ).toHaveCount(1);
});
