import { expect, test, type Locator } from "@playwright/test";

test("project page and shared toolbar stories render their selector and states", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=components-pagetoolbar--project-selector&viewMode=story",
  );
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("Web workspace");
  await picker.click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(picker).toHaveText("API workspace");
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
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

test("session table preserves the identity card and supports individual and select-all checkboxes", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  await expect(table.getByRole("columnheader")).toHaveText([
    "",
    "Session",
    "Account",
    "State",
  ]);
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(2);
  await expect(rows.first().locator("td").nth(1)).toContainText(
    "Plan the next release",
  );
  await expect(rows.first().locator("td").nth(1)).toContainText("oak");
  await expect(rows.first().locator("td").nth(2)).toHaveText("Work account");
  await expect(
    rows.first().getByRole("link", { name: "Work account", exact: true }),
  ).toHaveAttribute("href", "/accounts?account=work");
  await expect(rows.first().locator("td").nth(3)).toHaveText(
    "Waiting for input",
  );
  const all = table.getByRole("checkbox", {
    name: "Select all sessions",
    exact: true,
  });
  await rows.first().getByRole("checkbox").check();
  await expect(all).toBeChecked({ indeterminate: true });
  await expect(rows.first().getByRole("button")).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await all.check();
  for (const row of await rows.all())
    await expect(row.getByRole("checkbox")).toBeChecked();
  await rows.first().getByRole("button").click();
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
  const cards = list.getByRole("button");
  await expect(cards).toHaveCount(16);
  const frame = (await viewport.boundingBox())!;
  const eighth = (await cards.nth(7).boundingBox())!;
  expect(
    Math.abs(frame.y + frame.height - eighth.y - eighth.height / 2),
  ).toBeLessThan(1);
  const header = list.getByRole("columnheader").first();
  const headerBefore = (await header.boundingBox())!;
  const outer = page.locator(
    "main > .panel-scroll-region > .panel-scroll-content",
  );
  const outerTop = await outer.evaluate((el) => el.scrollTop);
  await viewport.hover();
  await page.mouse.wheel(0, 300);
  await expect
    .poll(() => viewport.evaluate((el) => el.scrollTop))
    .toBeGreaterThan(100);
  expect(await outer.evaluate((el) => el.scrollTop)).toBe(outerTop);
  expect((await header.boundingBox())!.y).toBe(headerBefore.y);
  const scrolled = await viewport.evaluate((el) => el.scrollTop);
  const section = page.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const before = await section.boundingBox();
  await cards.nth(10).click();
  await expect(cards.nth(10)).toHaveAttribute("aria-pressed", "true");
  expect(await viewport.evaluate((el) => el.scrollTop)).toBe(scrolled);
  expect(await viewport.boundingBox()).toEqual(frame);
  expect(await section.boundingBox()).toEqual(before);
  await expect(cards.nth(10).locator(".session-heading")).toContainText(
    "Session 11",
  );
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

test("session states stay in their column without leading icons", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--session-states&viewMode=story",
  );
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(9);
  await expect(table.locator(".session-indicator")).toHaveCount(0);
  await expect(rows.locator("td:last-child")).toHaveText([
    "Waiting for input",
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

test("dashboard scopes project data, resets selection on project change and keeps diagnostics project-only", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  const environment = main.getByRole("region", {
    name: "Environment",
    exact: true,
  });
  const tasks = main.getByRole("region", {
    name: "Project tasks",
    exact: true,
  });
  const sessions = main.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const diagnostics = main.getByRole("region", {
    name: "Diagnostics",
    exact: true,
  });
  await expect(environment).toContainText("ui/dashboard");
  await expect(environment).toContainText("/workspace");
  await expect(tasks).toContainText("Review the release plan");
  await expect(
    tasks.getByRole("link", { name: "Open task board" }),
  ).toHaveAttribute("href", /project=project-1/);
  const toolbar = (await main.locator(":scope > header").boundingBox())!;
  const sectionGap = await sessions
    .locator("..")
    .evaluate((el) => parseFloat(getComputedStyle(el).rowGap));
  const sessionsBox = (await sessions.boundingBox())!;
  expect(sessionsBox.y).toBe(toolbar.y + toolbar.height + sectionGap);
  const environmentBox = (await environment.boundingBox())!;
  expect(environmentBox.x + environmentBox.width).toBeLessThan(sessionsBox.x);
  expect((await diagnostics.boundingBox())!.x).toBe(environmentBox.x);
  await expect(diagnostics.locator("details, summary")).toHaveCount(0);
  await expect(
    diagnostics.getByText("Provider session ID", { exact: true }),
  ).toHaveCount(0);
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
  await expect(environment).toContainText("api-main");
  await expect(tasks).toContainText("No outstanding tasks");
  await expect(all).not.toBeChecked();
  await expect(sessions.getByRole("button")).toHaveCount(2);
  await expect(
    main.getByRole("region", { name: "Session details", exact: true }),
  ).toHaveCount(0);
});

test("stopped and unavailable runtimes retain the session table and account names", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--stopped&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  await expect(
    main.getByRole("region", { name: "Environment", exact: true }),
  ).toContainText("Project is not running");
  await expect(
    main.getByRole("region", { name: "Session overview" }),
  ).toContainText("Stopped2");
  await expect(
    main.locator("tbody tr").first().locator("td").nth(3),
  ).toHaveText("Stopped");
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  await expect(
    main.getByRole("region", { name: "Environment", exact: true }),
  ).toContainText("Workspace is unavailable");
  await expect(
    main.locator("tbody tr").first().locator("td").nth(2),
  ).toHaveText("Work account");
  await expect(
    main.getByRole("button", { name: /Plan the next release/ }),
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
  await list.getByRole("button").first().click();
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
  await expect(row.locator("td").nth(2)).toHaveText("Work account");
  // Several inventory revisions occur while checking selection and card position.
  const before = await row.boundingBox();
  await expect
    .poll(async () => {
      await expect(check).toBeChecked();
      return row.locator("td").nth(2).textContent();
    })
    .toBe("Work account");
  expect(await row.boundingBox()).toEqual(before);
});

test("diagnostics show short IDs, copy complete values, and use machine-readable dates", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  const diagnostic = main.getByRole("region", {
    name: "Diagnostics",
    exact: true,
  });
  await expect(diagnostic).toBeVisible();
  await expect(main.locator("h1, details, summary")).toHaveCount(0);
  const container = diagnostic
    .locator("dl > div")
    .filter({ has: page.locator("dt", { hasText: /^Container ID$/ }) });
  await expect(container.getByText("contain", { exact: true })).toBeVisible();
  const trigger = container.locator("[tabindex]");
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
  await container.getByRole("button", { name: "Copy", exact: true }).click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("container-1");
  const created = diagnostic
    .locator("dl > div")
    .filter({ has: page.locator("dt", { hasText: /^Created$/ }) })
    .locator("time");
  await expect(created).toHaveAttribute("datetime", /^\d{4}-\d\d-\d\dT.*Z$/);
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
});

test("full ID hints escape clipped containers and remain inside the viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 600 });
  await page.goto(
    "/iframe.html?id=components-overflowhint--default&viewMode=story",
  );
  const trigger = page.locator("[tabindex='0']");
  await trigger.hover();
  const hint = page.getByRole("tooltip");
  await expect(hint).toHaveText("1a2b3c4d-5e6f-7788-9900-aabbccddeeff");
  const card = (await trigger.locator("..").boundingBox())!;
  const bounds = (await hint.boundingBox())!;
  expect(bounds.x + bounds.width).toBeGreaterThan(card.x + card.width);
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(600);
  expect(await hint.evaluate((el) => el.parentElement === document.body)).toBe(
    true,
  );
  await page.mouse.move(0, 0);
  await expect(hint).toHaveCount(0);
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

test("in-place ID overlays keep the text origin and baseline with inherited typography", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 600 });
  await page.goto(
    "/iframe.html?id=components-overflowhint--in-place&viewMode=story",
  );
  const trigger = page.locator("[tabindex='0']");
  await trigger.hover();
  const hint = page.getByRole("tooltip");
  await expect(hint).toBeVisible();
  await expectAlignedText(trigger);
  await expect(hint).toHaveCSS("font-size", "18px");
  await expect(hint).toHaveCSS("line-height", "36px");
  await page.mouse.move(0, 0);
  await expect(hint).toHaveCount(0);
  await trigger.hover();
  await expect(hint).toBeVisible();
  await expectAlignedText(trigger);
  const frame = (await hint.boundingBox())!;
  expect(frame.x + frame.width).toBeLessThanOrEqual(390);
  expect(await hint.evaluate((el) => el.parentElement === document.body)).toBe(
    true,
  );
});

for (const scale of [1, 1.25, 1.5, 2]) {
  test.describe(`in-place text at device scale ${scale}`, () => {
    test.use({ deviceScaleFactor: scale });
    test("aligns one complete overlay with the original text and typography", async ({
      page,
    }) => {
      await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
      const row = page
        .getByRole("region", { name: "Diagnostics", exact: true })
        .locator("dl > div")
        .filter({ has: page.locator("dt", { hasText: /^Container ID$/ }) });
      const trigger = row.locator("[tabindex]");
      await page.evaluate(() => document.fonts.ready);
      await expect(
        page.getByRole("region", { name: "Project tasks", exact: true }),
      ).toContainText("Review the release plan");
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
