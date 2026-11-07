import { expect, test } from "@playwright/test";
const recovery = "/iframe.html?id=workspace-errors--recovery&viewMode=story";
const pageStory = "/iframe.html?id=workspace-errors--page&viewMode=story";

test("dialog and independent page share errors while page shows all statuses without tabs", async ({
  page,
}) => {
  await page.goto(recovery);
  await page.getByRole("button", { name: "Fail request", exact: true }).click();
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  const link = dialog.getByRole("link", {
    name: "Open errors page",
    exact: true,
  });
  await expect(link).toHaveAttribute("href", "/c/_/errors");
  await link.click();
  const main = page.getByRole("main", { name: "Errors", exact: true });
  await expect(dialog).toHaveCount(0);
  await expect(main).toContainText("Account service is offline");
  await expect(main).toContainText("Read Account");
  await expect(main).toContainText("work");
  await expect(
    main.getByRole("link", { name: "Open errors page", exact: true }),
  ).toHaveCount(0);
  await main.getByRole("button", { name: "Dismiss", exact: true }).click();
  await expect(trigger).not.toHaveAttribute("data-active");
  await expect(main.getByRole("radio")).toHaveCount(0);
  await expect(
    main.getByRole("img", { name: "Dismissed", exact: true }),
  ).toBeVisible();
  await trigger.click();
  await dialog.getByRole("radio", { name: "History 1", exact: true }).click();
  await expect(
    dialog.getByRole("row", { name: /Account service is offline/ }),
  ).toHaveAttribute("title", /^Dismissed/);
  await expect(
    dialog.getByRole("button", { name: "Clear history", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  await main
    .getByRole("button", { name: "Clear history", exact: true })
    .click();
  await expect(main).toContainText("No errors in recent records");
});

test("one hour without recurrence becomes Stale while closed, persists and opens the unified page", async ({
  page,
}) => {
  await page.clock.install({ time: new Date("2026-11-07T03:00:00Z") });
  await page.goto(recovery);
  await page.getByRole("button", { name: "Fail request", exact: true }).click();
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).toHaveAttribute("data-active", "true");
  await page.clock.fastForward(60 * 60_000 + 1);
  await expect(trigger).not.toHaveAttribute("data-active");
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  await expect(dialog).toContainText("No active errors");
  await dialog.getByRole("radio", { name: "History 1", exact: true }).click();
  await expect(
    dialog.getByRole("row", { name: /Account service is offline/ }),
  ).toHaveAttribute("title", /^Stale/);
  await expect(dialog.getByText("Resolved", { exact: true })).toHaveCount(0);
  const link = dialog.getByRole("link", {
    name: "Open errors page",
    exact: true,
  });
  await expect(link).toHaveAttribute("href", "/c/_/errors");
  await link.click();
  const main = page.getByRole("main", { name: "Errors", exact: true });
  await expect(main.getByRole("radio")).toHaveCount(0);
  await expect(
    main.getByRole("img", { name: "Stale", exact: true }),
  ).toBeVisible();
  const stale = main.getByRole("img", { name: "Stale", exact: true });
  await expect(stale).toHaveAttribute("data-icon", "error-stale");
  await expect(stale.locator("path")).toHaveCount(1);
  await expect(stale.locator("*")).toHaveCount(1);
  await expect(stale.locator("path")).toHaveAttribute("d", "M8 3 14 13H2Z");
  await page.goto(pageStory);
  await expect(
    page
      .getByRole("main", { name: "Errors", exact: true })
      .getByRole("img", { name: "Stale", exact: true }),
  ).toBeVisible();
});

for (const width of [1200, 390])
  test(`independent Errors page fits ${width}px and renders the same record details`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 800 });
    await page.goto(pageStory);
    const main = page.getByRole("main", { name: "Errors", exact: true });
    await expect(main).toContainText("Account service is offline");
    await expect(main).toContainText("Read Account");
    await expect(main).toContainText("cxz.AccountService/Get");
    await expect(main.locator(".floating-card")).toHaveCount(0);
    const table = main.getByRole("table", { name: "Errors", exact: true });
    await expect(table).toBeVisible();
    expect(
      (await table.locator("tbody > tr[data-status]").boundingBox())!.height,
    ).toBeLessThan(120);
    const toolbar = main.locator("[data-resource-toolbar-content]");
    expect((await table.boundingBox())!.x).toBeCloseTo(
      (await toolbar.boundingBox())!.x,
      0,
    );
    expect((await table.boundingBox())!.width).toBeCloseTo(
      (await toolbar.boundingBox())!.width,
      0,
    );
    const messageWidth = (await table
      .getByRole("columnheader", { name: "Message", exact: true })
      .boundingBox())!.width;
    expect(messageWidth / (await table.boundingBox())!.width).toBeGreaterThan(
      width < 600 ? 0.6 : 0.2,
    );
    await table.locator("tbody > tr").first().click();
    await expect(
      page.getByRole("dialog", { name: "Error details", exact: true }),
    ).toContainText("First seen");
    await expect(
      page.getByRole("dialog", { name: "Error details", exact: true }),
    ).toContainText("cxz.AccountService/Get");
    await expect(
      main.getByRole("button", { name: "Close errors", exact: true }),
    ).toHaveCount(0);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
    const box = (await main.boundingBox())!;
    const list = (await main
      .getByLabel("Error list", { exact: true })
      .boundingBox())!;
    expect(list.height).toBeGreaterThan(200);
    expect(list.y + list.height).toBeLessThanOrEqual(box.y + box.height);
  });

