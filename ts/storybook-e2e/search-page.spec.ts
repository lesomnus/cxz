import { expect, test, type Page } from "@playwright/test";
import { pickSearchDate } from "../test-support/search-dates";
const open = async (page: Page, story = "default") => {
  await page.clock.setFixedTime(new Date("2026-11-05T12:00:00Z"));
  await page.goto(`/iframe.html?id=pages-search--${story}&viewMode=story`);
};
const query = (page: Page) =>
  page.getByRole("searchbox", { name: "Search conversations", exact: true });
test("Search opens with the shared bar and only filter controls below it", async ({
  page,
}) => {
  await open(page);
  await expect(
    page.getByRole("heading", { name: "Search", exact: true }),
  ).toHaveCount(0);
  await expect(query(page)).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Search project", exact: true }),
  ).toHaveText("All projects");
  await expect(
    page.getByRole("button", { name: "Filters", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "From (UTC): From", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: "Until (UTC, exclusive): Until",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("switch", { name: "Include tools", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Search", exact: true }),
  ).toBeDisabled();
  await query(page).fill("search");
  await query(page).press("Enter");
  await expect(
    page.getByRole("link", { name: /Keep the search bar consistent/ }),
  ).toHaveAttribute("href", "/c/_/session/oak?seq=42");
});
test("search modes are toggles and project filters include an all-projects scope", async ({
  page,
}) => {
  await open(page);
  const regex = page.getByRole("button", {
    name: "Use regular expression",
    exact: true,
  });
  const fuzzy = page.getByRole("button", { name: "Fuzzy search", exact: true });
  await regex.click();
  await expect(regex).toHaveAttribute("aria-pressed", "true");
  await fuzzy.click();
  await expect(fuzzy).toHaveAttribute("aria-pressed", "true");
  await expect(regex).toHaveAttribute("aria-pressed", "false");
  await page
    .getByRole("combobox", { name: "Search project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(
    page.getByRole("combobox", { name: "Search project", exact: true }),
  ).toHaveText("API workspace");
  await pickSearchDate(page, "From (UTC)", "2026-11-10");
  await pickSearchDate(page, "Until (UTC, exclusive)", "2026-11-01");
  await query(page).fill("search");
  await query(page).press("Enter");
  await expect(page.getByRole("alert")).toContainText(
    "Choose an end date after the start date.",
  );
  await page
    .getByRole("button", { name: "Reset filters", exact: true })
    .click();
  await expect(
    page.getByRole("combobox", { name: "Search project", exact: true }),
  ).toHaveText("All projects");
  await expect(page.locator('input[name="since"]')).toHaveValue("");
  await expect(page.locator('input[name="until"]')).toHaveValue("");
});
test("changing the query clears old results rather than presenting stale matches", async ({
  page,
}) => {
  await open(page);
  await query(page).fill("first");
  await query(page).press("Enter");
  await expect(
    page.getByRole("link", { name: /Keep the search bar consistent/ }),
  ).toBeVisible();
  await query(page).fill("new");
  await expect(
    page.getByRole("link", { name: /Keep the search bar consistent/ }),
  ).toHaveCount(0);
});
test("streaming search can be stopped without a late result replacing it", async ({
  page,
}) => {
  await open(page, "loading");
  await query(page).fill("search");
  await query(page).press("Enter");
  await page.getByRole("button", { name: "Stop search", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Search", exact: true }),
  ).toBeEnabled();
  await expect(page.getByRole("status")).toContainText("Search stopped.");
  await expect(
    page.getByRole("link", { name: /Keep the search bar consistent/ }),
  ).toHaveCount(0);
});
for (const [story, message] of [
  ["empty", "No matches in retained history."],
  ["failure", "Search is temporarily unavailable"],
] as const)
  test(`Search ${story} state is visible`, async ({ page }) => {
    await open(page, story);
    await query(page).fill("search");
    await query(page).press("Enter");
    await expect(page.getByText(message, { exact: false })).toBeVisible();
  });
test("pagination appends matches to the existing conversation", async ({
  page,
}) => {
  await open(page, "pagination");
  await query(page).fill("search");
  await query(page).press("Enter");
  await page
    .getByRole("button", { name: "Load more results", exact: true })
    .click();
  await expect(
    page.getByRole("link", { name: /Keep the search bar consistent/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: /The follow-up keeps/ }),
  ).toHaveAttribute("href", "/c/_/session/oak?seq=200");
  await expect(
    page.getByRole("button", { name: "Load more results", exact: true }),
  ).toHaveCount(0);
});
for (const width of [390, 1200])
  test(`Search fits ${width}px with inline date filters and long text`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await open(page);
    await query(page).fill("x".repeat(500));
    await query(page).press("Enter");
    await expect(
      page.getByRole("link", { name: /Keep the search bar consistent/ }),
    ).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
  });

test("Nox calendar selection and clearing keep each date boundary independent", async ({
  page,
}) => {
  await open(page);
  await pickSearchDate(page, "From (UTC)", "2026-11-01");
  await pickSearchDate(page, "Until (UTC, exclusive)", "2026-11-05");
  await expect(page.locator('input[name="since"]')).toHaveValue("2026-11-01");
  await expect(page.locator('input[name="until"]')).toHaveValue("2026-11-05");
  await page.getByRole("button", { name: "From (UTC):", exact: false }).click();
  await page
    .getByRole("dialog", { name: "Calendar for From (UTC)", exact: true })
    .getByRole("button", { name: "Clear date", exact: true })
    .click();
  await expect(page.locator('input[name="since"]')).toHaveValue("");
  await expect(page.locator('input[name="until"]')).toHaveValue("2026-11-05");
});
test("Nox calendar remains inside a narrow viewport", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 700 });
  await open(page);
  await page
    .getByRole("button", { name: "Until (UTC, exclusive):", exact: false })
    .click();
  const calendar = page.getByRole("dialog", {
    name: "Calendar for Until (UTC, exclusive)",
    exact: true,
  });
  await expect(calendar).toBeVisible();
  const bounds = await calendar.boundingBox();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.keyboard.press("Escape");
  await expect(calendar).toHaveCount(0);
});
