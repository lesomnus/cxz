import { expect, test, type Locator } from "@playwright/test";

test("project list stories show resource links, activity, states and empty/loading tables", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--list&viewMode=story");
  const table = page.getByRole("table", { name: "Projects", exact: true });
  await expect(table).toBeVisible();
  await expect(table.locator("tbody tr")).toHaveCount(2);
  await expect(
    table.getByRole("link", { name: "Web workspace web", exact: true }),
  ).toHaveAttribute("href", "/c/_/project/web");
  await expect(
    table
      .locator("tbody tr")
      .first()
      .getByRole("link", { name: "Plan the next release oak", exact: true }),
  ).toBeVisible();
  await expect(
    table
      .locator("tbody tr")
      .first()
      .getByRole("img", { name: "Working", exact: true }),
  ).toBeVisible();
  await expect(table.locator("time")).toHaveCount(2);
  await expect(table.getByRole("columnheader")).toHaveText([
    "",
    "",
    "Name",
    "Recent session",
    "",
    "",
  ]);
  await table
    .getByRole("link", { name: "API workspace api", exact: true })
    .click();
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("API workspace");
  await expect(
    page.getByRole("table", { name: "Sessions", exact: true }),
  ).toBeVisible();
  await page.goto(
    "/iframe.html?id=pages-projects--list-loading&viewMode=story",
  );
  await expect(table).toHaveAttribute("aria-busy", "true");
  await expect(table.getByRole("status")).toHaveText("Loading projects…");
  await page.goto("/iframe.html?id=pages-projects--list-empty&viewMode=story");
  await expect(table.getByRole("status")).toHaveText("No projects yet.");
  await expect(table.getByRole("columnheader")).toHaveCount(6);
});

test("project page stories render their selector and states", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(
    page.getByRole("table", { name: "Sessions", exact: true }),
  ).toBeVisible();
  await expect(picker).toHaveText("Web workspace");
  await page.goto("/iframe.html?id=pages-projects--empty&viewMode=story");
  await expect(picker).toBeDisabled();
  await expect(
    page.getByText("No projects yet.", { exact: true }),
  ).toBeVisible();
  await page.goto("/iframe.html?id=pages-projects--loading&viewMode=story");
  await expect(picker).toBeDisabled();
  await expect(
    page.locator("main > .panel-scroll-region > .panel-scroll-content"),
  ).toContainText("Loading projects…");
  expect(errors).toEqual([]);
});

test("project list checkboxes select rows without navigation and action buttons stay square", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--list&viewMode=story");
  const table = page.getByRole("table", { name: "Projects", exact: true });
  const rows = table.locator("tbody tr");
  const all = table.getByRole("checkbox", {
    name: "Select all projects",
    exact: true,
  });
  await expect(rows).toHaveCount(2);
  await rows
    .first()
    .locator("td")
    .first()
    .click({ position: { x: 2, y: 2 } });
  await expect(rows.first().getByRole("checkbox")).toBeChecked();
  await expect(rows.first()).toHaveAttribute("data-selected", "true");
  await expect(all).toBeChecked({ indeterminate: true });
  await rows.first().getByRole("checkbox").press("Space");
  await expect(all).not.toBeChecked();
  await table
    .getByRole("columnheader")
    .first()
    .click({ position: { x: 2, y: 2 } });
  await expect(all).toBeChecked();
  for (const row of await rows.all())
    await expect(row.getByRole("checkbox")).toBeChecked();
  await all.uncheck();
  const more = rows
    .first()
    .getByRole("button", { name: "Controls for Web workspace", exact: true });
  const bounds = (await more.boundingBox())!;
  const cell = (await rows.first().locator("td").last().boundingBox())!;
  expect(Math.abs(bounds.width - bounds.height)).toBeLessThan(0.1);
  expect(
    Math.abs(
      bounds.y - cell.y - (cell.y + cell.height - bounds.y - bounds.height),
    ),
  ).toBeLessThan(0.1);
  await more.click();
  const menu = page.getByRole("menu", {
    name: "Controls for Web workspace",
    exact: true,
  });
  await expect(menu.getByRole("menuitem")).toHaveText([
    "Stop",
    "Start",
    "Remove",
    "Erase",
  ]);
  await menu.getByRole("menuitem", { name: "Stop", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Stop project",
    exact: true,
  });
  await expect(dialog).toContainText("Web workspace");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(more).toBeFocused();
  await expect(table).toBeVisible();
});