test("unified resource table sorts Active first and opens complete details in a modal", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=workspace-errors--overview&viewMode=story");
  const main = page.getByRole("main", { name: "Errors", exact: true });
  const table = main.getByRole("table", { name: "Errors", exact: true });
  await expect(main.getByRole("radio")).toHaveCount(0);
  await expect(table.getByRole("row")).toHaveCount(10);
  const statuses = await table
    .locator("tbody tr")
    .evaluateAll((rows) => rows.map((row) => row.getAttribute("data-status")));
  expect(statuses.slice(0, 6)).toEqual(Array(6).fill("active"));
  expect(statuses.slice(6)).not.toContain("active");
  await expect(
    table.getByRole("columnheader", { name: "Status", exact: true }),
  ).toHaveText("");
  const row = table
    .getByRole("row")
    .filter({ hasText: "Memory revision changed" });
  const details = row;
  await details.focus();
  await page.keyboard.press("Enter");
  const expanded = page.getByRole("dialog", {
    name: "Error details",
    exact: true,
  });
  await expect(expanded).toContainText("cxz.MemoryService/Patch");
  await expect(expanded).toContainText("https://workspace.example.com");
  await expect(expanded).toContainText("Request ID");
  await expect(table.locator("tr")).toHaveCount(10);
  await page.keyboard.press("Escape");
  await expect(expanded).toHaveCount(0);
  await expect(details).toBeFocused();
  await main
    .getByRole("button", { name: "Clear history", exact: true })
    .click();
  await expect(table.getByRole("row")).toHaveCount(7);
});

