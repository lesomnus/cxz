import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

// The sandbox used to emit one bare `tool_result` with no call, so nothing here
// produced a *paired* tool: a call and a result that the reading projection
// merges into one row carrying the command it ran. sandbox.spec covers that a
// card reaches the work dialog; what was never covered is the merging, and
// where the bytes come from once the row does not carry them. See cxz#155.
//
// The working state is pinned in Go instead -- TestToolRecordsBecomeOneAdvancingRow
// -- because catching it in a browser is a race against the sandbox's own
// pacing, and a test that polls for a window it might miss asserts nothing.
const details = (page: import("@playwright/test").Page) =>
  page
    .locator(".transcript-row")
    .filter({ has: page.locator("[data-work-summary]") })
    .last()
    .getByRole("button", { name: "View details", exact: true });

test("paired tools are one card each, carrying what the row says they are", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/session/session-9");
  await expect(page.locator(".transcript")).toBeVisible({ timeout: 45000 });
  await details(page).click();
  const cards = page.locator("#work-dialog .tool-activity");
  // Five records, two cards: a call, its output and its result merge into one,
  // and the skill's call and result into another.
  await expect(cards).toHaveCount(2);
  const command = page
    .locator('#work-dialog .tool-activity[data-state="completed"]')
    .first();
  // The command is on the row. A client that had to pair records to find it
  // would be deriving what the projection already decided.
  await expect(command).toContainText("ls -la ts/src");

  // "Skill" names every skill, so a card with only the name says nothing about
  // which one ran. The row carries the provider's own words for the call.
  await expect(cards.last()).toContainText("Skill dataviz palette");

  // The output bytes are deliberately not here: the projection drops output
  // records once the row carries the state the tool reached, and a reader that
  // wants them fetches the records. That fetch is the subject of the change
  // which rewires this card, so it is asserted there rather than guessed at
  // through this dialog.
  await expect(page.locator("#work-dialog")).not.toContainText("total 48");
});
