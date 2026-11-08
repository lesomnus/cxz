import { test, expect, type Page } from "@playwright/test";

const picker = (page: Page) =>
  page.getByRole("combobox", { name: "Auxiliary account", exact: true });
const choices = (page: Page) =>
  page.getByRole("listbox", { name: "Auxiliary account choices", exact: true });
async function open(page: Page, story = "default") {
  await page.clock.setFixedTime(new Date("2026-11-08T10:00:00Z"));
  await page.goto(
    `/iframe.html?id=pages-auxiliary-ai--${story}&viewMode=story`,
  );
  await expect(picker(page)).toBeVisible();
}
for (const width of [390, 1280])
  test(`account trigger and options use Section surfaces at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await open(page);
    await expect(picker(page)).toHaveAttribute(
      "data-section-layout",
      "section",
    );
    await expect(picker(page)).toContainText("Work");
    await expect(picker(page)).toContainText("work");
    await expect(picker(page)).toContainText("64");
    await expect(picker(page).locator("small time")).toHaveText("in 2 hours");
    await expect(picker(page).locator("small time")).toHaveAttribute(
      "datetime",
      "2026-11-08T12:25:00.000Z",
    );
    await expect(
      page.getByText("Shared by Summary, Suggestion and Title", {
        exact: true,
      }),
    ).toHaveCount(0);
    for (const kind of ["Summary", "Suggestion", "Title"]) {
      await expect(
        page.getByRole("combobox", { name: `${kind} model`, exact: true }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("combobox", { name: `${kind} effort`, exact: true }),
      ).toHaveCount(0);
    }
    const authentication = page.getByRole("button", {
      name: "Log in",
      exact: true,
    });
    await expect(authentication.getByRole("status")).toContainText("Ready");
    const actions = ["Log in", "Save configuration", "Revert changes"];
    const actionBounds = await Promise.all(
      actions.map(async (name) => {
        const button = page.getByRole("button", {
          name,
          exact: true,
        });
        await expect(button).toBeVisible();
        return (await button.boundingBox())!;
      }),
    );
    for (let i = 1; i < actionBounds.length; i++) {
      expect(actionBounds[i].y + actionBounds[i].height / 2).toBeCloseTo(
        actionBounds[0].y + actionBounds[0].height / 2,
        0,
      );
      expect(actionBounds[i].width).toBe(actionBounds[i].height);
      expect(actionBounds[i].x).toBeGreaterThan(
        actionBounds[i - 1].x + actionBounds[i - 1].width,
      );
    }
    if (width === 1280) {
      const boxes = await page
        .getByRole("group", { name: "Auxiliary configuration", exact: true })
        .evaluate((node) =>
          [...node.children].map((child) =>
            child.getBoundingClientRect().toJSON(),
          ),
        );
      expect(boxes).toHaveLength(4);
      for (let i = 0; i < boxes.length; i++) {
        expect(boxes[i].y).toBeCloseTo(boxes[0].y, 0);
        expect(boxes[i].height).toBeCloseTo(boxes[0].height, 0);
        if (i) expect(boxes[i].x).toBeGreaterThan(boxes[i - 1].right);
      }
      const panels = await Promise.all(
        ["Summary", "Suggestion", "Title"].map(
          async (name) =>
            (await page
              .getByRole("region", { name: `${name} configuration` })
              .boundingBox())!,
        ),
      );
      for (let i = 0; i < panels.length; i++) {
        expect(panels[i].y).toBeCloseTo(panels[0].y, 0);
        expect(panels[i].width).toBeGreaterThan(2 * panels[i].height);
        if (i)
          expect(panels[i].x).toBeGreaterThan(
            panels[i - 1].x + panels[i - 1].width,
          );
      }
    }
    await picker(page).click();
    const current = page.locator(".setting-menu .setting-current");
    await expect(current).toBeVisible();
    await expect(current).toContainText("Work");
    await expect
      .poll(async () => {
        const trigger = await picker(page)
          .locator(".button-content")
          .boundingBox();
        const overlay = await current.locator(".button-content").boundingBox();
        return Math.abs(
          trigger!.y + trigger!.height / 2 - overlay!.y - overlay!.height / 2,
        );
      })
      .toBeLessThan(1);
    const work = choices(page).getByRole("option", { name: /Work/ });
    const personal = choices(page).getByRole("option", { name: /Personal/ });
    for (const option of [work, personal]) {
      await expect(option).toHaveAttribute("data-section-layout", "section");
      await expect(option.locator(".agent-brand")).toBeVisible();
      await expect(option.getByText("Remaining", { exact: true })).toHaveCount(
        0,
      );
      await expect(option).toContainText("Resets");
    }
    await expect(work).toContainText("64");
    await expect(personal).toContainText("personal");
    await expect(
      page.getByRole("combobox", { name: "Account", exact: true }),
    ).toHaveCount(0);
    for (const name of ["Summary", "Suggestion", "Title"])
      await expect(
        page.getByRole("region", { name: `${name} configuration` }),
      ).toHaveAttribute("data-section-layout", "section");
    await expect(
      page.getByRole("button", {
        name: "Log in",
        exact: true,
      }),
    ).toHaveCount(1);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    const layout = await work.evaluate((node) => {
      const bounds = node.getBoundingClientRect();
      return [...node.querySelectorAll("time")].every(
        (time) => time.getBoundingClientRect().right <= bounds.right + 1,
      );
    });
    expect(layout).toBe(true);
  });

test("feature panels, switches and keyboard toggle once, and revert uses the latest save", async ({
  page,
}) => {
  await open(page);
  const summary = page.getByRole("region", { name: "Summary configuration" });
  const toggle = summary.getByRole("switch", { name: "Summary enabled" });
  await expect(toggle).toHaveCSS("height", "16px");
  const revert = page.getByRole("button", {
    name: "Revert changes",
    exact: true,
  });
  await expect(toggle).toBeChecked();
  await expect(revert).toBeDisabled();
  await summary.click();
  await expect(toggle).not.toBeChecked();
  await toggle.click();
  await expect(toggle).toBeChecked();
  await toggle.press("Space");
  await expect(toggle).not.toBeChecked();
  const save = page.getByRole("button", {
    name: "Save configuration",
    exact: true,
  });
  await save.click();
  await expect(save).toBeEnabled();
  await expect(revert).toBeDisabled();
  await summary.click();
  await expect(toggle).toBeChecked();
  await revert.click();
  await expect(toggle).not.toBeChecked();
  await expect(revert).toBeDisabled();
});

test("revert restores the saved shared account, model, effort and feature switches", async ({
  page,
}) => {
  await open(page);
  const effort = page.getByRole("combobox", {
    name: "Auxiliary effort",
    exact: true,
  });
  await effort.click();
  await page.getByRole("option", { name: "high", exact: true }).click();
  await page.getByRole("region", { name: "Suggestion configuration" }).click();
  await picker(page).click();
  await choices(page)
    .getByRole("option", { name: /Personal/ })
    .click();
  await expect(picker(page)).toContainText("Personal");
  await page
    .getByRole("button", { name: "Revert changes", exact: true })
    .click();
  await expect(picker(page)).toContainText("Work");
  await expect(
    page.getByRole("combobox", { name: "Auxiliary model", exact: true }),
  ).toContainText("Demo model");
  await expect(effort).toContainText("low");
  await expect(
    page.getByRole("switch", { name: "Summary enabled" }),
  ).toBeChecked();
  for (const name of ["Suggestion", "Title"]) {
    await expect(
      page.getByRole("switch", { name: `${name} enabled` }),
    ).not.toBeChecked();
  }
  await expect(
    page.getByRole("button", { name: "Revert changes", exact: true }),
  ).toBeDisabled();
});

test("changing the shared account clears every task model before configuring and saving", async ({
  page,
}) => {
  await open(page);
  await expect(
    page.getByRole("switch", { name: "Summary enabled" }),
  ).toBeChecked();
  await picker(page).click();
  await choices(page)
    .getByRole("option", { name: /Personal/ })
    .click();
  await expect(picker(page)).toContainText("Personal");
  for (const name of ["Summary", "Suggestion", "Title"]) {
    await expect(
      page.getByRole("switch", { name: `${name} enabled`, exact: true }),
    ).not.toBeChecked();
  }
  await expect(
    page.getByRole("combobox", { name: "Auxiliary model", exact: true }),
  ).toContainText("Choose a model");
  await page
    .getByRole("combobox", { name: "Auxiliary model", exact: true })
    .click();
  await page.getByRole("option", { name: "Demo model", exact: true }).click();
  await page
    .getByRole("switch", { name: "Summary enabled", exact: true })
    .click();
  await page
    .getByRole("combobox", { name: "Auxiliary effort", exact: true })
    .click();
  await page.getByRole("option", { name: "high", exact: true }).click();
  await page
    .getByRole("button", { name: "Save configuration", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Save configuration", exact: true }),
  ).toBeEnabled();
  await expect(picker(page)).toContainText("Personal");
  await expect(
    page.getByRole("switch", { name: "Summary enabled", exact: true }),
  ).toBeChecked();
  await expect(
    page.getByRole("combobox", { name: "Auxiliary effort", exact: true }),
  ).toContainText("high");
});

test("keyboard selection and Escape retain focus on the account selector", async ({
  page,
}) => {
  await open(page);
  await picker(page).focus();
  await page.keyboard.press("ArrowDown");
  await expect(choices(page)).toBeVisible();
  await page.keyboard.press("End");
  await expect(
    choices(page).getByRole("option", { name: /Personal/ }),
  ).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(picker(page)).toContainText("Personal");
  await picker(page).click();
  await page.keyboard.press("Escape");
  await expect(choices(page)).toHaveCount(0);
  await expect(picker(page)).toBeFocused();
});

test("usage without a report stays unknown", async ({ page }) => {
  await open(page, "usage-not-reported");
  await expect(picker(page).locator('[data-level="unknown"]')).toContainText(
    "—",
  );
  await expect(picker(page)).not.toContainText("0%");
  await expect(picker(page).locator("time")).toHaveCount(0);
});

for (const [story, value, level] of [
  ["usage-depleted", 0, "critical"],
  ["usage-full", 100, "normal"],
] as const)
  test(`account quota retains its numeric width and tint at ${value}%`, async ({
    page,
  }) => {
    await open(page, story);
    const remaining = picker(page).locator("[data-level]");
    await expect(remaining).toHaveAttribute("data-level", level);
    await expect(remaining).toContainText(String(value));
    await expect(remaining).toContainText("%");
    const fits = await remaining.evaluate(
      (node) =>
        node.getBoundingClientRect().width <=
        node.parentElement!.getBoundingClientRect().width,
    );
    expect(fits).toBe(true);
  });

test("unavailable accounts cannot be saved or enabled", async ({ page }) => {
  await open(page, "no-accounts");
  await expect(picker(page)).toBeDisabled();
  await expect(
    page.getByRole("switch", { name: "Summary enabled", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Save configuration", exact: true }),
  ).toBeDisabled();
});

test("initial configuration loading disables selection and saving", async ({
  page,
}) => {
  await open(page, "loading");
  await expect(picker(page)).toBeDisabled();
  await expect(
    page.getByText("Loading auxiliary settings…", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Save configuration", exact: true }),
  ).toBeDisabled();
});

test("save failures keep the shared account and allow retry", async ({
  page,
}) => {
  await open(page, "save-failure");
  const save = page.getByRole("button", {
    name: "Save configuration",
    exact: true,
  });
  await save.click();
  await expect(page.getByRole("alert")).toContainText(
    "Configuration save failed",
  );
  await expect(picker(page)).toContainText("Work");
  await expect(
    page.getByRole("switch", { name: "Summary enabled", exact: true }),
  ).toBeChecked();
  await save.click();
  await expect(save).toBeEnabled();
  await expect(page.getByRole("alert")).toHaveCount(0);
});

test("a pending save retains its label and prevents another click", async ({
  page,
}) => {
  await open(page, "pending-save");
  const save = page.getByRole("button", {
    name: "Save configuration",
    exact: true,
  });
  await save.click();
  await expect(save).toBeDisabled();
  await expect(save).toHaveAttribute("aria-busy", "true");
  await expect(save).toHaveAccessibleName("Save configuration");
  await expect(save.locator('[data-icon="save"]')).toBeVisible();
  await expect(picker(page)).toBeDisabled();
  await expect(save).toBeEnabled();
});

test("helper authentication refreshes after completing the shared login", async ({
  page,
}) => {
  await open(page, "login-required");
  const status = page
    .getByRole("button", { name: "Log in", exact: true })
    .getByRole("status");
  await expect(status).toContainText("Login required");
  await expect(
    page.getByRole("combobox", { name: "Auxiliary model", exact: true }),
  ).toBeDisabled();
  const login = page.getByRole("button", { name: "Log in", exact: true });
  await expect(login).toHaveAttribute("data-section-action", "button");
  await expect(login.locator("button")).toHaveCount(0);
  await expect(login.locator('[data-icon="login"]')).toBeVisible();
  await expect(login.getByText("Log in", { exact: true })).toHaveCount(0);
  await login.click();
  const dialog = page.getByRole("dialog", {
    name: "Log in · work",
    exact: true,
  });
  await dialog
    .getByLabel("Authentication code", { exact: true })
    .fill("demo-code");
  await dialog
    .getByRole("button", { name: "Submit code", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(status).toContainText("Ready");
  await expect(
    page.getByRole("combobox", { name: "Auxiliary model", exact: true }),
  ).toBeEnabled();
});

test("mixed saved models are preserved until a shared model and effort are chosen", async ({
  page,
}) => {
  await open(page, "mixed-configuration");
  const model = page.getByRole("combobox", {
    name: "Auxiliary model",
    exact: true,
  });
  const effort = page.getByRole("combobox", {
    name: "Auxiliary effort",
    exact: true,
  });
  const sidebar = page.getByRole("navigation", {
    name: "Resources",
    exact: true,
  });
  async function revisit() {
    await sidebar.getByRole("link", { name: "Accounts", exact: true }).click();
    await sidebar
      .getByRole("link", { name: "Auxiliary AI", exact: true })
      .click();
  }
  await expect(model).toContainText("Mixed models");
  await expect(effort).toContainText("Mixed efforts");
  await page
    .getByRole("button", { name: "Save configuration", exact: true })
    .click();
  await revisit();
  await expect(model).toContainText("Mixed models");
  await expect(effort).toContainText("Mixed efforts");
  await model.click();
  await page
    .getByRole("option", { name: "Alternative model", exact: true })
    .click();
  await expect(effort).toContainText("Provider default");
  await effort.click();
  await page.getByRole("option", { name: "medium", exact: true }).click();
  await page
    .getByRole("button", { name: "Save configuration", exact: true })
    .click();
  await revisit();
  await expect(model).toContainText("Alternative model");
  await expect(effort).toContainText("medium");
  await expect(
    page.getByRole("switch", { name: "Summary enabled", exact: true }),
  ).toBeChecked();
  await expect(
    page.getByRole("switch", { name: "Suggestion enabled", exact: true }),
  ).toBeChecked();
});

for (const width of [390, 1280])
  test(`workspace keeps Auxiliary beside its sidebar without the old panel at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await open(page, "in-workspace");
    const sidebar = page.getByRole("navigation", {
      name: "Resources",
      exact: true,
    });
    const main = page.getByRole("main", { name: "Auxiliary AI", exact: true });
    await expect(sidebar).toBeVisible();
    await expect(page.locator(".workspace > .resource-panel")).toHaveCount(0);
    const bounds = await main.boundingBox();
    const nav = await sidebar.boundingBox();
    expect(bounds!.y).toBeCloseTo(nav!.y, 0);
    expect(bounds!.x).toBeGreaterThanOrEqual(nav!.x + nav!.width);
    expect(bounds!.height).toBe(900);
    await expect(
      page.getByRole("link", { name: "Auxiliary AI", exact: true }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    if (width === 1280) {
      await sidebar
        .getByRole("link", { name: "Sessions view", exact: true })
        .click();
      await expect(
        page.getByRole("table", { name: "Sessions", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("complementary", { name: "Session list", exact: true }),
      ).toHaveCount(0);
      await sidebar
        .getByRole("link", { name: "Auxiliary AI", exact: true })
        .click();
      await expect(main).toBeVisible();
      await expect(page.locator(".workspace > .resource-panel")).toHaveCount(0);
    }
  });
