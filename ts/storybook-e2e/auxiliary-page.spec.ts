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
    await expect(picker(page).locator("time")).toHaveAttribute(
      "datetime",
      "2026-11-08T12:25:00.000Z",
    );
    await picker(page).click();
    const work = choices(page).getByRole("option", { name: /Work/ });
    const personal = choices(page).getByRole("option", { name: /Personal/ });
    for (const option of [work, personal]) {
      await expect(option).toHaveAttribute("data-section-layout", "section");
      await expect(option.locator(".agent-brand")).toBeVisible();
      await expect(option).toContainText("Remaining");
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
      ).not.toHaveAttribute("data-section-layout");
    await expect(
      page.getByRole("button", {
        name: "Log in to helper account",
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
      page.getByRole("combobox", { name: `${name} model`, exact: true }),
    ).toContainText("Choose a model");
    await expect(
      page.getByRole("switch", { name: `${name} enabled`, exact: true }),
    ).not.toBeChecked();
  }
  await page
    .getByRole("combobox", { name: "Summary model", exact: true })
    .click();
  await page.getByRole("option", { name: "Demo model", exact: true }).click();
  await page
    .getByRole("switch", { name: "Summary enabled", exact: true })
    .click();
  await page
    .getByRole("combobox", { name: "Summary effort", exact: true })
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
    page.getByRole("combobox", { name: "Summary effort", exact: true }),
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
  await expect(picker(page)).toContainText("Remaining");
  await expect(picker(page)).not.toContainText("0%");
  await expect(picker(page).locator("time")).toHaveCount(0);
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
  await expect(save).toContainText("Save configuration");
  await expect(picker(page)).toBeDisabled();
  await expect(save).toBeEnabled();
});