test("normalizes mixed error kinds in the sidebar and page, leaving raw transport fields in details", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=workspace-errors--overview&viewMode=story");
  const main = page.getByRole("main", { name: "Errors", exact: true });
  const table = main.getByRole("table", { name: "Errors", exact: true });
  await expect(
    table.getByRole("columnheader", { name: "Kind", exact: true }),
  ).toBeVisible();
  await expect(
    table
      .getByRole("row")
      .filter({ hasText: "Conversation stream disconnected" }),
  ).toContainText("Release review");
  await expect(table).toContainText("cxz.SessionService/Events");
  const upload = table
    .getByRole("row")
    .filter({ hasText: "File upload failed" });
  await expect(upload).toContainText("HTTP");
  await expect(upload).toContainText("HTTP request");
  await upload.click();
  const details = page.getByRole("dialog", {
    name: "Error details",
    exact: true,
  });
  await expect(details).toContainText("HTTP status");
  await expect(details).toContainText("503");
  await expect(details).toContainText("/upload");
  await details
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await expect(details).toHaveCount(0);
  await page.getByRole("button", { name: "Errors", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  const compact = dialog.getByRole("table", { name: "Errors", exact: true });
  await expect(compact).toContainText("cxz.SessionService/Events");
  await expect(compact).not.toContainText("89502035e952a2c227802d29");
  await dialog.getByRole("radio", { name: "History 3", exact: true }).click();
  const login = compact
    .getByRole("row")
    .filter({ hasText: "Provider login was interrupted" });
  await expect(login).toContainText("Provider login");
  await expect(login).toHaveAttribute("title", /personal/);
  await expect(
    dialog.getByRole("button", { name: "Details", exact: true }),
  ).toHaveCount(0);
});

test("new page rows slide down once and count updates stay still", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-errors-rows--incoming-page&viewMode=story",
  );
  const table = page.getByRole("table", { name: "Errors", exact: true });
  await expect(
    table.getByText("Existing error", { exact: true }),
  ).toBeVisible();
  await page.evaluate(() => {
    const table = document.querySelector(
      'table[aria-label="Errors"]',
    )! as HTMLElement;
    table.dataset.animations = "0";
    table.addEventListener("animationstart", (event) => {
      if (
        !(event.target instanceof HTMLElement) ||
        !event.target.matches("tr[data-animate-enter]")
      )
        return;
      table.dataset.animations = String(Number(table.dataset.animations) + 1);
    });
  });
  await page.getByRole("button", { name: "Add error", exact: true }).click();
  await expect(table.locator("tbody tr").first()).toContainText("New error 1");
  await expect(table).toHaveAttribute("data-animations", "1");
  await page.waitForTimeout(350);
  await page
    .getByRole("button", { name: "Repeat latest error", exact: true })
    .click();
  await expect(table.locator("tbody tr").first()).toContainText("002");
  await page.waitForTimeout(300);
  await expect(table).toHaveAttribute("data-animations", "1");
});

for (const theme of ["dark", "light"])
  test(`Errors rows keep Nox transparent defaults on the Projects surface in ${theme} mode`, async ({
    page,
  }) => {
    const styles = async () =>
      page
        .locator("main table")
        .first()
        .evaluate((table) => {
          const row = table.querySelector("tbody tr")!;
          const cell = row.querySelector("td")!;
          const t = getComputedStyle(table),
            r = getComputedStyle(row),
            c = getComputedStyle(cell);
          return {
            background: t.backgroundColor,
            headerBackground: getComputedStyle(table.querySelector("thead")!)
              .backgroundColor,
            rowBackground: r.backgroundColor,
            surface: getComputedStyle(table.closest(".panel-scroll-region")!)
              .backgroundColor,
            rowHeight: t.getPropertyValue("--ui-table-row-height").trim(),
            gap: t.borderSpacing,
            radius: t.borderRadius,
            divider: c.boxShadow,
            border: c.borderBottomWidth,
            align: c.verticalAlign,
          };
        });
    await page.goto(
      `/iframe.html?id=pages-projects--list&viewMode=story&globals=theme:${theme}`,
    );
    await expect(page.locator("main table tbody tr").first()).toBeVisible();
    const { rowBackground: projectRow, ...project } = await styles();
    expect(projectRow).toBe("rgba(0, 0, 0, 0)");
    await page.goto(
      `/iframe.html?id=workspace-errors--overview&viewMode=story&globals=theme:${theme}`,
    );
    const table = page
      .getByRole("main", { name: "Errors", exact: true })
      .getByRole("table");
    await expect(table).toBeVisible();
    const { rowBackground, ...errors } = await styles();
    expect(rowBackground).toBe("rgba(0, 0, 0, 0)");
    expect(errors.background).toBe("rgba(0, 0, 0, 0)");
    expect(errors.headerBackground).toBe("rgba(0, 0, 0, 0)");
    expect(errors).toEqual(project);
    await table.locator("tbody tr").first().hover();
    await expect(table.locator("tbody tr").first()).toHaveCSS(
      "background-color",
      theme === "dark" ? "rgb(32, 32, 32)" : "rgb(241, 241, 241)",
    );
  });

