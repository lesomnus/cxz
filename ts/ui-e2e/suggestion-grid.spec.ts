import { expect, test } from "@playwright/test";

test("reusable suggestion grid previews retain host focus", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-suggestiongrid--candidates&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Query" });
  const grid = page.getByRole("grid", { name: "Candidates" });
  await expect(grid.getByRole("gridcell")).toHaveCount(24);
  await input.focus();
  await input.press("ArrowRight");
  await expect(grid.locator('[aria-selected="true"]')).toHaveAttribute(
    "aria-label",
    "candidate-2",
  );
  await grid
    .getByRole("gridcell", { name: "candidate-4", exact: true })
    .click();
  await expect(input).toBeFocused();
  await expect(grid.locator('[aria-selected="true"]')).toHaveAttribute(
    "aria-label",
    "candidate-4",
  );
});