test("project index checkboxes toggle on down, drag, and Shift without release toggling", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--list&viewMode=story");
  const table = page.getByRole("table", { name: "Projects", exact: true }),
    rows = table.locator("tbody tr");
  const first = rows.first().getByRole("checkbox"),
    second = rows.last().getByRole("checkbox");
  const move = async (target: Locator) => {
    const box = (await target.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  };
  await move(first);
  await page.mouse.down();
  await expect(first).toBeChecked();
  await move(second);
  await expect(second).toBeChecked();
  await page.mouse.up();
  await expect(first).toBeChecked();
  await expect(second).toBeChecked();
  const all = table.getByRole("checkbox", {
    name: "Select all projects",
    exact: true,
  });
  await move(all);
  await page.mouse.down();
  await expect(first).not.toBeChecked();
  await expect(second).not.toBeChecked();
  await page.mouse.up();
  await first.click();
  await second.click({ modifiers: ["Shift"] });
  await expect(first).toBeChecked();
  await expect(second).toBeChecked();
});

test("project session table uses the reusable down/drag/Shift selection", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--long-sessions&viewMode=story",
  );
  const table = page.getByRole("table", { name: "Sessions", exact: true }),
    rows = table.locator("tbody tr");
  const first = rows.first().getByRole("checkbox"),
    sixth = rows.nth(5).getByRole("checkbox");
  const start = (await first.boundingBox())!,
    end = (await sixth.boundingBox())!;
  await page.mouse.move(start.x + start.width / 2, start.y + start.height / 2);
  await page.mouse.down();
  await expect(first).toBeChecked();
  await page.mouse.move(end.x + end.width / 2, end.y + end.height / 2);
  await expect(table.locator("tbody input:checked")).toHaveCount(6);
  await page.mouse.move(start.x + start.width / 2, start.y + start.height / 2);
  await expect(table.locator("tbody input:checked")).toHaveCount(1);
  await page.mouse.up();
  await expect(table.locator("tbody input:checked")).toHaveCount(1);
  await rows
    .nth(6)
    .getByRole("checkbox")
    .click({ modifiers: ["Shift"] });
  await expect(table.locator("tbody input:checked")).toHaveCount(7);
});

test("session checkbox Shift ranges shrink until the modifier is released", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--long-sessions&viewMode=story",
  );
  const table = page.getByRole("table", { name: "Sessions", exact: true }),
    rows = table.locator("tbody tr");
  await rows.first().getByRole("checkbox").check();
  await page.keyboard.down("Shift");
  await rows.nth(4).getByRole("checkbox").click();
  await expect(table.locator("tbody input:checked")).toHaveCount(5);
  await rows.nth(2).getByRole("checkbox").click();
  await expect(table.locator("tbody input:checked")).toHaveCount(3);
  await expect(rows.nth(3).getByRole("checkbox")).not.toBeChecked();
  await expect(rows.nth(4).getByRole("checkbox")).not.toBeChecked();
  await page.keyboard.up("Shift");
  await page.keyboard.down("Shift");
  await rows.nth(1).getByRole("checkbox").click();
  await expect(table.locator("tbody input:checked")).toHaveCount(1);
  await expect(rows.nth(2).getByRole("checkbox")).toBeChecked();
  await page.keyboard.up("Shift");
});

test("project list icons cover all container states and menus follow their lifecycle availability", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--list-states&viewMode=story");
  const rows = page
    .getByRole("table", { name: "Projects", exact: true })
    .locator("tbody tr");
  const states = [
    { name: "Running", stop: true, start: false, remove: true },
    { name: "Not started", stop: false, start: true, remove: true },
    { name: "Stopped", stop: false, start: true, remove: true },
    { name: "No container", stop: false, start: true, remove: false },
    { name: "Unknown", stop: false, start: false, remove: false },
  ];
  await expect(rows).toHaveCount(states.length);
  for (const [index, state] of states.entries()) {
    const row = rows.nth(index);
    await expect(
      row.getByRole("img", { name: state.name, exact: true }),
    ).toBeVisible();
    await row
      .getByRole("button", {
        name: `Controls for ${state.name} workspace`,
        exact: true,
      })
      .click();
    for (const [action, enabled] of [
      ["Stop", state.stop],
      ["Start", state.start],
      ["Remove", state.remove],
      ["Erase", true],
    ] as const) {
      const item = page.getByRole("menuitem", { name: action, exact: true });
      if (enabled) await expect(item).toBeEnabled();
      else await expect(item).toBeDisabled();
    }
    await page.keyboard.press("Escape");
  }
});