test("status glyphs are plain filled shapes without inner symbols", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=workspace-errors--overview&viewMode=story");
  const main = page.getByRole("main", { name: "Errors", exact: true });
  const active = main.getByRole("img", { name: "Active", exact: true }).first();
  await expect(active).toHaveAttribute("data-icon", "error-active");
  await expect(active).toHaveCSS("color", "rgb(230, 161, 90)");
  await expect(active.locator("path")).toHaveCount(1);
  await expect(active.locator("path")).toHaveAttribute("d", "M8 3 14 13H2Z");
  await expect(active).toHaveAttribute("fill", "currentColor");
  const resolved = main
    .getByRole("img", { name: "Resolved", exact: true })
    .first();
  await expect(resolved).toHaveAttribute("data-icon", "error-resolved");
  await expect(resolved).toHaveCSS("color", "rgb(133, 186, 145)");
  await expect(resolved.locator("circle")).toHaveCount(1);
  await expect(resolved.locator("*")).toHaveCount(1);
});

test("error columns separate operation and diagnostics with an unlabeled count after centered Kind", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=workspace-errors--overview&viewMode=story");
  const table = page
    .getByRole("main", { name: "Errors", exact: true })
    .getByRole("table");
  const heads = table.getByRole("columnheader");
  await expect(heads).toHaveCount(9);
  expect(
    await heads.evaluateAll((heads) =>
      heads.map(
        (head) => head.getAttribute("aria-label") || head.textContent?.trim(),
      ),
    ),
  ).toEqual([
    "Status",
    "Kind",
    "Count",
    "Operation",
    "Diagnostics",
    "Target",
    "Message",
    "Last seen",
    "Actions",
  ]);
  await expect(heads.nth(1)).toHaveCSS("text-align", "center");
  await expect(heads.nth(2)).toHaveText("");
  const row = table
    .getByRole("row")
    .filter({ hasText: "Memory revision changed" });
  const cells = row.getByRole("cell");
  await expect(cells).toHaveCount(9);
  await expect(cells.nth(1)).toHaveText("RPC");
  await expect(cells.nth(1)).toHaveCSS("text-align", "center");
  await expect(cells.nth(2)).toContainText("001");
  await expect(cells.nth(2)).toHaveCSS("text-align", "center");
  await expect(heads.nth(2)).toHaveCSS("text-align", "center");
  await expect(cells.nth(3)).toHaveText("Update Memory");
  await expect(cells.nth(4)).toHaveText("cxz.MemoryService/Patch · Aborted");
  const runtime = table
    .getByRole("row")
    .filter({ hasText: "The project container is unavailable" })
    .getByRole("cell");
  await expect(runtime.nth(3)).toHaveText("Project runtime");
  await expect(runtime.nth(4)).toHaveText("—");
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    1280,
  );
});

for (const key of ["click", "Enter", "Space"])
  test(`error row opens its modal via ${key} and restores row focus`, async ({
    page,
  }) => {
    await page.goto(
      "/iframe.html?id=workspace-errors--overview&viewMode=story",
    );
    const main = page.getByRole("main", { name: "Errors", exact: true });
    const row = main
      .getByRole("row")
      .filter({ hasText: "Memory revision changed" });
    await expect(
      main.getByRole("button", { name: "Details", exact: true }),
    ).toHaveCount(0);
    await expect(row).toHaveAttribute("tabindex", "0");
    if (key === "click")
      await row.getByText(/^Memory revision changed/).click();
    else {
      await row.focus();
      await page.keyboard.press(key);
    }
    const modal = page.getByRole("dialog", {
      name: "Error details",
      exact: true,
    });
    await expect(modal).toContainText("cxz.MemoryService/Patch");
    await page.keyboard.press("Escape");
    await expect(modal).toHaveCount(0);
    await expect(row).toBeFocused();
  });