test("project row start uses the shared configuration trust confirmation", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--list-requires-trust&viewMode=story",
  );
  const row = page
    .getByRole("table", { name: "Projects", exact: true })
    .locator("tbody tr")
    .first();
  const more = row.getByRole("button", {
    name: "Controls for Web workspace",
    exact: true,
  });
  await more.click();
  await page.getByRole("menuitem", { name: "Start", exact: true }).click();
  const start = page.getByRole("dialog", {
    name: "Start project",
    exact: true,
  });
  await start.getByRole("button", { name: "Start", exact: true }).click();
  const trust = page.getByRole("dialog", {
    name: "Trust configuration",
    exact: true,
  });
  await expect(trust).toBeVisible();
  await expect(trust).toContainText("privileged container requested");
  await trust.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(trust).toHaveCount(0);
  await expect(
    row.getByRole("img", { name: "Stopped", exact: true }),
  ).toBeVisible();
  await more.click();
  await page.getByRole("menuitem", { name: "Start", exact: true }).click();
  await start.getByRole("button", { name: "Start", exact: true }).click();
  await trust
    .getByRole("button", { name: "Trust and start", exact: true })
    .click();
  await expect(trust).toHaveCount(0);
  await expect(
    row.getByRole("img", { name: "Running", exact: true }),
  ).toBeVisible();
});

test("project activity favors unread completed turns over work, and questions do not replace working sessions", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--list-activity&viewMode=story",
  );
  const rows = page
    .getByRole("table", { name: "Projects", exact: true })
    .locator("tbody tr");
  const unread = rows.first().locator("td").nth(3).getByRole("link");
  await expect(unread).toHaveText("Unread completed conversationsession-3");
  await expect(unread).toHaveAttribute("href", "/c/_/session/session-3");
  await expect(rows.first().locator("time")).toHaveText("30 minutes ago");
  await expect(rows.last().locator("td").nth(3)).toHaveText(
    "Working conversationelm",
  );
  await expect(
    rows.last().getByRole("img", { name: "Working", exact: true }),
  ).toBeVisible();
  await expect(rows.last().locator(".session-activity-dot")).toHaveCount(8);
});

test("inactive project cards are darker, names stay bounded, and state descriptions overlay on hover and focus", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--list-states&viewMode=story");
  const rows = page
    .getByRole("table", { name: "Projects", exact: true })
    .locator("tbody tr");
  const link = (index: number) =>
    rows.nth(index).locator("td").nth(2).getByRole("link");
  const lightness = (index: number) =>
    link(index).evaluate((node) => {
      const probe = document.createElement("span");
      probe.style.color = getComputedStyle(node).getPropertyValue(
        "--button-background",
      );
      node.append(probe);
      const color = getComputedStyle(probe).color;
      probe.remove();
      return color;
    });
  const running = await lightness(0),
    stopped = await lightness(2),
    absent = await lightness(3);
  expect(stopped).toBe(absent);
  expect(stopped).not.toBe(running);
  expect((await link(0).boundingBox())!.width).toBeLessThanOrEqual(180);
  const state = rows.nth(2).getByRole("img", { name: "Stopped", exact: true });
  await state.hover();
  await expect(page.getByRole("tooltip")).toHaveText(
    "The project container is stopped. Its data is retained.",
  );
  await page.keyboard.press("Escape");
  await expect(page.getByRole("tooltip")).toHaveCount(0);
  await state.focus();
  await expect(page.getByRole("tooltip")).toBeVisible();
});

test("selecting a hovered inactive project brightens both row-aware cards", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--list-requires-trust&viewMode=story",
  );
  const row = page
    .getByRole("table", { name: "Projects", exact: true })
    .locator("tbody tr")
    .first();
  const cards = row.getByRole("link");
  await expect(cards).toHaveCount(2);
  const tones = () =>
    cards.evaluateAll((nodes) =>
      nodes.map((node) => {
        const ctx = document.createElement("canvas").getContext("2d")!;
        ctx.fillStyle = getComputedStyle(node).backgroundColor;
        ctx.fillRect(0, 0, 1, 1);
        return ctx.getImageData(0, 0, 1, 1).data[0]!;
      }),
    );
  const settle = () =>
    row.evaluate(async (node) => {
      await Promise.all(
        node
          .getAnimations({ subtree: true })
          .filter((a) => a instanceof CSSTransition)
          .map((a) => a.finished.catch(() => {})),
      );
    });
  await row.locator("td").nth(4).hover();
  await settle();
  const before = await tones();
  const checkbox = row.getByRole("checkbox");
  await checkbox.focus();
  await checkbox.press("Space");
  await expect(row).toHaveAttribute("data-selected", "true");
  await expect
    .poll(async () =>
      (await tones()).every((value, index) => value > before[index]!),
    )
    .toBe(true);
  await checkbox.press("Space");
  await expect(row).not.toHaveAttribute("data-selected", "true");
  await expect.poll(tones).toEqual(before);
});

test("header actions confirm all eligible checked projects and retain only failures for retry", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--list-batch-partial-failure&viewMode=story",
  );
  const table = page.getByRole("table", { name: "Projects", exact: true }),
    rows = table.locator("tbody tr");
  const more = table.getByRole("button", {
    name: "Selected project actions",
    exact: true,
  });
  await expect(more).toBeDisabled();
  await table
    .getByRole("checkbox", { name: "Select all projects", exact: true })
    .check();
  await more.click();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  const confirmation = page.getByRole("dialog");
  await expect(confirmation).toHaveAccessibleName("Stop projects");
  await expect(confirmation).toContainText("Web workspace");
  await expect(confirmation).toContainText("API workspace");
  await confirmation.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(confirmation.getByRole("alert")).toContainText(
    "Simulated project action failure",
  );
  await expect(confirmation).not.toContainText("Web workspace");
  await expect(
    rows.first().getByRole("img", { name: "Stopped", exact: true }),
  ).toBeVisible();
  await expect(rows.first().getByRole("checkbox")).not.toBeChecked();
  await expect(rows.last().getByRole("checkbox")).toBeChecked();
  await confirmation.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(confirmation).toHaveCount(0);
  await expect(
    rows.last().getByRole("img", { name: "Stopped", exact: true }),
  ).toBeVisible();
  await expect(more).toBeDisabled();
});

test("batch start preserves per-project configuration trust requirements", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--list-requires-trust&viewMode=story",
  );
  const table = page.getByRole("table", { name: "Projects", exact: true });
  await table
    .getByRole("checkbox", { name: "Select all projects", exact: true })
    .check();
  await table
    .getByRole("button", { name: "Selected project actions", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Start", exact: true }).click();
  await page
    .getByRole("dialog", { name: "Start projects", exact: true })
    .getByRole("button", { name: "Start", exact: true })
    .click();
  const trust = page.getByRole("dialog", {
    name: "Trust configuration",
    exact: true,
  });
  await expect(trust).toContainText("Web workspace, API workspace");
  await trust
    .getByRole("button", { name: "Trust and start", exact: true })
    .click();
  await expect(trust).toHaveCount(0);
  await expect(
    table.locator("tbody").getByRole("img", { name: "Running", exact: true }),
  ).toHaveCount(2);
  await expect(
    table.getByRole("button", {
      name: "Selected project actions",
      exact: true,
    }),
  ).toBeDisabled();
});

test("project row action failures keep confirmation open and the project unchanged", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--list-unavailable&viewMode=story",
  );
  const row = page
    .getByRole("table", { name: "Projects", exact: true })
    .locator("tbody tr")
    .first();
  await row
    .getByRole("button", { name: "Controls for Web workspace", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Stop project",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Project stop is unavailable",
  );
  await expect(
    row.getByRole("img", { name: "Working", exact: true }),
  ).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});