test("Dismiss only dismisses the error and uses the gray square glyph", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=workspace-errors--overview&viewMode=story");
  const row = page
    .getByRole("main", { name: "Errors", exact: true })
    .getByRole("row")
    .filter({ hasText: "Memory revision changed" });
  await row.getByRole("button", { name: "Dismiss", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "Error details", exact: true }),
  ).toHaveCount(0);
  await expect(row).toHaveAttribute("data-status", "dismissed");
  const icon = row.getByRole("img", { name: "Dismissed", exact: true });
  await expect(icon).toHaveAttribute("data-icon", "error-dismissed");
  await expect(icon.locator("rect")).toHaveCount(1);
  await expect(icon.locator("path")).toHaveCount(0);
  const rgb = await icon.evaluate((icon) =>
    getComputedStyle(icon).color.match(/\d+/g)!.map(Number),
  );
  expect(rgb[0]).toBe(rgb[1]);
  expect(rgb[1]).toBe(rgb[2]);
});

test("selecting row text does not open the modal", async ({ page }) => {
  await page.goto("/iframe.html?id=workspace-errors--overview&viewMode=story");
  const row = page
    .getByRole("main", { name: "Errors", exact: true })
    .getByRole("row")
    .filter({ hasText: "Memory revision changed" });
  await row.getByText(/^Memory revision changed/).evaluate((text) => {
    const range = document.createRange();
    range.selectNodeContents(text);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    text.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await expect(
    page.getByRole("dialog", { name: "Error details", exact: true }),
  ).toHaveCount(0);
});

for (const width of [1200, 390])
  test(`toolbar has small Clear and rightmost Dismiss all actions at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 850 });
    await page.goto(
      "/iframe.html?id=workspace-errors--overview&viewMode=story",
    );
    const main = page.getByRole("main", { name: "Errors", exact: true });
    const clear = main.getByRole("button", {
      name: "Clear history",
      exact: true,
    });
    const dismiss = main.getByRole("button", {
      name: "Dismiss all",
      exact: true,
    });
    await expect(clear).toBeEnabled();
    await expect(dismiss).toBeEnabled();
    expect((await dismiss.boundingBox())!.x).toBeGreaterThan(
      (await clear.boundingBox())!.x,
    );
    const actions = dismiss.locator("..");
    await expect(actions.getByRole("button").last()).toHaveAccessibleName(
      "Dismiss all",
    );
    for (const button of [clear, dismiss]) {
      expect((await button.locator("svg").boundingBox())!.width).toBe(12);
      expect((await button.locator("svg").boundingBox())!.height).toBe(12);
    }
    const rows = main.locator("tbody tr[data-status]");
    await expect(rows).toHaveCount(9);
    await dismiss.click();
    await expect(dismiss).toBeDisabled();
    await expect(rows).toHaveCount(9);
    await expect(main.locator('tbody tr[data-status="active"]')).toHaveCount(0);
    await expect(main.locator('tbody tr[data-status="dismissed"]')).toHaveCount(
      7,
    );
    await expect(main.locator('tbody tr[data-status="resolved"]')).toHaveCount(
      2,
    );
    await expect(
      page.getByRole("dialog", { name: "Error details", exact: true }),
    ).toHaveCount(0);
    await expect(clear).toBeEnabled();
    await clear.click();
    await expect(main).toContainText("No errors in recent records");
    await expect(clear).toBeDisabled();
    await expect(dismiss).toBeDisabled();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
  });