test("project sessions use the account table design with stacked resource links and scoped selection", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  await expect(table.getByRole("columnheader")).toHaveText([
    "",
    "Name",
    "Account",
    "",
    "",
  ]);
  await expect(
    table.getByRole("columnheader", { name: "State", exact: true }),
  ).toBeVisible();
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(2);
  await expect(table.locator(".editable-value-trigger, fieldset")).toHaveCount(
    0,
  );
  const session = rows
    .first()
    .getByRole("link", { name: "Plan the next release oak", exact: true });
  const account = rows
    .first()
    .getByRole("link", { name: "Work account work", exact: true });
  await expect(session).toHaveAttribute("href", "/c/_/session/oak");
  await expect(account).toHaveAttribute("href", "/c/_/account/work");
  await expect(account.locator(".agent-brand-codex")).toBeVisible();
  await expect(
    rows
      .last()
      .getByRole("link", { name: "Work account work", exact: true })
      .locator(".agent-brand-codex"),
  ).toBeVisible();
  const logo = (await account.locator(".agent-brand").boundingBox())!;
  const accountText = (await account
    .getByText("Work account", { exact: true })
    .boundingBox())!;
  expect(logo.x + logo.width).toBeLessThanOrEqual(accountText.x);
  for (const [link, name, alias] of [
    [session, "Plan the next release", "oak"],
    [account, "Work account", "work"],
  ] as const) {
    const title = link.getByText(name, { exact: true });
    const subtitle = link.getByText(alias, { exact: true });
    const top = (await title.boundingBox())!;
    const bottom = (await subtitle.boundingBox())!;
    expect(bottom.x).toBe(top.x);
    expect(bottom.y).toBeGreaterThanOrEqual(top.y + top.height);
    expect(
      await subtitle.evaluate((node) =>
        parseFloat(getComputedStyle(node).fontSize),
      ),
    ).toBeLessThan(
      await title.evaluate((node) =>
        parseFloat(getComputedStyle(node).fontSize),
      ),
    );
    await expect(link).toHaveCSS("border-top-width", "0px");
    await expect(link).toHaveCSS("box-shadow", "none");
  }
  await expect(
    rows.first().getByRole("img", { name: "Awaiting answer", exact: true }),
  ).toBeVisible();
  const all = table.getByRole("checkbox", {
    name: "Select all sessions",
    exact: true,
  });
  await rows
    .first()
    .locator("td")
    .first()
    .click({ position: { x: 2, y: 2 } });
  await expect(rows.first().getByRole("checkbox")).toBeChecked();
  await expect(all).toBeChecked({ indeterminate: true });
  await rows.first().getByRole("checkbox").press("Space");
  await expect(rows.first().getByRole("checkbox")).not.toBeChecked();
  await expect(all).not.toBeChecked();
  await rows.first().getByRole("checkbox").check();
  await expect(all).toBeChecked({ indeterminate: true });
  await expect(rows.first()).toHaveAttribute("data-selected", "true");
  await table
    .getByRole("columnheader")
    .first()
    .click({ position: { x: 2, y: 2 } });
  for (const row of await rows.all())
    await expect(row.getByRole("checkbox")).toBeChecked();
  await rows.first().getByRole("checkbox").uncheck();
  await expect(rows.first().getByRole("checkbox")).not.toBeChecked();
  await expect(rows.nth(1).getByRole("checkbox")).toBeChecked();
  await all.check();
  await all.uncheck();
  for (const row of await rows.all())
    await expect(row.getByRole("checkbox")).not.toBeChecked();
  await expect(
    page.getByRole("region", { name: "Session details", exact: true }),
  ).toHaveCount(0);
});

test("session table reserves seven and a half rows with a sticky header and independent scrolling", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(
    "/iframe.html?id=pages-projects--long-sessions&viewMode=story",
  );
  const list = page.getByRole("region", { name: "Session list", exact: true });
  const viewport = list.locator(".panel-scroll-content");
  const cards = list.locator("tbody tr");
  await expect(cards).toHaveCount(16);
  const frame = (await viewport.boundingBox())!;
  const eighth = (await cards.nth(7).boundingBox())!;
  // Rows include their divider; the former inset cards did not.
  expect(
    Math.abs(frame.y + frame.height - eighth.y - eighth.height / 2),
  ).toBeLessThanOrEqual(1);
  const header = list.getByRole("columnheader").first();
  const headerBefore = (await header.boundingBox())!;
  const outer = page.locator(
    "main > .panel-scroll-region > .panel-scroll-content",
  );
  const outerTop = await outer.evaluate((el) => el.scrollTop);
  const target = await viewport.evaluate((el) =>
    Math.min(el.scrollTop + 300, el.scrollHeight - el.clientHeight),
  );
  await viewport.hover();
  await page.mouse.wheel(0, 300);
  await expect.poll(() => viewport.evaluate((el) => el.scrollTop)).toBe(target);
  expect(await outer.evaluate((el) => el.scrollTop)).toBe(outerTop);
  expect((await header.boundingBox())!.y).toBe(headerBefore.y);
  const scrolled = await viewport.evaluate((el) => el.scrollTop);
  const section = page.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const before = await section.boundingBox();
  await cards.nth(10).getByRole("checkbox").check();
  await expect(cards.nth(10)).toHaveAttribute("data-selected", "true");
  expect(await viewport.evaluate((el) => el.scrollTop)).toBe(scrolled);
  expect(await viewport.boundingBox()).toEqual(frame);
  expect(await section.boundingBox()).toEqual(before);
  await expect(cards.nth(10).locator("td").nth(1)).toContainText("Session 11");
  for (const story of ["default", "empty-sessions"]) {
    await page.goto(`/iframe.html?id=pages-projects--${story}&viewMode=story`);
    await expect(list).toBeVisible();
    expect((await viewport.boundingBox())!.height).toBe(frame.height);
  }
  await expect(list).toContainText("No sessions");
  await expect(
    list.getByRole("checkbox", { name: "Select all sessions" }),
  ).toBeDisabled();
});

test("session states use the same status icons as account sessions", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--session-states&viewMode=story",
  );
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(9);
  await expect(table.locator(".agent-brand")).toHaveCount(9);
  await expect(rows.locator("td:nth-child(4) .session-indicator")).toHaveCount(
    9,
  );
  expect(
    await rows
      .locator("td:nth-child(4) [role=img]")
      .evaluateAll((nodes) =>
        nodes.map((node) => node.getAttribute("aria-label")),
      ),
  ).toEqual([
    "Awaiting answer",
    "Working",
    "Idle",
    "Stopped",
    "Interrupted",
    "Failed",
    "Waiting for input",
    "Unknown",
    "Working",
  ]);
});

test("dashboard scopes project usage and places sessions below Info", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  const info = main.getByRole("region", { name: "Information", exact: true });
  const sessions = main.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const usage = main.getByRole("region", {
    name: "Conversation usage",
    exact: true,
  });
  await expect(info).toContainText("cxz-web-devcontainer");
  for (const panel of [
    info,
    sessions,
    main.getByRole("region", { name: "Actions", exact: true }),
  ])
    await expect(panel.getByRole("heading")).toHaveCount(0);
  const header = sessions.getByRole("columnheader", {
    name: "Controls",
    exact: true,
  });
  await expect(
    header.getByRole("button", {
      name: "Selected session actions",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    header.getByRole("button", { name: "Add session", exact: true }),
  ).toHaveCount(0);
  await expect(
    usage
      .getByRole("region", { name: "Recent 30 days" })
      .getByRole("img", { name: "2400", exact: true }),
  ).toBeVisible();
  await expect(
    main.getByRole("region", { name: "Environment", exact: true }),
  ).toHaveCount(0);
  await expect(
    main.getByRole("region", { name: "Diagnostics", exact: true }),
  ).toHaveCount(0);
  const infoBox = (await info.boundingBox())!,
    sessionsBox = (await sessions.boundingBox())!,
    usageBox = (await usage.boundingBox())!;
  expect(infoBox.width).toBe(420);
  expect(sessionsBox.x).toBe(infoBox.x);
  expect(sessionsBox.width).toBe(infoBox.width);
  expect(sessionsBox.y).toBeGreaterThanOrEqual(infoBox.y + infoBox.height);
  expect(usageBox.y).toBe(infoBox.y);
  expect(usageBox.x - infoBox.x - infoBox.width).toBe(10);
  expect(usageBox.width).toBe(420);
  const all = sessions.getByRole("checkbox", {
    name: "Select all sessions",
    exact: true,
  });
  await all.check();
  await main
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(info).toContainText("cxz-api-devcontainer");
  await expect(info).toContainText("Backend services and API contracts");
  await expect(info).not.toContainText("Frontend and design system");
  await expect(
    usage
      .getByRole("region", { name: "Recent 30 days" })
      .getByRole("img", { name: "4800", exact: true }),
  ).toBeVisible();
  await expect(all).not.toBeChecked();
  await expect(sessions.locator("tbody tr")).toHaveCount(2);
});

test("stopped and unavailable runtimes retain the session table and account names", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--stopped&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  await expect(
    main
      .getByRole("region", { name: "Conversation usage", exact: true })
      .getByRole("region", { name: "Recent 30 days" }),
  ).toBeVisible();
  await expect(
    main.getByRole("region", { name: "Session overview" }),
  ).toHaveCount(0);
  await expect(
    main
      .locator("tbody tr")
      .first()
      .getByRole("img", { name: "Stopped", exact: true }),
  ).toBeVisible();
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  await expect(
    main.getByRole("region", { name: "Information", exact: true }),
  ).toContainText("cxz-web-devcontainer");
  await expect(
    main
      .locator("tbody tr")
      .first()
      .getByRole("link", { name: "Work account work", exact: true }),
  ).toBeVisible();
  await expect(
    main
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr")
      .first()
      .getByRole("checkbox"),
  ).toBeEnabled();
});

test("narrow screens scroll the table without overflowing the page or moving the toolbar", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 600 });
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  const toolbar = main.locator(":scope > header");
  const before = await toolbar.boundingBox();
  const list = main.getByRole("region", { name: "Session list", exact: true });
  const viewport = list.locator(".panel-scroll-content");
  await list.locator("tbody tr").first().getByRole("checkbox").check();
  await expect(
    list.getByRole("checkbox", {
      name: "Select Plan the next release",
      exact: true,
    }),
  ).toBeChecked();
  await viewport.evaluate((el) => (el.scrollLeft = el.scrollWidth));
  await expect(
    list.getByRole("columnheader", { name: "State", exact: true }),
  ).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  expect(await toolbar.boundingBox()).toEqual(before);
});

test("live inventory preserves row selection and account labels", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--live-inventory&viewMode=story",
  );
  const row = page
    .getByRole("table", { name: "Sessions", exact: true })
    .locator("tbody tr")
    .first();
  const check = row.getByRole("checkbox");
  await check.check();
  await expect(row.locator("td").nth(2)).toContainText("Work account");
  // Several inventory revisions occur while checking selection and card position.
  const before = await row.boundingBox();
  await expect
    .poll(async () => {
      await expect(check).toBeChecked();
      return row.locator("td").nth(2).textContent();
    })
    .toContain("Work account");
  expect(await row.boundingBox()).toEqual(before);
});

test("project metadata shows short IDs, copy complete values, and use machine-readable dates", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  const info = main.getByRole("region", { name: "Information", exact: true });
  await expect(info).toBeVisible();
  const entries = info.locator("dl > div");
  const left = (await entries.nth(0).boundingBox())!;
  const right = (await entries.nth(1).boundingBox())!;
  expect(right.y).toBe(left.y);
  expect(right.x).toBeGreaterThan(left.x + left.width);
  expect((await entries.nth(2).boundingBox())!.y).toBeGreaterThan(left.y);
  await expect(main.locator("h1, details, summary")).toHaveCount(0);
  const container = info.locator("dl > div").filter({
    has: page.locator("dt > span:first-child", { hasText: /^Container ID$/ }),
  });
  await expect(container.getByText("contain", { exact: true })).toBeVisible();
  const trigger = container.getByRole("button", {
    name: "Copy: Container ID",
    exact: true,
  });
  await trigger.hover();
  const full = page.getByRole("tooltip", { name: "container-1", exact: true });
  await expect(full).toBeVisible();
  await expectAlignedText(trigger);
  expect(await full.evaluate((el) => el.parentElement === document.body)).toBe(
    true,
  );
  await page.keyboard.press("Escape");
  await expect(full).toHaveCount(0);
  await trigger.focus();
  await expect(full).toBeVisible();
  await trigger.press("Escape");
  await expect(full).toHaveCount(0);
  await trigger.click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("container-1");
  const projectId = info.locator("dl > div").filter({
    has: page.locator("dt > span:first-child", { hasText: /^Project ID$/ }),
  });
  await expect(projectId.getByText("0000000", { exact: true })).toBeVisible();
  const copyId = projectId.getByRole("button", {
    name: "Copy: Project ID",
    exact: true,
  });
  await expect(projectId.locator("dd svg")).toHaveCount(0);
  await copyId.click();
  await expect(projectId.getByRole("status")).toHaveText("copied!");
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("00000000-0000-8000-8007-000000000001");
  await copyId.hover();
  await expect(
    page.getByRole("tooltip", {
      name: "00000000-0000-8000-8007-000000000001",
      exact: true,
    }),
  ).toBeVisible();
  await expectAlignedText(copyId);
  await page.keyboard.press("Escape");
  await copyId.evaluate((node) => node.blur());
  await copyId.focus();
  await page.evaluate(() =>
    navigator.clipboard.writeText("keyboard-copy-check"),
  );
  await copyId.press("Enter");
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("00000000-0000-8000-8007-000000000001");
  await expect(copyId).toBeFocused();
  await expect(projectId.getByRole("status")).toHaveText("copied!");
  await expect(projectId.getByRole("status")).toBeEmpty();
  await page.keyboard.press("Escape");
  await expect(info.locator("dt > span:first-child")).toHaveText([
    "Project ID",
    "Name",
    "Alias",
    "Description",
    "Workspace",
    "Container name",
    "Container ID",
    "Created",
    "Updated",
  ]);
  await expect(
    info
      .locator("dt > span:first-child")
      .filter({ hasText: /^(Configuration|Deleted)$/ }),
  ).toHaveCount(0);
  const created = info
    .locator("dl > div")
    .filter({ has: page.locator("dt", { hasText: /^Created/ }) })
    .locator("dd time");
  await expect(created).toHaveAttribute("datetime", /^\d{4}-\d\d-\d\dT.*Z$/);
  const elapsed = created.locator("xpath=../../dt//time");
  await expect(elapsed).toHaveText(/ago|yesterday/);
  expect(await elapsed.getAttribute("datetime")).toBe(
    await created.getAttribute("datetime"),
  );
  const elapsedBox = (await elapsed.boundingBox())!;
  const labelBox = (await created
    .locator("xpath=../../dt/span[1]")
    .boundingBox())!;
  expect(
    Math.abs(
      elapsedBox.y + elapsedBox.height / 2 - labelBox.y - labelBox.height / 2,
    ),
  ).toBeLessThanOrEqual(1);
  expect(elapsedBox.x - labelBox.x - labelBox.width).toBeGreaterThan(0);
  expect(elapsedBox.x - labelBox.x - labelBox.width).toBeLessThanOrEqual(8);

  expect(
    await main
      .locator("time")
      .evaluateAll((nodes) =>
        nodes.every(
          (node) =>
            !!node.textContent?.trim() &&
            Number.isFinite(Date.parse(node.getAttribute("datetime") ?? "")),
        ),
      ),
  ).toBe(true);
  await main
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(
    info
      .locator("dt > span:first-child")
      .filter({ hasText: /^(Configuration|Deleted)$/ }),
  ).toHaveCount(0);
});

async function expectAlignedText(trigger: Locator) {
  const boxes = await trigger.evaluate((element) => {
    function firstCharacters(target: Element) {
      const walker = document.createTreeWalker(target, NodeFilter.SHOW_TEXT);
      const text = walker.nextNode()!;
      const range = document.createRange();
      range.setStart(text, 0);
      range.setEnd(text, Math.min(7, text.textContent!.length));
      const rect = range.getBoundingClientRect();
      return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
    }
    const hint = document.getElementById(
      element.getAttribute("aria-describedby")!,
    )!;
    return {
      original: firstCharacters(element),
      expanded: firstCharacters(hint),
    };
  });
  for (const key of ["x", "y", "width", "height"] as const)
    expect(Math.abs(boxes.original[key] - boxes.expanded[key])).toBeLessThan(
      0.25,
    );
}

for (const scale of [1, 1.25, 1.5, 2]) {
  test.describe(`in-place text at device scale ${scale}`, () => {
    test.use({ deviceScaleFactor: scale });
    test("aligns one complete overlay with the original text and typography", async ({
      page,
    }) => {
      await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
      const row = page
        .getByRole("region", { name: "Information", exact: true })
        .locator("dl > div")
        .filter({
          has: page.locator("dt > span:first-child", {
            hasText: /^Container ID$/,
          }),
        });
      const trigger = row.getByRole("button", {
        name: "Copy: Container ID",
        exact: true,
      });
      await page.evaluate(() => document.fonts.ready);
      await expect(
        page.getByRole("region", { name: "Information", exact: true }),
      ).toContainText("Web workspace");
      await trigger.evaluate((element) =>
        element.scrollIntoView({
          block: "center",
          inline: "nearest",
          behavior: "instant",
        }),
      );
      const before = await trigger.boundingBox();
      const typography = await trigger.evaluate((element) => {
        const text = document
          .createTreeWalker(element, NodeFilter.SHOW_TEXT)
          .nextNode()!;
        const style = getComputedStyle(text.parentElement!);
        return {
          font: style.font,
          color: style.color,
          letterSpacing: style.letterSpacing,
          fontFeatureSettings: style.fontFeatureSettings,
          fontVariantNumeric: style.fontVariantNumeric,
        };
      });
      await trigger.hover();
      const hint = page.getByRole("tooltip", {
        name: "container-1",
        exact: true,
      });
      await expect(hint).toBeVisible();
      await expectAlignedText(trigger);
      expect(await trigger.boundingBox()).toEqual(before);
      expect(
        await hint.evaluate((element) => {
          const style = getComputedStyle(element);
          return {
            font: style.font,
            color: style.color,
            letterSpacing: style.letterSpacing,
            fontFeatureSettings: style.fontFeatureSettings,
            fontVariantNumeric: style.fontVariantNumeric,
          };
        }),
      ).toEqual(typography);
      // Scrolled and fixed layers can use different grayscale/LCD antialiasing.
      // Verify the aligned single surface rather than requiring identical pixels.
      await expect(hint).toHaveCSS("clip-path", "none");
      await expect(trigger).toHaveCSS("opacity", "0");
      await page.mouse.move(0, 0);
      await expect(hint).toHaveCount(0);
      await expect(trigger).toHaveCSS("opacity", "1");
      expect(await trigger.boundingBox()).toEqual(before);
    });
  });
}
